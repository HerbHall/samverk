package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/api"
	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/dispatcher"
	"github.com/herbhall/samverk/internal/forge"
	giteaadapter "github.com/herbhall/samverk/internal/forge/gitea"
	"github.com/herbhall/samverk/internal/forge/github"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/internal/prwatcher"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/provider/claude"
	"github.com/herbhall/samverk/internal/provider/claudecli"
	"github.com/herbhall/samverk/internal/provider/ollama"
	"github.com/herbhall/samverk/internal/provider/openai"
	"github.com/herbhall/samverk/internal/scaling"
	"github.com/herbhall/samverk/internal/server"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
)

func main() {
	root := &cobra.Command{
		Use:   "samverk",
		Short: "Async background development engine",
		Long:  "Samverk keeps side projects building while you live your life.",
	}

	root.AddCommand(serveCmd())
	root.AddCommand(dispatchCmd())
	root.AddCommand(scaleCmd())
	root.AddCommand(digestCmd())
	root.AddCommand(keyCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	var addr, owner, repo, dbPath, projectsConfig, authKeysPath string
	var budget float64

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server (MCP + API + dashboard)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			cfg := server.Config{Addr: addr}

			// Wire Bearer token auth for MCP endpoint.
			cfg.AuthToken = os.Getenv("SAMVERK_AUTH_TOKEN")
			if cfg.AuthToken != "" {
				slog.Info("MCP authentication enabled (env token)")
			}

			// Load YAML-backed API key store if the file exists.
			if authKeysPath != "" {
				if _, statErr := os.Stat(authKeysPath); statErr == nil {
					ks, ksErr := server.NewKeyStore(authKeysPath)
					if ksErr != nil {
						slog.Warn("could not load API key store", "path", authKeysPath, "error", ksErr)
					} else {
						cfg.KeyStore = ks
						slog.Info("API key authentication enabled", "keys", len(ks.List()), "path", authKeysPath)
					}
				}
			}

			// Open SQLite store for session/cost recording.
			var costs digest.CostSource
			var st store.Store
			if dbPath != "" {
				s, err := store.New(dbPath)
				if err != nil {
					slog.Warn("could not open database", "path", dbPath, "error", err)
				} else {
					st = s
					costs = digest.NewStoreCostSource(s, budget)
					defer func() { _ = s.Close() }()
					slog.Info("database opened", "path", dbPath)
				}
			}

			// Wire GitHub forge if credentials are available.
			var tracker forge.IssueTracker
			var mcpHandler *internalmcp.Handler
			token := os.Getenv("GITHUB_TOKEN")
			if owner == "" {
				owner = os.Getenv("SAMVERK_GITHUB_OWNER")
			}
			if repo == "" {
				repo = os.Getenv("SAMVERK_GITHUB_REPO")
			}
			if token != "" && owner != "" && repo != "" {
				ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
				httpClient := oauth2.NewClient(ctx, ts)
				ghClient := github.New(owner, repo, httpClient)
				tracker = ghClient

				// Load autonomy policy for tier enforcement.
				policyCfg, policyErr := autonomy.LoadOrDefault(".")
				if policyErr != nil {
					slog.Warn("could not load autonomy config, using defaults", "error", policyErr)
					policyCfg = autonomy.DefaultConfig()
				}
				policy := autonomy.NewPolicy(policyCfg)

				// The GitHub Client implements both IssueTracker and RepoReader.
				var repoReader forge.RepoReader = ghClient
				mcpHandler = internalmcp.NewHandler(tracker, costs, st, policy, repoReader)
				mcpHandler.SetPRManager(ghClient)

				// Wire multi-project support.
				registry := internalmcp.NewProjectRegistry()

				// Register the default project from --owner/--repo flags.
				defaultProject := &internalmcp.Project{
					Name:    repo,
					Owner:   owner,
					Repo:    repo,
					Tracker:   tracker,
					Reader:    repoReader,
					PRManager: ghClient,
				}
				if regErr := registry.Register(defaultProject); regErr != nil {
					slog.Warn("could not register default project", "error", regErr)
				}

				// Load additional projects from config file if it exists.
				if projectsConfig != "" {
					if configs, loadErr := internalmcp.LoadProjectConfig(projectsConfig); loadErr == nil {
						for _, pc := range configs {
							// Skip if already registered as the default.
							if pc.Name == repo && pc.Owner == owner && pc.Repo == repo {
								continue
							}

							var p *internalmcp.Project
							if pc.Forge == "gitea" {
								// Resolve token: config field takes precedence, then env var.
								giteaToken := pc.GiteaToken
								if giteaToken == "" {
									giteaToken = os.Getenv("GITEA_TOKEN")
								}
								gtClient, gtErr := giteaadapter.New(pc.GiteaURL, giteaToken, pc.Owner, pc.Repo)
								if gtErr != nil {
									slog.Warn("could not create Gitea client for project",
										"name", pc.Name, "error", gtErr)
									continue
								}
								p = &internalmcp.Project{
									Name:      pc.Name,
									Owner:     pc.Owner,
									Repo:      pc.Repo,
									Tracker:   gtClient,
									Reader:    gtClient,
									PRManager: gtClient,
								}
								slog.Info("registered Gitea project from config",
									"name", pc.Name, "owner", pc.Owner, "repo", pc.Repo, "url", pc.GiteaURL)
							} else {
								// Default: GitHub project.
								ghExtra := github.New(pc.Owner, pc.Repo, httpClient)
								p = &internalmcp.Project{
									Name:      pc.Name,
									Owner:     pc.Owner,
									Repo:      pc.Repo,
									Tracker:   ghExtra,
									Reader:    ghExtra,
									PRManager: ghExtra,
								}
								slog.Info("registered GitHub project from config",
									"name", pc.Name, "owner", pc.Owner, "repo", pc.Repo)
							}

							if regErr := registry.Register(p); regErr != nil {
								slog.Warn("could not register project from config",
									"name", pc.Name, "error", regErr)
							}
						}
					} else if !os.IsNotExist(loadErr) {
						slog.Warn("could not load projects config",
							"path", projectsConfig, "error", loadErr)
					}
				}

				mcpHandler.SetProjects(registry)
				mcpHandler.SetMetrics(nil, nil, metrics.NewSystemCollector())
				if st != nil {
					mcpHandler.SetScalingEventReader(st)
				}
			}

			// Wire REST API handler for dashboard.
			apiHandler := api.New(tracker, st, costs)
			apiHandler.SetMetrics(nil, nil, metrics.NewSystemCollector())
			cfg.APIHandler = apiHandler
			cfg.PressureProvider = apiHandler
			slog.Info("REST API enabled")

			// Wire worker lister from API into MCP digest so the get_digest tool
			// shows registered PC agent workers in the RUNTIME METRICS section.
			if mcpHandler != nil {
				mcpHandler.SetWorkerLister(&apiWorkerAdapter{api: apiHandler})
				cfg.MCPHandler = internalmcp.NewHTTPHandler(mcpHandler)
				slog.Info("MCP handler enabled", "owner", owner, "repo", repo)
			} else {
				slog.Info("MCP handler disabled (set GITHUB_TOKEN, owner, and repo to enable)")
			}

			s := server.New(cfg)
			slog.Info("starting samverk server", "addr", addr)

			if err := s.Start(ctx); err != nil {
				return fmt.Errorf("server error: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	cmd.Flags().StringVar(&owner, "owner", "", "GitHub repository owner")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repository name")
	cmd.Flags().StringVar(&dbPath, "db", ".samverk/samverk.db", "Path to SQLite database for session/cost tracking")
	cmd.Flags().StringVar(&projectsConfig, "projects-config", ".samverk/server.yaml", "Path to multi-project YAML config")
	cmd.Flags().StringVar(&authKeysPath, "auth-keys", ".samverk/auth.yaml", "Path to API key YAML file")
	cmd.Flags().Float64Var(&budget, "budget", 0, "Daily budget in USD (0 = unlimited)")
	return cmd
}

func dispatchCmd() *cobra.Command {
	var owner, repo, dbPath, providersConfig, scalingConfig string
	var pollSeconds, workers, scalingMin, scalingMax int
	var budget float64
	var scalingEnabled bool

	cmd := &cobra.Command{
		Use:   "dispatch",
		Short: "Start the dispatcher (watches issues, routes work to agents)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			token := os.Getenv("GITHUB_TOKEN")
			if owner == "" {
				owner = os.Getenv("SAMVERK_GITHUB_OWNER")
			}
			if repo == "" {
				repo = os.Getenv("SAMVERK_GITHUB_REPO")
			}
			if token == "" || owner == "" || repo == "" {
				return fmt.Errorf("GITHUB_TOKEN, --owner, and --repo are required")
			}

			ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
			httpClient := oauth2.NewClient(ctx, ts)
			ghClient := github.New(owner, repo, httpClient)

			if pollSeconds > 0 {
				ghClient.SetPollInterval(time.Duration(pollSeconds) * time.Second)
			}

			// Load autonomy policy.
			policyCfg, policyErr := autonomy.LoadOrDefault(".")
			if policyErr != nil {
				slog.Warn("could not load autonomy config, using defaults", "error", policyErr)
				policyCfg = autonomy.DefaultConfig()
			}
			policy := autonomy.NewPolicy(policyCfg)

			// Open optional store.
			var st store.Store
			if dbPath != "" {
				s, err := store.New(dbPath)
				if err != nil {
					slog.Warn("could not open database", "path", dbPath, "error", err)
				} else {
					st = s
					defer func() { _ = s.Close() }()
				}
			}

			// Load provider registry and construct agent pool if config exists.
			var pool *agent.Pool
			if providersConfig != "" {
				registry, regErr := provider.LoadRegistry(providersConfig, providerFactory)
				if regErr != nil {
					slog.Warn("could not load provider registry, agents disabled", "path", providersConfig, "error", regErr)
				} else {
					costs := cost.NewTracker(st, budget, 24)
					pool = agent.NewPool(registry, ghClient, st, costs, workers)
					defer pool.Shutdown()
					slog.Info("agent pool started", "workers", workers, "providers", len(registry.List(ctx)))
				}
			}

			disp := dispatcher.New(ghClient, policy, st, pool, nil)
			slog.Info("starting dispatcher", "owner", owner, "repo", repo)

			g, gctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				return disp.Run(gctx)
			})

			// Start autoscaler if pool is active and scaling is enabled.
			if pool != nil && scalingEnabled {
				policyCfgS, cfgErr := scaling.LoadConfigFile(scalingConfig)
				if cfgErr != nil {
					slog.Warn("could not load scaling config, using defaults", "error", cfgErr)
					policyCfgS = scaling.DefaultPolicyConfig()
				}
				if scalingMin > 0 {
					policyCfgS.MinWorkers = scalingMin
				}
				if scalingMax > 0 {
					policyCfgS.MaxWorkers = scalingMax
				}
				// Sync pool max workers to the scaling policy max so
				// AddWorkers/Resize does not reject scale-up events.
				pool.SetMaxWorkers(policyCfgS.MaxWorkers)
				scalingPol := scaling.NewThresholdPolicy(policyCfgS)
				autoscaler := scaling.NewAutoscaler(scalingPol, pool, metrics.NewSystemCollector())
				if st != nil {
					autoscaler.SetPersister(st)
				autoscaler.SetControlReader(st)
				}
				g.Go(func() error {
					err := autoscaler.Run(gctx)
					if err == context.Canceled {
						return nil
					}
					return err
				})
				slog.Info("autoscaler started",
					"min_workers", policyCfgS.MinWorkers,
					"max_workers", policyCfgS.MaxWorkers,
				)
			}

			// Start PR watcher if auto-merge is enabled.
			if policyCfg.Merge.AutoMergeOnCIPass {
				pw := prwatcher.New(ghClient, ghClient, policyCfg.Merge, time.Duration(pollSeconds)*time.Second)
				g.Go(func() error {
					return pw.Run(gctx)
				})
			}

			if err := g.Wait(); err != nil && err != context.Canceled {
				return fmt.Errorf("dispatch error: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "GitHub repository owner")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repository name")
	cmd.Flags().StringVar(&dbPath, "db", ".samverk/samverk.db", "Path to SQLite database")
	cmd.Flags().StringVar(&providersConfig, "providers-config", ".samverk/providers.yaml", "Path to provider registry YAML config")
	cmd.Flags().IntVar(&pollSeconds, "poll-interval", 30, "Polling interval in seconds")
	cmd.Flags().IntVar(&workers, "workers", 3, "Number of agent worker goroutines")
	cmd.Flags().Float64Var(&budget, "budget", 0, "Daily budget in USD (0 = unlimited)")
	cmd.Flags().BoolVar(&scalingEnabled, "scaling", false, "Enable adaptive worker scaling (default: disabled)")
	cmd.Flags().StringVar(&scalingConfig, "scaling-config", "", "Path to scaling policy YAML config (optional)")
	cmd.Flags().IntVar(&scalingMin, "scaling-min", 0, "Override min workers from scaling config (0 = use config value)")
	cmd.Flags().IntVar(&scalingMax, "scaling-max", 0, "Override max workers from scaling config (0 = use config value)")

	return cmd
}

func scaleCmd() *cobra.Command {
	var serverURL, token string

	cmd := &cobra.Command{
		Use:   "scale",
		Short: "Inspect and control adaptive worker scaling",
		Long:  "Commands for viewing scaling state and issuing manual override commands to a running samverk serve process.",
	}

	// Shared flags on the parent.
	cmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8080", "URL of the samverk serve process")
	cmd.PersistentFlags().StringVar(&token, "token", "", "Bearer token for API authentication (or set SAMVERK_AUTH_TOKEN)")

	// Helper: perform an authenticated HTTP request.
	doScaleRequest := func(method, path string, body []byte) ([]byte, int, error) {
		t := token
		if t == "" {
			t = os.Getenv("SAMVERK_AUTH_TOKEN")
		}
		return doHTTPRequest(method, serverURL+path, t, body)
	}

	// samverk scale status
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show current scaling control state",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := doScaleRequest("GET", "/api/v1/scaling/control", nil)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	})

	// samverk scale pause
	cmd.AddCommand(&cobra.Command{
		Use:   "pause",
		Short: "Pause the autoscaler (keep current worker count)",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := doScaleRequest("POST", "/api/v1/scaling/pause", nil)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	})

	// samverk scale resume
	cmd.AddCommand(&cobra.Command{
		Use:   "resume",
		Short: "Resume autonomous autoscaling",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := doScaleRequest("POST", "/api/v1/scaling/resume", nil)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	})

	// samverk scale set N
	setCmd := &cobra.Command{
		Use:   "set <workers>",
		Short: "Force worker count to N and pause the autoscaler",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			n := 0
			if _, err := fmt.Sscanf(args[0], "%d", &n); err != nil || n < 1 {
				return fmt.Errorf("workers must be a positive integer, got %q", args[0])
			}
			body := []byte(fmt.Sprintf(`{"workers":%d}`, n))
			data, _, err := doScaleRequest("POST", "/api/v1/scaling/set", body)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	}
	cmd.AddCommand(setCmd)

	// samverk scale history
	cmd.AddCommand(&cobra.Command{
		Use:   "history",
		Short: "Show recent scaling events",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := doScaleRequest("GET", "/api/v1/metrics", nil)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	})

	return cmd
}

// doHTTPRequest performs an HTTP request and returns the response body and status code.
func doHTTPRequest(method, url, bearerToken string, body []byte) (data []byte, statusCode int, err error) {
	var req *http.Request
	if len(body) > 0 {
		req, err = http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(body))
		if err != nil {
			return nil, 0, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(context.Background(), method, url, http.NoBody)
		if err != nil {
			return nil, 0, fmt.Errorf("build request: %w", err)
		}
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G107: URL comes from --server flag, not user input
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return data, resp.StatusCode, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

func digestCmd() *cobra.Command {
	var owner, repo, since, dbPath string
	var budget float64

	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Show check-in digest from issue tracker (spike prototype)",
		Long:  "Queries GitHub issues and renders a check-in digest showing pending decisions, completed work, and project status.",
		RunE: func(cmd *cobra.Command, args []string) error {
			token := os.Getenv("GITHUB_TOKEN")
			if token == "" {
				return fmt.Errorf("GITHUB_TOKEN environment variable is required")
			}
			if owner == "" {
				owner = os.Getenv("SAMVERK_GITHUB_OWNER")
			}
			if repo == "" {
				repo = os.Getenv("SAMVERK_GITHUB_REPO")
			}
			if owner == "" || repo == "" {
				return fmt.Errorf("--owner and --repo flags (or SAMVERK_GITHUB_OWNER/SAMVERK_GITHUB_REPO env vars) are required")
			}

			dur, err := time.ParseDuration(since)
			if err != nil {
				return fmt.Errorf("invalid --since duration: %w", err)
			}
			lastCheckIn := time.Now().Add(-dur)

			ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
			httpClient := oauth2.NewClient(context.Background(), ts)

			tracker := github.New(owner, repo, httpClient)
			ctx := context.Background()

			// Wire cost source from SQLite store if the database exists.
			var costs digest.CostSource
			if dbPath != "" {
				s, storeErr := store.New(dbPath)
				if storeErr != nil {
					slog.Warn("could not open cost database, continuing without cost data", "path", dbPath, "error", storeErr)
				} else {
					defer func() { _ = s.Close() }()
					costs = digest.NewStoreCostSource(s, budget)
				}
			}

			d, err := digest.BuildDigest(ctx, tracker, costs, lastCheckIn)
			if err != nil {
				return fmt.Errorf("building digest: %w", err)
			}

			fmt.Print(digest.FormatDigest(d))
			return nil
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "GitHub repository owner")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repository name")
	cmd.Flags().StringVar(&since, "since", "24h", "Time window for digest (e.g., 24h, 720h)")
	cmd.Flags().StringVar(&dbPath, "db", ".samverk/samverk.db", "Path to SQLite database for cost tracking")
	cmd.Flags().Float64Var(&budget, "budget", 0, "Daily budget in USD (0 = unlimited)")

	return cmd
}

func keyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage API keys for server authentication",
	}

	cmd.AddCommand(keyCreateCmd())
	cmd.AddCommand(keyListCmd())
	cmd.AddCommand(keyRevokeCmd())

	return cmd
}

func keyCreateCmd() *cobra.Command {
	var name, authKeysPath string
	var projects []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			ks, err := server.NewKeyStore(authKeysPath)
			if err != nil {
				return fmt.Errorf("load key store: %w", err)
			}

			plaintext, err := ks.Create(name, projects)
			if err != nil {
				return fmt.Errorf("create key: %w", err)
			}

			fmt.Printf("API key created. Token: %s\n", plaintext)
			fmt.Println("Save this key -- it cannot be retrieved again.")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name for the API key (required)")
	cmd.Flags().StringSliceVar(&projects, "project", nil, "Project scope (repeatable; omit for all projects)")
	cmd.Flags().StringVar(&authKeysPath, "auth-keys", ".samverk/auth.yaml", "Path to API key YAML file")

	return cmd
}

func keyListCmd() *cobra.Command {
	var authKeysPath string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			ks, err := server.NewKeyStore(authKeysPath)
			if err != nil {
				return fmt.Errorf("load key store: %w", err)
			}

			keys := ks.List()
			if len(keys) == 0 {
				fmt.Println("No API keys configured.")
				return nil
			}

			fmt.Printf("%-20s %-30s %s\n", "NAME", "PROJECTS", "CREATED")
			fmt.Printf("%-20s %-30s %s\n", "----", "--------", "-------")
			for i := range keys {
				proj := "(all)"
				if len(keys[i].Projects) > 0 {
					proj = fmt.Sprintf("%v", keys[i].Projects)
				}
				fmt.Printf("%-20s %-30s %s\n", keys[i].Name, proj, keys[i].CreatedAt)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&authKeysPath, "auth-keys", ".samverk/auth.yaml", "Path to API key YAML file")

	return cmd
}

func keyRevokeCmd() *cobra.Command {
	var name, authKeysPath string

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke an API key by name",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			ks, err := server.NewKeyStore(authKeysPath)
			if err != nil {
				return fmt.Errorf("load key store: %w", err)
			}

			if err := ks.Revoke(name); err != nil {
				return fmt.Errorf("revoke key: %w", err)
			}

			fmt.Printf("API key %q revoked.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the API key to revoke (required)")
	cmd.Flags().StringVar(&authKeysPath, "auth-keys", ".samverk/auth.yaml", "Path to API key YAML file")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("samverk %s (commit: %s, built: %s)\n",
				version.Version, version.GitCommit, version.BuildDate)
		},
	}
}

