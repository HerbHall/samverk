package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"go.uber.org/zap"

	"samverk.dev/samverk/internal/logging"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: intentional pprof endpoint for memory diagnosis (#516)
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"samverk.dev/samverk/internal/advisor"
	"samverk.dev/samverk/internal/agent"
	"samverk.dev/samverk/internal/api"
	"samverk.dev/samverk/internal/docaudit"
	"samverk.dev/samverk/internal/hostmetrics"
	"samverk.dev/samverk/internal/infra"
	"samverk.dev/samverk/internal/autonomy"
	"samverk.dev/samverk/internal/cost"
	"samverk.dev/samverk/internal/digest"
	"samverk.dev/samverk/internal/dispatcher"
	"samverk.dev/samverk/internal/eventbus"
	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/internal/logstore"
	giteaadapter "samverk.dev/samverk/internal/forge/gitea"
	"samverk.dev/samverk/internal/forge/github"
	"samverk.dev/samverk/internal/loganalyst"
	internalmcp "samverk.dev/samverk/internal/mcp"
	"samverk.dev/samverk/internal/metrics"
	"samverk.dev/samverk/internal/prwatcher"
	"samverk.dev/samverk/internal/provider"
	"samverk.dev/samverk/internal/provider/claude"
	"samverk.dev/samverk/internal/provider/claudecli"
	"samverk.dev/samverk/internal/provider/ollama"
	"samverk.dev/samverk/internal/provider/openai"
	"samverk.dev/samverk/internal/scaling"
	"samverk.dev/samverk/internal/server"
	"samverk.dev/samverk/internal/status"
	"samverk.dev/samverk/internal/store"
	"samverk.dev/samverk/internal/synapset"
	"samverk.dev/samverk/internal/version"
	"samverk.dev/samverk/pkg/models"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
)

var logger *zap.Logger

func main() {
	os.Exit(run())
}

