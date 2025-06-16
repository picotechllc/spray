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
