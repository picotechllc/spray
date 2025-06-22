package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
)

// Version can be set at build time using -ldflags "-X main.Version=x.y.z"
var Version = "dev"

// mockStorageClient for debugging - implements StorageClient interface
type debugMockStorageClient struct {
	objects map[string]debugMockObject
}

type debugMockObject struct {
	data        []byte
	contentType string
}

func (c *debugMockStorageClient) Bucket(name string) *storage.BucketHandle {
	// Return a fake bucket handle - we'll override GetObject anyway
	return &storage.BucketHandle{}
}

func (c *debugMockStorageClient) Close() error {
	return nil
}

// debugMockObjectStore for debugging - implements ObjectStore interface
type debugMockObjectStore struct {
	objects    map[string]debugMockObject
	bucketName string
}

func (s *debugMockObjectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	if obj, ok := s.objects[path]; ok {
		return io.NopCloser(strings.NewReader(string(obj.data))), &storage.ObjectAttrs{
			ContentType: obj.contentType,
			Size:        int64(len(obj.data)),
		}, nil
	}
	return nil, nil, storage.ErrObjectNotExist
}

// storageClientFactory is a variable that can be replaced in tests
var storageClientFactory = func(ctx context.Context) (StorageClient, error) {
	// Check if we should use mock storage for debugging
	if os.Getenv("STORAGE_MOCK") == "true" {
		return &debugMockStorageClient{
			objects: make(map[string]debugMockObject),
		}, nil
	}

	// Check for common indicators that we're in an environment without proper credentials
	// If we detect this upfront, skip the authenticated client attempt entirely
	hasCredentials := false

	// Check for explicit service account credentials
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
		hasCredentials = true
	}

	// Check for GCE metadata server (indicates we're running on GCP with service account)
	if os.Getenv("GCE_METADATA_HOST") != "" ||
		os.Getenv("METADATA_SERVER_ADDRESS") != "" ||
		os.Getenv("GCE_METADATA_IP") != "" {
		hasCredentials = true
	}

	// Check for Kubernetes service account token (GKE Workload Identity)
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		hasCredentials = true
	}

	// Check for common GCP environment indicators
	if os.Getenv("GOOGLE_PROJECT_ID") != "" ||
		os.Getenv("GCP_PROJECT") != "" ||
		os.Getenv("GOOGLE_CLOUD_PROJECT") != "" ||
		os.Getenv("GCLOUD_PROJECT") != "" {
		hasCredentials = true
	}

	// If no clear indicators of credentials, try unauthenticated first for public buckets
	if !hasCredentials {
		log.Printf("No clear credential indicators found, attempting unauthenticated client for public bucket access...")
		client, err := storage.NewClient(ctx, option.WithoutAuthentication())
		if err == nil {
			log.Printf("Successfully created unauthenticated storage client")
			return client, nil
		}
		log.Printf("Unauthenticated client failed: %v, falling back to authenticated client", err)
	}

	// Try to create an authenticated client first (or as fallback)
	client, err := storage.NewClient(ctx)
	if err != nil {
		// Preserve the original error for reporting
		originalErr := err

		// If authentication fails, try creating an unauthenticated client
		// This is useful for public buckets where authentication isn't required
		log.Printf("Failed to create authenticated storage client: %v", originalErr)

		// Check if this looks like a credential issue that might work with unauthenticated access
		errStr := originalErr.Error()
		if strings.Contains(errStr, "metadata") ||
			strings.Contains(errStr, "credential") ||
			strings.Contains(errStr, "token") ||
			strings.Contains(errStr, "authentication") {
			log.Printf("Attempting unauthenticated client for public bucket access...")

			client, err = storage.NewClient(ctx, option.WithoutAuthentication())
			if err != nil {
				return nil, fmt.Errorf("failed to create both authenticated and unauthenticated storage clients. Authenticated error: %v, Unauthenticated error: %v", originalErr, err)
			}
			log.Printf("Successfully created unauthenticated storage client for public bucket access")
		} else {
			return nil, fmt.Errorf("failed to create authenticated storage client (non-credential error): %v", originalErr)
		}
	}

	return client, nil
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

	// Log startup message
	log.Printf("Spray version %s starting up on port %s", Version, port)

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
	var store ObjectStore
	if os.Getenv("STORAGE_MOCK") == "true" {
		// Use mock store for debugging
		mockStore := &debugMockObjectStore{
			objects:    make(map[string]debugMockObject),
			bucketName: cfg.bucketName,
		}
		// Add some sample objects for testing
		mockStore.objects["index.html"] = debugMockObject{
			data:        []byte("<html><body><h1>Mock Index Page</h1></body></html>"),
			contentType: "text/html",
		}
		mockStore.objects["test.txt"] = debugMockObject{
			data:        []byte("This is a test file"),
			contentType: "text/plain",
		}
		store = mockStore
	} else {
		store = &GCSObjectStore{
			bucket: storageClient.Bucket(cfg.bucketName),
		}
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

	// Log startup message
	log.Printf("Spray version %s starting up on port %s", Version, port)

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
