package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge/github"
	"github.com/herbhall/samverk/internal/server"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

func main() {
	root := &cobra.Command{
		Use:   "samverk",
		Short: "Async background development engine",
		Long:  "Samverk keeps side projects building while you live your life.",
	}

	root.AddCommand(serveCmd())
	root.AddCommand(dispatchCmd())
	root.AddCommand(digestCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server (MCP + API + dashboard)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			s := server.New(server.Config{Addr: addr})
			slog.Info("starting samverk server", "addr", addr)

			if err := s.Start(ctx); err != nil {
				return fmt.Errorf("server error: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	return cmd
}

func dispatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dispatch",
		Short: "Start the dispatcher agent (watches issue tracker, routes work)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("samverk dispatch: not yet implemented")
		},
	}
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
