package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/herbhall/samverk/internal/audit"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Run diagnostic audits",
	}

	cmd.AddCommand(auditProvidersCmd())
	return cmd
}

func auditProvidersCmd() *cobra.Command {
	var providersConfigAudit, dbPath string

	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Health-check all configured providers",
		Long:  "Runs a health check against every provider in the registry and prints a summary table. Exits with code 1 if any provider is unhealthy.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Load provider registry.
			reg, err := provider.LoadRegistry(providersConfigAudit, makeProviderFactory(logger), logger)
			if err != nil {
				return fmt.Errorf("load provider registry: %w", err)
			}

			// Collect provider info and instances.
			infos := reg.List(ctx)
			provs := make([]provider.Provider, len(infos))
			for i := range infos {
				p := reg.ProviderByName(infos[i].Name)
				if p == nil {
					return fmt.Errorf("provider %q registered but not found", infos[i].Name)
				}
				provs[i] = p
			}

			// Run audit.
			pa := audit.New(infos, provs)
			results, err := pa.Run(ctx)
			if err != nil {
				return fmt.Errorf("audit run: %w", err)
			}

			// Save to SQLite if available.
			if dbPath != "" {
				s, storeErr := store.New(dbPath)
				if storeErr != nil {
					logger.Warn("could not open database, skipping persistence",
						zap.String("path", dbPath), zap.Error(storeErr))
				} else {
					defer func() { _ = s.Close() }()
					if saveErr := s.SaveAuditResults(ctx, results); saveErr != nil {
						logger.Warn("could not save audit results", zap.Error(saveErr))
					} else {
						logger.Info("audit results saved to database", zap.String("path", dbPath))
					}
				}
			}

			// Print results table.
			printAuditTable(results)

			// Exit code 1 if any unhealthy.
			for i := range results {
				if !results[i].Healthy {
					return fmt.Errorf("one or more providers are unhealthy")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&providersConfigAudit, "providers-config", ".samverk/providers.yaml", "Path to provider registry YAML config")
	cmd.Flags().StringVar(&dbPath, "db", ".samverk/samverk.db", "Path to SQLite database (empty to skip persistence)")
	return cmd
}

func printAuditTable(results []audit.AuditResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tTYPE\tMODEL\tHEALTHY\tLATENCY\tERROR")
	fmt.Fprintln(w, "--------\t----\t-----\t-------\t-------\t-----")
	for i := range results {
		r := &results[i]
		healthy := "yes"
		if !r.Healthy {
			healthy = "NO"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ProviderName,
			r.ProviderType,
			r.Model,
			healthy,
			r.Latency.Round(1_000_000).String(), // round to nearest ms
			r.Error,
		)
	}
	_ = w.Flush()
}