func run() int {
	var err error
	logger, err = logging.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		return 1
	}
	defer func() { _ = logger.Sync() }()

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
	root.AddCommand(statusCmd())
	root.AddCommand(wakeCmd())
	root.AddCommand(docAuditCmd())
	root.AddCommand(auditCmd())
	root.AddCommand(probeCmd())
	root.AddCommand(backfillLabelsCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

func serveCmd() *cobra.Command {
	var addr, owner, repo, dbPath, projectsConfig, authKeysPath, providersConfigServe string
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
				logger.Info("MCP authentication enabled (env token)")
			}

			// Wire Cloudflare Access JWT auto-login.
			cfg.CFAccessTeamDomain = os.Getenv("CF_ACCESS_TEAM_DOMAIN")
			cfg.SecureCookies = true
			cfg.CFAccessMCPClientID = os.Getenv("CF_ACCESS_MCP_CLIENT_ID")
			if cfg.CFAccessTeamDomain != "" && cfg.CFAccessMCPClientID != "" {
				logger.Info("CF Access JWT enforcement enabled for /mcp",
					zap.String("team_domain", cfg.CFAccessTeamDomain))
			}
			if cfg.CFAccessTeamDomain != "" {
				logger.Info("Cloudflare Access JWT auto-login enabled",
					zap.String("team_domain", cfg.CFAccessTeamDomain))
			}

			// Persist browser sessions alongside the database so logins survive restarts.
			if dbPath != "" {
				cfg.SessionFile = strings.TrimSuffix(dbPath, ".db") + "-sessions.json"
			}

			// Load YAML-backed API key store if the file exists.
			if authKeysPath != "" {
				if _, statErr := os.Stat(authKeysPath); statErr == nil {
					ks, ksErr := server.NewKeyStore(authKeysPath)
					if ksErr != nil {
						logger.Warn("could not load API key store", zap.String("path", authKeysPath), zap.Error(ksErr)) //nolint:gosec // G101: authKeysPath is a filesystem path, not a credential
					} else {
						cfg.KeyStore = ks
						logger.Info("API key authentication enabled", zap.Int("keys", len(ks.List())), zap.String("path", authKeysPath)) //nolint:gosec // G101: authKeysPath is a filesystem path, not a credential
					}
				}
			}

			// Open SQLite store for session/cost recording.
			var costs digest.CostSource
			var st store.Store
			if dbPath != "" {
				s, err := store.New(dbPath)
				if err != nil {
					logger.Warn("could not open database", zap.String("path", dbPath), zap.Error(err))
				} else {
					st = s
					costs = digest.NewStoreCostSource(s, budget)
					defer func() { _ = s.Close() }()
					logger.Info("database opened", zap.String("path", dbPath))
				}
			}

			// Create log store for structured log persistence (separate DB).
			var ls *logstore.LogStore
			var lsSyncer *logstore.Syncer
			if dbPath != "" {
				logsDBPath := strings.TrimSuffix(dbPath, ".db") + "-logs.db"
				var lsErr error
				ls, lsErr = logstore.New(logsDBPath)
				if lsErr != nil {
					logger.Warn("could not open log store", zap.String("path", logsDBPath), zap.Error(lsErr))
				} else {
					defer func() { _ = ls.Close() }()
					// Upgrade logger to tee to SQLite.
					teeLogger, syncer, teeErr := logging.NewWithTee(ls)
					if teeErr != nil {
						logger.Warn("could not create tee logger", zap.Error(teeErr))
					} else {
						logger = teeLogger
						lsSyncer = syncer
					}
					// Start log pruner (30 day retention).
					go logstore.StartPruner(ctx, ls, 30*24*time.Hour, logger)
					logger.Info("log store enabled", zap.String("path", logsDBPath))
				}
			}

			// Wire forge clients and MCP handler.
			// The MCP handler and project registry are always created; GitHub env
			// vars are only required to register a GitHub default project.
			var tracker forge.IssueTracker
			var repoReader forge.RepoReader
			var mcpHandler *internalmcp.Handler

			token := os.Getenv("GITHUB_TOKEN")
			if owner == "" {
				owner = os.Getenv("SAMVERK_GITHUB_OWNER")
			}
			if repo == "" {
				repo = os.Getenv("SAMVERK_GITHUB_REPO")
			}

			// Load autonomy policy for tier enforcement.
			policyCfg, policyErr := autonomy.LoadOrDefault(".")
			if policyErr != nil {
				logger.Warn("could not load autonomy config, using defaults", zap.Error(policyErr))
				policyCfg = autonomy.DefaultConfig()
			}
			policy := autonomy.NewPolicy(policyCfg)

			// Build project registry unconditionally.
			registry := internalmcp.NewProjectRegistry()

			// Register default GitHub project only when all env vars are present.
			var ghDefaultClient *github.Client
			var httpClient *http.Client
			if token != "" && owner != "" && repo != "" {
				ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
				httpClient = oauth2.NewClient(ctx, ts)
				ghDefaultClient = github.New(owner, repo, httpClient)
				tracker = ghDefaultClient
				repoReader = ghDefaultClient
				defaultProject := &internalmcp.Project{
					Name:      repo,
					Owner:     owner,
					Repo:      repo,
					Phase:     "development",
					ForgeType: "github",
					ForgeURL:  "https://github.com",
					Tracker:   tracker,
					Reader:    repoReader,
					PRManager: ghDefaultClient,
				}
				if regErr := registry.Register(defaultProject); regErr != nil {
					logger.Warn("could not register default project", zap.Error(regErr))
				}
				logger.Info("GitHub default project registered",
					zap.String("owner", owner), zap.String("repo", repo))
			} else {
				logger.Info("no GitHub default project (GITHUB_TOKEN/SAMVERK_GITHUB_OWNER/SAMVERK_GITHUB_REPO not set)")
			}

			// Load projects from config file unconditionally.
			if projectsConfig != "" {
				if configs, loadErr := internalmcp.LoadProjectConfig(projectsConfig); loadErr == nil {
					for i := range configs {
						pc := &configs[i]
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
								logger.Warn("could not create Gitea client for project",
									zap.String("name", pc.Name), zap.Error(gtErr))
								continue
							}
							p = &internalmcp.Project{
								Name:      pc.Name,
								Owner:     pc.Owner,
								Repo:      pc.Repo,
								Phase:     pc.Phase,
								Tags:      pc.Tags,
								ForgeType: "gitea",
								ForgeURL:  pc.GiteaURL,
								RepoDir:   pc.RepoDir,
								Tracker:   gtClient,
								Reader:    gtClient,
								Writer:    gtClient,
								PRManager: gtClient,
							}
							logger.Info("registered Gitea project from config",
								zap.String("name", pc.Name), zap.String("owner", pc.Owner),
								zap.String("repo", pc.Repo), zap.String("url", pc.GiteaURL))
						} else {
							// GitHub project from config requires httpClient.
							if httpClient == nil {
								logger.Warn("skipping GitHub project from config: GITHUB_TOKEN not set",
									zap.String("name", pc.Name))
								continue
							}
							ghExtra := github.New(pc.Owner, pc.Repo, httpClient)
							p = &internalmcp.Project{
								Name:      pc.Name,
								Owner:     pc.Owner,
								Repo:      pc.Repo,
								Phase:     pc.Phase,
								Tags:      pc.Tags,
								ForgeType: "github",
								ForgeURL:  "https://github.com",
								RepoDir:   pc.RepoDir,
								Tracker:   ghExtra,
								Reader:    ghExtra,
								Writer:    ghExtra,
								PRManager: ghExtra,
							}
							logger.Info("registered GitHub project from config",
								zap.String("name", pc.Name), zap.String("owner", pc.Owner), zap.String("repo", pc.Repo))
						}

						if regErr := registry.Register(p); regErr != nil {
							logger.Warn("could not register project from config",
								zap.String("name", pc.Name), zap.Error(regErr))
						}
					}
					// Record the config path so set_project_phase can persist phase changes.
					registry.SetConfigPath(projectsConfig)
				} else if !os.IsNotExist(loadErr) {
					logger.Warn("could not load projects config",
						zap.String("path", projectsConfig), zap.Error(loadErr))
				}
			}

			// If no GitHub default, fall back to first registry project as the active
			// tracker. This makes the server functional with Gitea-only config.
			if tracker == nil {
				if projects := registry.List(); len(projects) > 0 {
					tracker = projects[0].Tracker
					repoReader = projects[0].Reader
					logger.Info("using first project as default tracker",
						zap.String("name", projects[0].Name))
				}
			}

			// Resolve PRManager: prefer GitHub default client, then first
			// registry project that provides one (e.g. Gitea-only config).
			var primaryPRMgr forge.PullRequestManager
			if ghDefaultClient != nil {
				primaryPRMgr = ghDefaultClient
			} else if projects := registry.List(); len(projects) > 0 && projects[0].PRManager != nil {
				primaryPRMgr = projects[0].PRManager
				logger.Info("using first project as default PR manager",
					zap.String("name", projects[0].Name))
			}

			// Startup health assertion: fail loudly if auto-merge is enabled
			// but the PR manager could not be resolved.
			if policyCfg.Merge.AutoMergeOnCIPass && primaryPRMgr == nil {
				return fmt.Errorf("auto_merge_on_ci_pass is enabled but PR manager failed to initialize")
			}

			// Always create the MCP handler with the resolved tracker.
			mcpHandler = internalmcp.NewHandler(tracker, costs, st, policy, repoReader)
			if primaryPRMgr != nil {
				mcpHandler.SetPRManager(primaryPRMgr)
			}
			mcpHandler.SetProjects(registry)
			mcpHandler.SetWorkCoordinator(internalmcp.NewForgeWorkCoordinator(registry, logger))
			if st != nil {
				poolM, dispM := api.NewStoreBackedMetrics(st)
				mcpHandler.SetMetrics(poolM, dispM, metrics.NewSystemCollector())
			} else {
				mcpHandler.SetMetrics(nil, nil, metrics.NewSystemCollector())
			}
			if st != nil {
				mcpHandler.SetScalingEventReader(st)
			}

			// Wire REST API handler for dashboard.
			apiHandler := api.New(tracker, st, costs, logger)
			// Use store-backed metrics to bridge cross-process gap:
			// dispatch process writes snapshots to SQLite, serve reads them here.
			if st != nil {
				poolM, dispM := api.NewStoreBackedMetrics(st)
				apiHandler.SetMetrics(poolM, dispM, metrics.NewSystemCollector())
			} else {
				apiHandler.SetMetrics(nil, nil, metrics.NewSystemCollector())
			}
			// Load provider config for capacity display (read-only, no provider instances).
			if providersConfigServe != "" {
				regCfg, pcErr := provider.LoadRegistryConfig(providersConfigServe)
				if pcErr == nil {
					pdtos := make([]api.ProviderDTO, 0, len(regCfg.Providers))
					for name := range regCfg.Providers {
						pcfg := regCfg.Providers[name]
						pdtos = append(pdtos, api.ProviderDTO{
							Name:       name,
							Type:       pcfg.Type,
							Model:      pcfg.DefaultModel,
							Healthy:    true, // Present in config = assumed healthy; real probe is a separate concern.
							AccountURL: pcfg.AccountURL,
						})
					}
					apiHandler.SetCapacity(pdtos, regCfg.Routing)
					// Build a routing-only registry for the health endpoint so it can
					// include routing_chains without requiring full provider instantiation.
					routingReg := provider.NewRegistry(logger)
					routingReg.SetRouting(regCfg.Routing)
					apiHandler.SetProviderRegistry(routingReg)
					logger.Info("provider capacity loaded for dashboard", zap.Int("providers", len(pdtos)))
				} else if !os.IsNotExist(pcErr) {
					logger.Warn("could not load providers config for dashboard", zap.Error(pcErr))
				}
			}
			if ls != nil {
				apiHandler.SetLogStore(ls)
				mcpHandler.SetLogQuerier(ls)
			}
			apiHandler.SetProjectRegistry(registry)
			apiHandler.SetSynapsetProxy("https://synapset.herbhall.net")
			cfg.APIHandler = apiHandler
			cfg.PressureProvider = apiHandler
			logger.Info("REST API enabled")

			// Start host metrics collector.
			hm := hostmetrics.NewCollector("/")
			apiHandler.SetHostMetrics(hm)

			// Configure Proxmox infra context for capacity action generation.
			containerID := os.Getenv("SAMVERK_CONTAINER_ID")
			if containerID == "" {
				containerID = "202"
			}
			proxmoxHost := os.Getenv("SAMVERK_PROXMOX_HOST")
			if proxmoxHost == "" {
				proxmoxHost = "192.168.1.203"
			}
			apiHandler.SetInfraContext(hostmetrics.InfraContext{
				ContainerID: containerID,
				ProxmoxHost: proxmoxHost,
			})
			go func() {
				if hmErr := hm.Run(ctx); hmErr != nil && hmErr != context.Canceled {
					logger.Warn("host metrics collector stopped", zap.Error(hmErr))
				}
			}()
			logger.Info("host metrics collector started")

			// Start advisory engine (quality KPI recommendations).
			if st != nil {
				adv := advisor.New(st, logger)
				apiHandler.SetAdvisor(adv)
				go func() {
					if advErr := adv.Start(ctx); advErr != nil && !errors.Is(advErr, context.Canceled) {
						logger.Warn("advisory engine stopped", zap.Error(advErr))
					}
				}()
				logger.Info("advisory engine started")
			}

			// Wire log analyst if logstore is available.
			if ls != nil {
				ollamaURL := os.Getenv("SAMVERK_OLLAMA_URL")
				if ollamaURL == "" {
					ollamaURL = "http://192.168.1.207:11434"
				}
				ollamaModel := os.Getenv("SAMVERK_ANALYST_MODEL")
				if ollamaModel == "" {
					ollamaModel = "qwen2.5-coder:7b"
				}
				var ollamaClient loganalyst.OllamaClient = loganalyst.NewOllamaAnalystClient(ollamaURL, ollamaModel, 120*time.Second)
				analyst := loganalyst.New(ls, ollamaClient, logger)
				apiHandler.SetLogAnalyst(analyst)
				logger.Info("log analyst enabled",
					zap.String("ollama_url", ollamaURL),
					zap.String("model", ollamaModel),
				)
			}

			// Wire worker lister from API into MCP digest so the get_digest tool
			// shows registered PC agent workers in the RUNTIME METRICS section.
			mcpHandler.SetWorkerLister(&apiWorkerAdapter{api: apiHandler})
			mcpHandler.SetDiagnosticsSource(apiHandler)

			// Audit MCP handler wiring -- surface missing dependencies at startup
			// instead of silently degrading tools at runtime.
			if gaps := mcpHandler.AuditWiring(); len(gaps) > 0 {
				for _, g := range gaps {
					logger.Warn("MCP wiring gap: tool will degrade",
						zap.String("dependency", g.Dependency),
						zap.String("affected_tools", g.Tool),
						zap.String("impact", g.Description),
					)
				}
			}

			cfg.MCPHandler = internalmcp.NewHTTPHandler(mcpHandler)
			cfg.VanityResolver = registry
			apiHandler.SetToolCount(internalmcp.CountTools())
			logger.Info("MCP handler enabled",
				zap.String("owner", owner), zap.String("repo", repo))

			// Create subsystem liveness registry and register serve-process subsystems.
			subsystemReg := status.NewRegistry()
			apiHandler.SetSubsystemRegistry(subsystemReg)
			subsystemReg.Register("serve", true)
			logger.Info("subsystem liveness registry created")

			s := server.New(cfg, logger)
			// Wire log.entry WS broadcasts: each persisted log entry is pushed
			// to all connected dashboard clients via the Hub broadcaster.
			if lsSyncer != nil {
				lsSyncer.SetBroadcaster(s.Hub())
			}
			logger.Info("starting samverk server", zap.String("addr", addr))

			// Start a standalone MCP-only listener on port 8081 for
			// external access via mcp.herbhall.net. No SPA, no API --
			// just the MCP handler. This avoids Cloudflare WAF's
			// "Validate Headers" rule which blocks /mcp on servers
			// that serve HTML (the SPA catch-all triggers it).
			if cfg.MCPHandler != nil {
				mcpMux := http.NewServeMux()
				// Apply CF Access JWT enforcement and Bearer auth to MCP-only listener.
				mcpHandler := cfg.MCPHandler
				if cfg.CFAccessTeamDomain != "" && cfg.CFAccessMCPClientID != "" {
					mcpHandler = server.CFAccessEnforceMiddleware(cfg.CFAccessTeamDomain, cfg.CFAccessMCPClientID)(mcpHandler)
				}
				if cfg.AuthToken != "" || cfg.KeyStore != nil {
					mcpMux.Handle("/", server.BearerAuth(cfg.AuthToken, cfg.KeyStore)(mcpHandler))
				} else {
					mcpMux.Handle("/", mcpHandler)
				}
				mcpMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				})
				mcpSrv := &http.Server{
					Addr:              ":8081",
					Handler:           mcpMux,
					ReadHeaderTimeout: 10 * time.Second,
				}
				go func() {
					logger.Info("MCP-only listener starting", zap.String("addr", ":8081"))
					if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						logger.Error("MCP-only listener failed", zap.Error(err))
					}
				}()
				go func() {
					<-ctx.Done()
					_ = mcpSrv.Shutdown(context.Background())
				}()
			}

			// Wire Synapset client for infra probe (optional, best-effort).
			var infraSynapsetClient *synapset.Client
			synCfg := synapset.ConfigFromEnv(synapset.Config{})
			if synCfg.URL != "" {
				infraSynapsetClient = synapset.New(synCfg, logger)
				logger.Info("synapset client wired for infra probe", zap.String("url", synCfg.URL))
			}

			// Start nightly infra probe goroutine — fires daily at 3am local time.
			go func() {
				prober := infra.NewProber(infraSynapsetClient, logger, infra.DefaultEndpoints())

				now := time.Now()
				next3am := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
				if !next3am.After(now) {
					next3am = next3am.Add(24 * time.Hour)
				}
				timer := time.NewTimer(time.Until(next3am))
				defer timer.Stop()
				logger.Info("nightly infra probe scheduled", zap.Time("next_run", next3am))

				for {
					select {
					case <-ctx.Done():
						return
					case <-timer.C:
						prober.Run(ctx)
						logger.Info("infra probe complete")
						timer.Reset(24 * time.Hour)
					}
				}
			}()

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
	cmd.Flags().StringVar(&providersConfigServe, "providers-config", ".samverk/providers.yaml", "Path to provider registry YAML (read-only, for capacity display)")
	return cmd
}

