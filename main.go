package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/storage"
	"github.com/spf13/cobra"
)

// Version can be set at build time using -ldflags "-X main.Version=x.y.z"
var Version = "dev"

// storageClientFactory is a variable that can be replaced in tests
var storageClientFactory = func(ctx context.Context) (StorageClient, error) {
	return storage.NewClient(ctx)
}

// loggingClientFactory is a variable that can be replaced in tests
var loggingClientFactory = func(ctx context.Context, projectID string) (LoggingClient, error) {
	return createLoggingClient(ctx, projectID)
}

func main() {
	ctx := context.Background()

	var port string

	rootCmd := &cobra.Command{
		Use:   "spray",
		Short: "Spray is a GCS static file server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return startServer(ctx, port)
		},
	}

	// Add version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Spray version %s\n", Version)
		},
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.Flags().StringVar(&port, "port", "8080", "Server port")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// startServer handles the server initialization and startup
func startServer(ctx context.Context, port string) error {
	// Initialize logging
	logClient, err := loggingClientFactory(ctx, os.Getenv("GOOGLE_PROJECT_ID"))
	if err != nil {
		return fmt.Errorf("failed to create logging client: %v", err)
	}
	defer logClient.Close()

	// Load initial config without store to get bucket name
	cfg, err := loadConfig(ctx, &config{port: port}, nil)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Create storage client
	storageClient, err := storageClientFactory(ctx)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %v", err)
	}
	defer storageClient.Close()

	// Create store
	store := &GCSObjectStore{
		bucket: storageClient.Bucket(cfg.bucketName),
	}

	// Reload config with store to get redirects
	cfg, err = loadConfig(ctx, cfg, store)
	if err != nil {
		return fmt.Errorf("failed to load config with redirects: %v", err)
	}

	// Create server
	srv, err := createServer(ctx, cfg, logClient)
	if err != nil {
		return fmt.Errorf("failed to create server: %v", err)
	}

	return runServerImpl(ctx, srv)
}

// RunApp contains the main orchestration logic and is testable.
func RunApp(ctx context.Context, port string) error {
	// Load initial config without store to get bucket name
	cfg, err := loadConfig(ctx, &config{port: port}, nil)
	if err != nil {
		return err
	}

	logClient, err := loggingClientFactory(ctx, cfg.projectID)
	if err != nil {
		return err
	}
	defer logClient.Close()

	// Create storage client
	storageClient, err := storageClientFactory(ctx)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %v", err)
	}
	defer storageClient.Close()

	// Create store
	store := &GCSObjectStore{
		bucket: storageClient.Bucket(cfg.bucketName),
	}

	// Reload config with store to get redirects
	cfg, err = loadConfig(ctx, cfg, store)
	if err != nil {
		return fmt.Errorf("failed to load config with redirects: %v", err)
	}

	srv, err := DefaultServerSetup(ctx, cfg, logClient)
	if err != nil {
		return err
	}

	return runServerImpl(ctx, srv)
}