// apiWorkerAdapter bridges api.WorkerRecord to mcp.WorkerInfo without
// creating an import cycle between the api and mcp packages.
type apiWorkerAdapter struct{ api *api.API }

func (a *apiWorkerAdapter) ListWorkers() []internalmcp.WorkerInfo {
	records := a.api.ListWorkers()
	out := make([]internalmcp.WorkerInfo, len(records))
	for i := range records {
		r := &records[i]
		out[i] = internalmcp.WorkerInfo{
			AgentID:         r.AgentID,
			Hostname:        r.Hostname,
			Status:          string(r.Status),
			CurrentTask:     r.CurrentTask,
			ActiveWorktrees: r.ActiveWorktrees,
			CPUPercent:      r.CPUPercent,
			MemoryPercent:   r.MemoryPercent,
		}
	}
	return out
}

// providerFactory constructs a provider.Provider from YAML config.
// It wires the concrete provider sub-packages (claude, openai, ollama)
// so the registry package doesn't import them directly.
func providerFactory(name string, cfg provider.ProviderConfig) (provider.Provider, error) {
	switch cfg.Type {
	case "claude":
		apiKey := os.Getenv(cfg.APIKeyEnv)
		if apiKey == "" {
			return nil, fmt.Errorf("provider %q: env var %s is not set", name, cfg.APIKeyEnv)
		}
		model := cfg.DefaultModel
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		return claude.New(apiKey, model), nil
	case "openai":
		apiKey := os.Getenv(cfg.APIKeyEnv)
		if apiKey == "" {
			return nil, fmt.Errorf("provider %q: env var %s is not set", name, cfg.APIKeyEnv)
		}
		model := cfg.DefaultModel
		if model == "" {
			model = "gpt-4o"
		}
		return openai.New(apiKey, model), nil
	case "ollama":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		if cfg.TimeoutSeconds > 0 {
			return ollama.NewWithTimeout(baseURL, time.Duration(cfg.TimeoutSeconds)*time.Second), nil
		}
		return ollama.New(baseURL), nil
	case "claude-cli":
		model := cfg.DefaultModel
		if cfg.TimeoutSeconds > 0 {
			return claudecli.NewWithTimeout(model, time.Duration(cfg.TimeoutSeconds)*time.Second), nil
		}
		return claudecli.New(model), nil
	default:
		return nil, fmt.Errorf("provider %q: unknown type %q", name, cfg.Type)
	}
}