func dispatchCmd() *cobra.Command {
	var owner, repo, dbPath, projectsConfig, providersConfig, scalingConfig, repoDir string
	var forgeName, giteaURL string
	var pollSeconds, workers, scalingMin, scalingMax int
	var budget float64
	var scalingEnabled bool

	cmd := &cobra.Command{
		Use:   "dispatch",
		Short: "Start the dispatcher (watches issues, routes work to agents)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			// Start pprof server for memory diagnosis (issue #516).
			go func() {
				pprofServer := &http.Server{Addr: ":6060", ReadHeaderTimeout: 10 * time.Second}
				_ = pprofServer.ListenAndServe()
			}()

			// Build project registry from config and/or legacy flags.
			registry := internalmcp.NewProjectRegistry()

			// Load projects from config file if specified.
			if projectsConfig != "" {
				if configs, loadErr := internalmcp.LoadProjectConfig(projectsConfig); loadErr == nil {
					var ghHTTPClient *http.Client
					for i := range configs {
						pc := &configs[i]
						var p *internalmcp.Project
						if pc.Forge == "gitea" {
							giteaToken := pc.GiteaToken
							if giteaToken == "" {
								giteaToken = os.Getenv("GITEA_TOKEN")
							}
							gtClient, gtErr := giteaadapter.New(pc.GiteaURL, giteaToken, pc.Owner, pc.Repo)
							if gtErr != nil {
								logger.Warn("could not create Gitea client for project",
									zap.String("name", pc.Name), zap.Error(gtErr))
								continue
							}
							if pollSeconds > 0 {
								gtClient.SetPollInterval(time.Duration(pollSeconds) * time.Second)
							}
							p = &internalmcp.Project{
								Name:      pc.Name,
								Owner:     pc.Owner,
								Repo:      pc.Repo,
								Phase:     pc.Phase,
								Tags:      pc.Tags,
								ForgeType: "gitea",
								ForgeURL:  pc.GiteaURL,
								RepoDir:   pc.RepoDir,
								Tracker:   gtClient,
								Writer:    gtClient,
								PRManager: gtClient,
							}
						} else {
							// GitHub project.
							token := os.Getenv("GITHUB_TOKEN")
							if token == "" {
								logger.Warn("skipping GitHub project: GITHUB_TOKEN not set",
									zap.String("name", pc.Name))
								continue
							}
							if ghHTTPClient == nil {
								ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
								ghHTTPClient = oauth2.NewClient(ctx, ts)
							}
							ghClient := github.New(pc.Owner, pc.Repo, ghHTTPClient)
							if pollSeconds > 0 {
								ghClient.SetPollInterval(time.Duration(pollSeconds) * time.Second)
							}
							p = &internalmcp.Project{
								Name:      pc.Name,
								Owner:     pc.Owner,
								Repo:      pc.Repo,
								Phase:     pc.Phase,
								Tags:      pc.Tags,
								ForgeType: "github",
								ForgeURL:  "https://github.com",
								RepoDir:   pc.RepoDir,
								Tracker:   ghClient,
								Writer:    ghClient,
								PRManager: ghClient,
							}
						}
						if regErr := registry.Register(p); regErr != nil {
							logger.Warn("could not register project",
								zap.String("name", pc.Name), zap.Error(regErr))
						}
					}
					registry.SetConfigPath(projectsConfig)
				} else if !os.IsNotExist(loadErr) {
					logger.Warn("could not load projects config",
						zap.String("path", projectsConfig), zap.Error(loadErr))
				}
			}

			// Legacy single-project flags: add as backward-compatible entry if not already in registry.
			if owner == "" {
				owner = os.Getenv("SAMVERK_GITHUB_OWNER")
			}
			if repo == "" {
				repo = os.Getenv("SAMVERK_GITHUB_REPO")
			}
			if owner != "" && repo != "" {
				if _, exists := registry.Get(repo); !exists {
					var legacyTracker forge.IssueTracker
					var legacyPRMgr forge.PullRequestManager
					var legacyWriter forge.RepoWriter
					var legacyForgeType, legacyForgeURL string
					switch forgeName {
					case "gitea":
						giteaToken := os.Getenv("GITEA_TOKEN")
						if giteaToken == "" {
							return fmt.Errorf("GITEA_TOKEN env var is required for --forge gitea")
						}
						if giteaURL == "" {
							return fmt.Errorf("--gitea-url is required for --forge gitea")
						}
						gtClient, gtErr := giteaadapter.New(giteaURL, giteaToken, owner, repo)
						if gtErr != nil {
							return fmt.Errorf("creating Gitea client: %w", gtErr)
						}
						if pollSeconds > 0 {
							gtClient.SetPollInterval(time.Duration(pollSeconds) * time.Second)
						}
						legacyTracker = gtClient
						legacyPRMgr = gtClient
						legacyWriter = gtClient
						legacyForgeType = "gitea"
						legacyForgeURL = giteaURL
					default: // "github" or empty
						token := os.Getenv("GITHUB_TOKEN")
						if token == "" {
							return fmt.Errorf("GITHUB_TOKEN env var is required")
						}
						ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
						hc := oauth2.NewClient(ctx, ts)
						ghClient := github.New(owner, repo, hc)
						if pollSeconds > 0 {
							ghClient.SetPollInterval(time.Duration(pollSeconds) * time.Second)
						}
						legacyTracker = ghClient
						legacyPRMgr = ghClient
						legacyWriter = ghClient
						legacyForgeType = "github"
						legacyForgeURL = "https://github.com"
					}
					legacyProject := &internalmcp.Project{
						Name:      repo,
						Owner:     owner,
						Repo:      repo,
						Phase:     "development",
						ForgeType: legacyForgeType,
						ForgeURL:  legacyForgeURL,
						Tracker:   legacyTracker,
						PRManager: legacyPRMgr,
						Writer:    legacyWriter,
					}
					if regErr := registry.Register(legacyProject); regErr != nil {
						logger.Warn("could not register legacy project", zap.Error(regErr))
					}
				}
			}

			// At least one project is required.
			if len(registry.List()) == 0 {
				return fmt.Errorf("no projects configured: set --projects-config or --owner/--repo flags")
			}

			// Resolve primary tracker from the active registry project.
			var primaryTracker forge.IssueTracker
			var primaryPRMgr forge.PullRequestManager
			if activeProj, activeErr := registry.Active(); activeErr == nil {
				primaryTracker = activeProj.Tracker
				primaryPRMgr = activeProj.PRManager
			}

			// Load autonomy policy.
			policyCfg, policyErr := autonomy.LoadOrDefault(".")
			if policyErr != nil {
				logger.Warn("could not load autonomy config, using defaults", zap.Error(policyErr))
				policyCfg = autonomy.DefaultConfig()
			}
			policy := autonomy.NewPolicy(policyCfg)

			// Open optional store.
			var st store.Store
			if dbPath != "" {
				s, err := store.New(dbPath)
				if err != nil {
					logger.Warn("could not open database", zap.String("path", dbPath), zap.Error(err))
				} else {
					st = s
					defer func() { _ = s.Close() }()
				}
			}

			// Create log store for structured log persistence (separate DB).
			if dbPath != "" {
				logsDBPath := strings.TrimSuffix(dbPath, ".db") + "-logs.db"
				dispLS, lsErr := logstore.New(logsDBPath)
				if lsErr != nil {
					logger.Warn("could not open log store", zap.String("path", logsDBPath), zap.Error(lsErr))
				} else {
					defer func() { _ = dispLS.Close() }()
					teeLogger, _, teeErr := logging.NewWithTee(dispLS)
					if teeErr != nil {
						logger.Warn("could not create tee logger", zap.Error(teeErr))
					} else {
						logger = teeLogger
					}
					go logstore.StartPruner(ctx, dispLS, 30*24*time.Hour, logger)
					logger.Info("log store enabled", zap.String("path", logsDBPath))
				}
			}

			// Load provider registry and construct agent pool if config exists.
			var pool *agent.Pool
			var providerRegistry *provider.Registry
			if providersConfig != "" {
				reg, regErr := provider.LoadRegistry(providersConfig, makeProviderFactory(logger), logger)
				if regErr != nil {
					logger.Warn("could not load provider registry, agents disabled", zap.String("path", providersConfig), zap.Error(regErr))
				} else {
					providerRegistry = reg
					costs := cost.NewTracker(st, budget, 24)
					pool = agent.NewPool(reg, primaryTracker, st, costs, workers, logger)
					defer pool.Shutdown()
					logger.Info("agent pool started", zap.Int("workers", workers), zap.Int("providers", len(reg.List(ctx))))
				}
			}

			// Wire repo directories for worktree-based workspace isolation.
			// Per-project repo_dir from config takes precedence; the global
			// --repo-dir / SAMVERK_REPO_DIR flag is the fallback default.
			if pool != nil && repoDir == "" {
				repoDir = os.Getenv("SAMVERK_REPO_DIR")
			}
			if pool != nil && repoDir != "" {
				pool.SetRepoDir(repoDir)
				logger.Info("workspace isolation enabled (default)", zap.String("repo_dir", repoDir))
			}
			if pool != nil {
				for _, p := range registry.List() {
					if p.RepoDir != "" {
						pool.SetRepoDirFor(p.Owner, p.Repo, p.RepoDir)
						logger.Info("workspace isolation enabled (project)",
							zap.String("project", p.Name),
							zap.String("repo_dir", p.RepoDir),
						)
					}
				}
			}

			// Wire Synapset memory client if configured (optional, best-effort).
			if pool != nil {
				synCfg := synapset.ConfigFromEnv(synapset.Config{})
				if synCfg.URL != "" {
					synClient := synapset.New(synCfg, logger)
					pool.SetSynapset(synClient)
					logger.Info("synapset memory enabled",
						zap.String("url", synCfg.URL),
						zap.String("pool", synCfg.Pool),
					)
				}
			}

			// Wire repo writer and PR manager from the active project so that
			// pool runners can create branches and open PRs for agent output.
			if pool != nil {
				if activeProj, activeErr := registry.Active(); activeErr == nil {
					if activeProj.Writer != nil {
						pool.SetRepoWriter(activeProj.Writer)
					}
					if activeProj.PRManager != nil {
						pool.SetPRManager(activeProj.PRManager)
					}
					logger.Info("pool forge wiring complete",
						zap.String("project", activeProj.Name),
						zap.Bool("repo_writer", activeProj.Writer != nil),
						zap.Bool("pr_manager", activeProj.PRManager != nil),
					)
				}
			}

			// Build tracker entries from registry for multi-repo dispatch.
			regProjects := registry.List()
			trackerEntries := make([]dispatcher.TrackerEntry, 0, len(regProjects))
			for _, p := range regProjects {
				trackerEntries = append(trackerEntries, dispatcher.TrackerEntry{
					Owner:   p.Owner,
					Repo:    p.Repo,
					Tracker: p.Tracker,
				})
			}
			disp := dispatcher.New(trackerEntries, policy, st, pool, nil, logger)
			disp.SetProjectResolver(registry)

			// Wire Synapset client for doc review enrichment in the dispatcher.
			if synCfg := synapset.ConfigFromEnv(synapset.Config{}); synCfg.URL != "" {
				disp.SetSynapset(synapset.New(synCfg, logger))
			}

			// Wire in-process event bus for pub/sub (event recording, future subscribers).
			bus := eventbus.New(64)
			disp.SetEventBus(bus)

			// Wire WebSocket hub for real-time event broadcasting.
			// The hub runs independently; a future serve+dispatch combined mode
			// or a dedicated WebSocket endpoint can attach clients to it.
			hub := server.NewHub(logger)
			disp.SetBroadcaster(hub)

			// Start health monitor for pre-flight provider health gating.
			if providerRegistry != nil {
				hm := provider.NewHealthMonitor(providerRegistry, provider.DefaultHealthCheckInterval, logger)

				// Wire WoL targets from provider config.
				if providersConfig != "" {
					regCfg, wolErr := provider.LoadRegistryConfig(providersConfig)
					if wolErr == nil {
						wolTargets := make(map[string]provider.WoLConfig)
						for pName := range regCfg.Providers {
							pcfg := regCfg.Providers[pName]
							if pcfg.WoLMAC != "" {
								wolTargets[pName] = provider.WoLConfig{MAC: pcfg.WoLMAC}
							}
						}
						if len(wolTargets) > 0 {
							hm.SetWoLTargets(wolTargets)
							logger.Info("Wake-on-LAN targets configured", zap.Int("count", len(wolTargets)))
						}
					}
				}

				hm.Start(ctx)
				disp.SetHealthMonitor(hm)
				logger.Info("provider health monitor started")

				// Periodically persist health snapshot to SQLite so the serve
				// process (separate OS process) can serve it via the API.
				if st != nil {
					go func() {
						ticker := time.NewTicker(30 * time.Second)
						defer ticker.Stop()
						for {
							select {
							case <-ctx.Done():
								return
							case <-ticker.C:
								if snap := hm.AllHealth(); len(snap) > 0 {
									if saveErr := st.SaveProviderHealthSnapshot(ctx, snap); saveErr != nil {
										logger.Warn("could not save provider health snapshot", zap.Error(saveErr))
									}
								}
							}
						}
					}()
					logger.Info("provider health snapshot writer started")
				}
			}

			// Start management HTTP server for drain endpoints.
			// This runs on the dispatch process so drain reads live state,
			// not stale SQLite snapshots from the serve process.
			drainMux := api.DrainMux(
				func() api.DrainResponse {
					s := disp.Drain()
					return api.DrainResponse{
						Draining:      s.Draining,
						ActiveWorkers: s.ActiveWorkers,
						ClaimedIssues: s.ClaimedIssues,
						QueueDepth:    s.QueueDepth,
					}
				},
				func() api.DrainResponse {
					s := disp.DrainState()
					return api.DrainResponse{
						Draining:      s.Draining,
						ActiveWorkers: s.ActiveWorkers,
						ClaimedIssues: s.ClaimedIssues,
						QueueDepth:    s.QueueDepth,
					}
				},
				func() api.DrainResponse {
					disp.CancelDrain()
					s := disp.DrainState()
					return api.DrainResponse{
						Draining:      s.Draining,
						ActiveWorkers: s.ActiveWorkers,
						ClaimedIssues: s.ClaimedIssues,
						QueueDepth:    s.QueueDepth,
					}
				},
			)
			mgmtSrv := &http.Server{
				Addr:              ":9090",
				Handler:           drainMux,
				ReadHeaderTimeout: 10 * time.Second,
			}

			logger.Info("starting dispatcher", zap.Int("projects", len(trackerEntries)))

			g, gctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				hub.Run(gctx)
				return nil
			})

			g.Go(func() error {
				logger.Info("management server starting", zap.String("addr", mgmtSrv.Addr))
				if err := mgmtSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					return fmt.Errorf("management server: %w", err)
				}
				return nil
			})
			g.Go(func() error {
				<-gctx.Done()
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer shutdownCancel()
				return mgmtSrv.Shutdown(shutdownCtx)
			})

			g.Go(func() error {
				return disp.Run(gctx)
			})

			// Start autoscaler if pool is active and scaling is enabled.
			if pool != nil && scalingEnabled {
				policyCfgS, cfgErr := scaling.LoadConfigFile(scalingConfig)
				if cfgErr != nil {
					logger.Warn("could not load scaling config, using defaults", zap.Error(cfgErr))
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
				autoscaler := scaling.NewAutoscaler(scalingPol, pool, metrics.NewSystemCollector(), logger)
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
				logger.Info("autoscaler started",
					zap.Int("min_workers", policyCfgS.MinWorkers),
					zap.Int("max_workers", policyCfgS.MaxWorkers),
				)
			}

			// Periodically persist pool + dispatcher metrics to SQLite
			// so the serve process can read them via StoreBackedMetrics.
			// NOTE: TasksCompleted and TasksFailed are in-memory counters that
			// reset when the dispatch process restarts. They reflect per-process-
			// lifetime totals, not cumulative all-time counts.
			if st != nil {
				g.Go(func() error {
					tick := time.NewTicker(30 * time.Second)
					defer tick.Stop()
					for {
						select {
						case <-gctx.Done():
							return nil
						case <-tick.C:
							snap := store.MetricSnapshot{
								DispCollectedAt: time.Now(),
								LastPollAt:      time.Now(),
							}
							if pool != nil {
								ps := pool.Snapshot()
								snap.CollectedAt = ps.CollectedAt
								snap.TotalWorkers = ps.TotalWorkers
								snap.ActiveWorkers = ps.ActiveWorkers
								snap.IdleWorkers = ps.IdleWorkers
								snap.QueueDepth = ps.QueueDepth
								snap.TasksCompleted = ps.TasksCompleted
								snap.TasksFailed = ps.TasksFailed
								snap.AvgTaskDurationMs = float64(ps.AvgTaskDuration.Nanoseconds()) / 1e6
								snap.P95TaskDurationMs = float64(ps.P95TaskDuration.Nanoseconds()) / 1e6
							} else {
								snap.CollectedAt = time.Now()
							}
							ds := disp.Snapshot()
							snap.DispCollectedAt = ds.CollectedAt
							snap.ClaimedCount = ds.ClaimedCount
							snap.TotalRouted = ds.TotalRouted
							snap.TotalRequeued = ds.TotalRequeued
							snap.TotalEventsProcessed = ds.TotalEventsProcessed
							snap.AvgPollLatencyMs = float64(ds.AvgPollLatency.Nanoseconds()) / 1e6
							snap.LastPollAt = ds.LastPollAt
							if err := st.SaveMetricSnapshot(gctx, snap); err != nil {
								logger.Warn("save metric snapshot", zap.Error(err))
							}
						}
					}
				})
				logger.Info("metric snapshot writer started (30s interval)")
			}

			// Start PR watcher if auto-merge is enabled.
			if policyCfg.Merge.AutoMergeOnCIPass {
				if primaryPRMgr == nil {
					return fmt.Errorf("auto_merge_on_ci_pass is enabled but PR manager failed to initialize")
				}
				var pwOpts []prwatcher.Option
				if st != nil {
					projectName := ""
					if ap, apErr := registry.Active(); apErr == nil {
						projectName = ap.Name
					}
					pwOpts = append(pwOpts, prwatcher.WithIssueCache(st, projectName))
				}
				pw := prwatcher.New(primaryPRMgr, primaryTracker, policyCfg.Merge, time.Duration(pollSeconds)*time.Second, logger, pwOpts...)
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

	cmd.Flags().StringVar(&owner, "owner", "", "Repository owner")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&forgeName, "forge", "github", "Forge type: github or gitea")
	cmd.Flags().StringVar(&giteaURL, "gitea-url", "", "Gitea instance URL (required when --forge=gitea)")
	cmd.Flags().StringVar(&dbPath, "db", ".samverk/samverk.db", "Path to SQLite database")
	cmd.Flags().StringVar(&providersConfig, "providers-config", ".samverk/providers.yaml", "Path to provider registry YAML config")
	cmd.Flags().IntVar(&pollSeconds, "poll-interval", 30, "Polling interval in seconds")
	cmd.Flags().IntVar(&workers, "workers", 3, "Number of agent worker goroutines")
	cmd.Flags().Float64Var(&budget, "budget", 0, "Daily budget in USD (0 = unlimited)")
	cmd.Flags().BoolVar(&scalingEnabled, "scaling", false, "Enable adaptive worker scaling (default: disabled)")
	cmd.Flags().StringVar(&scalingConfig, "scaling-config", "", "Path to scaling policy YAML config (optional)")
	cmd.Flags().IntVar(&scalingMin, "scaling-min", 0, "Override min workers from scaling config (0 = use config value)")
	cmd.Flags().IntVar(&scalingMax, "scaling-max", 0, "Override max workers from scaling config (0 = use config value)")
	cmd.Flags().StringVar(&projectsConfig, "projects-config", ".samverk/server.yaml", "Path to projects YAML config (multi-repo dispatch)")
	cmd.Flags().StringVar(&repoDir, "repo-dir", "", "Local git clone path for agent worktree isolation (env: SAMVERK_REPO_DIR)")

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
					logger.Warn("could not open cost database, continuing without cost data", zap.String("path", dbPath), zap.Error(storeErr))
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
	var name, authKeysPath, scope, workerID string
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

			var plaintext string
			if scope != "" || workerID != "" {
				plaintext, err = ks.CreateScoped(name, projects, scope, workerID)
			} else {
				plaintext, err = ks.Create(name, projects)
			}
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
	cmd.Flags().StringVar(&scope, "scope", "", "Key scope: admin (default) or worker")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Worker ID (required when scope=worker)")

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

			fmt.Printf("%-20s %-10s %-15s %-25s %s\n", "NAME", "SCOPE", "WORKER_ID", "PROJECTS", "CREATED")
			fmt.Printf("%-20s %-10s %-15s %-25s %s\n", "----", "-----", "---------", "--------", "-------")
			for i := range keys {
				proj := "(all)"
				if len(keys[i].Projects) > 0 {
					proj = fmt.Sprintf("%v", keys[i].Projects)
				}
				scope := keys[i].Scope
				if scope == "" {
					scope = "admin"
				}
				wid := keys[i].WorkerID
				if wid == "" {
					wid = "-"
				}
				fmt.Printf("%-20s %-10s %-15s %-25s %s\n", keys[i].Name, scope, wid, proj, keys[i].CreatedAt)
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

func statusCmd() *cobra.Command {
	var owner, repo, healthURL string
	var write bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show or update project status from issue tracker",
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

			ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
			httpClient := oauth2.NewClient(context.Background(), ts)
			tracker := github.New(owner, repo, httpClient)

			w := status.New(tracker, healthURL)
			content, err := w.Generate(context.Background())
			if err != nil {
				return fmt.Errorf("generating status: %w", err)
			}

			if write {
				if writeErr := os.WriteFile(".samverk/status.md", []byte(content), 0o600); writeErr != nil {
					return fmt.Errorf("writing status.md: %w", writeErr)
				}
				fmt.Println("Updated .samverk/status.md")
			} else {
				fmt.Print(content)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "GitHub repository owner")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repository name")
	cmd.Flags().BoolVar(&write, "write", false, "Write to .samverk/status.md instead of stdout")
	cmd.Flags().StringVar(&healthURL, "health-url", "http://localhost:8080/healthz", "Health check URL")

	return cmd
}

func wakeCmd() *cobra.Command {
	var providersConfigWake string

	cmd := &cobra.Command{
		Use:   "wake <provider-name>",
		Short: "Send a Wake-on-LAN packet to a provider's host",
		Long:  "Reads the wol_mac field from the provider's YAML config and sends a magic packet.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			providerName := args[0]

			cfg, err := provider.LoadRegistryConfig(providersConfigWake)
			if err != nil {
				return fmt.Errorf("load providers config: %w", err)
			}

			pcfg, ok := cfg.Providers[providerName]
			if !ok {
				return fmt.Errorf("provider %q not found in config", providerName)
			}
			if pcfg.WoLMAC == "" {
				return fmt.Errorf("provider %q has no wol_mac configured", providerName)
			}

			fmt.Printf("Sending Wake-on-LAN packet to %s (MAC: %s)...\n", providerName, pcfg.WoLMAC)
			if err := provider.WakeOnLAN(pcfg.WoLMAC); err != nil {
				return fmt.Errorf("wake-on-LAN failed: %w", err)
			}
			fmt.Println("Magic packet sent successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&providersConfigWake, "providers-config", ".samverk/providers.yaml", "Path to provider registry YAML config")
	return cmd
}

func docAuditCmd() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:   "doc-audit",
		Short: "Detect documentation drift (stale docs, broken links, missing files)",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := docaudit.New(repoRoot)
			findings, err := a.Run()
			if err != nil {
				return fmt.Errorf("doc audit: %w", err)
			}

			if len(findings) == 0 {
				_, _ = fmt.Fprintln(os.Stdout, "No documentation issues found.")
				return nil
			}

			// Print findings as a table.
			_, _ = fmt.Fprintf(os.Stdout, "%-8s | %-40s | %s\n", "SEVERITY", "FILE", "MESSAGE")
			_, _ = fmt.Fprintf(os.Stdout, "%-8s | %-40s | %s\n", "--------", "----------------------------------------", "-------")
			hasError := false
			for i := range findings {
				f := &findings[i]
				loc := f.File
				if f.Line > 0 {
					loc = fmt.Sprintf("%s:%d", f.File, f.Line)
				}
				_, _ = fmt.Fprintf(os.Stdout, "%-8s | %-40s | %s\n", string(f.Severity), loc, f.Message)
				if f.Severity == docaudit.SeverityError {
					hasError = true
				}
			}

			if hasError {
				return fmt.Errorf("found %d documentation issue(s)", len(findings))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoRoot, "root", ".", "Repository root directory")
	return cmd
}

func probeCmd() *cobra.Command {
	var synapsetURL string

	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Run infrastructure probe and sync to Synapset",
		Long:  "Polls known endpoints (Ollama, Samverk, Gitea) and updates Synapset machine memories with meaningful changes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			cfg := synapset.ConfigFromEnv(synapset.Config{URL: synapsetURL})
			sc := synapset.New(cfg, logger)

			endpoints := infra.DefaultEndpoints()
			prober := infra.NewProber(sc, logger, endpoints)

			results := prober.Run(ctx)

			// Print summary.
			var reachable, unreachable int
			for i := range results {
				if results[i].Reachable {
					reachable++
				} else {
					unreachable++
				}
			}
			fmt.Printf("Probe complete: %d reachable, %d unreachable (of %d endpoints)\n",
				reachable, unreachable, len(results))
			return nil
		},
	}

	cmd.Flags().StringVar(&synapsetURL, "synapset-url", "", "Synapset MCP endpoint URL (default: env or "+synapset.DefaultURL+")")
	return cmd
}

func backfillLabelsCmd() *cobra.Command {
	var projectsConfig string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "backfill-labels",
		Short: "Backfill qc:pass/fail/review labels on pre-existing issues (one-time migration)",
		Long: `Scans open issues for QC verdict comments and adds the corresponding
label (qc:pass, qc:fail, qc:review). Processes issues one at a time to
limit memory usage. Safe to re-run: skips issues that already have a qc: label.

WARNING: This command calls ListComments per issue. On large repos, run
from a local machine (not CT 202) or set GOMEMLIMIT to cap RSS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			if projectsConfig == "" {
				return fmt.Errorf("--projects is required")
			}

			configs, err := internalmcp.LoadProjectConfig(projectsConfig)
			if err != nil {
				return fmt.Errorf("load project config: %w", err)
			}

			for i := range configs {
				pc := &configs[i]
				var tracker forge.IssueTracker
				if pc.Forge == "gitea" {
					giteaToken := pc.GiteaToken
					if giteaToken == "" {
						giteaToken = os.Getenv("GITEA_TOKEN")
					}
					gt, gtErr := giteaadapter.New(pc.GiteaURL, giteaToken, pc.Owner, pc.Repo)
					if gtErr != nil {
						logger.Warn("skip project", zap.String("name", pc.Name), zap.Error(gtErr))
						continue
					}
					tracker = gt
				} else {
					logger.Warn("skip non-gitea project", zap.String("name", pc.Name))
					continue
				}

				issues, listErr := tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateOpen})
				if listErr != nil {
					logger.Warn("list issues failed", zap.String("project", pc.Name), zap.Error(listErr))
					continue
				}

				fmt.Printf("%s: checking %d open issues...\n", pc.Name, len(issues))
				backfilled := 0
				for _, issue := range issues {
					hasQCLabel := false
					for _, l := range issue.Labels {
						if l == models.LabelQcPass || l == models.LabelQcFail || l == models.LabelQcReview {
							hasQCLabel = true
							break
						}
					}
					if hasQCLabel {
						continue
					}

					label := backfillScanIssue(ctx, tracker, issue.Number)
					if label == "" {
						continue
					}

					if dryRun {
						fmt.Printf("  #%d: would add %s\n", issue.Number, label)
						backfilled++
						continue
					}

					if addErr := tracker.AddLabels(ctx, issue.Number, label); addErr == nil {
						backfilled++
						fmt.Printf("  #%d: added %s\n", issue.Number, label)
					} else {
						fmt.Printf("  #%d: FAILED to add %s: %v\n", issue.Number, label, addErr)
					}
				}

				action := "backfilled"
				if dryRun {
					action = "would backfill"
				}
				fmt.Printf("%s: %s %d issues out of %d open\n", pc.Name, action, backfilled, len(issues))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&projectsConfig, "projects", "", "Path to projects.yaml config")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be done without making changes")
	return cmd
}

// backfillScanIssue checks a single issue's comments for a QC verdict and
// returns the label to apply (empty string if none found). Comments are
// scanned in reverse order (most recent first) and discarded after the scan
// to limit memory pressure.
func backfillScanIssue(ctx context.Context, tracker forge.IssueTracker, issueNumber int) string {
	comments, err := tracker.ListComments(ctx, issueNumber)
	if err != nil || len(comments) == 0 {
		return ""
	}

	// Scan from newest to oldest -- the most recent verdict wins.
	for i := len(comments) - 1; i >= 0; i-- {
		body := comments[i].Body
		switch {
		case strings.Contains(body, "**QC Review: [PASS]**"):
			return models.LabelQcPass
		case strings.Contains(body, "**QC Review: [FAIL]**"):
			return models.LabelQcFail
		case strings.Contains(body, "**QC Review: [REVIEW]**"):
			return models.LabelQcReview
		}
	}
	return ""
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

// makeProviderFactory returns a ProviderFactory closure that constructs
// provider.Provider instances from YAML config, wiring in the logger.
func makeProviderFactory(l *zap.Logger) provider.ProviderFactory {
	return func(name string, cfg provider.ProviderConfig) (provider.Provider, error) {
		plog := l.Named("provider." + name)
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
			return claude.New(apiKey, model, claude.WithLogger(plog)), nil
		case "openai":
			apiKey := os.Getenv(cfg.APIKeyEnv)
			if apiKey == "" {
				return nil, fmt.Errorf("provider %q: env var %s is not set", name, cfg.APIKeyEnv)
			}
			model := cfg.DefaultModel
			if model == "" {
				model = "gpt-4o"
			}
			return openai.New(apiKey, model, openai.WithLogger(plog)), nil
		case "ollama":
			baseURL := cfg.BaseURL
			if baseURL == "" {
				baseURL = "http://localhost:11434"
			}
			if cfg.TimeoutSeconds > 0 {
				return ollama.NewWithTimeout(baseURL, time.Duration(cfg.TimeoutSeconds)*time.Second, ollama.WithLogger(plog)), nil
			}
			return ollama.New(baseURL, ollama.WithLogger(plog)), nil
		case "claude-cli":
			if cfg.MaxTurns < 0 {
				return nil, fmt.Errorf("provider %q: max_turns must be non-negative, got %d", name, cfg.MaxTurns)
			}
			model := cfg.DefaultModel
			opts := claudecli.Options{
				AllowedTools: normalizeCSV(cfg.AllowedTools),
				MaxTurns:     cfg.MaxTurns,
				BaseURL:      cfg.BaseURL,
				Logger:       plog,
			}
			if cfg.TimeoutSeconds > 0 {
				return claudecli.NewWithTimeout(model, time.Duration(cfg.TimeoutSeconds)*time.Second, opts), nil
			}
			return claudecli.New(model, opts), nil
		default:
			return nil, fmt.Errorf("provider %q: unknown type %q", name, cfg.Type)
		}
	}
}

// normalizeCSV trims whitespace around comma-separated values to prevent
// incidental YAML whitespace from causing invalid tool names.
func normalizeCSV(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return strings.Join(parts, ",")
}
