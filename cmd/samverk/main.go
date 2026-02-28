package main

import (
	"fmt"
	"os"

	"github.com/herbhall/samverk/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "samverk",
		Short: "Async background development engine",
		Long:  "Samverk keeps side projects building while you live your life.",
	}

	root.AddCommand(serveCmd())
	root.AddCommand(dispatchCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server (MCP + API + dashboard)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("samverk serve: not yet implemented")
		},
	}
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
