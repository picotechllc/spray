package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

func main() {
	var port string

	rootCmd := &cobra.Command{
		Use:   "spray",
		Short: "Spray is a GCS static file server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			return RunApp(ctx, port)
		},
	}

	rootCmd.Flags().StringVar(&port, "port", "8080", "Server port")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// RunApp contains the main orchestration logic and is testable.
func RunApp(ctx context.Context, port string) error {
	cfg, err := loadConfig(&config{port: port})
	if err != nil {
		return err
	}

	logClient, err := loggingClientFactory(ctx, cfg.projectID)
	if err != nil {
		return err
	}
	defer logClient.Close()

	srv, err := DefaultServerSetup(ctx, cfg, logClient)
	if err != nil {
		return err
	}

	return runServer(ctx, srv)
}
