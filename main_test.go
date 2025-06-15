package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	// Save original setupServer and restore after test
	originalSetup := DefaultServerSetup
	defer func() { DefaultServerSetup = originalSetup }()

	// Create a mock server setup function
	DefaultServerSetup = func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
		// Create a mock GCS server
		objects := make(map[string]mockObject)
		server := createMockServer(t, objects, nil)

		mux := http.NewServeMux()
		mux.Handle("/", server)
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/readyz", readyzHandler)
		mux.HandleFunc("/livez", livezHandler)

		return &http.Server{
			Addr:    ":" + cfg.port,
			Handler: mux,
		}, nil
	}

	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock config
	cfg := &config{
		port:       "8080",
		bucketName: "test-bucket",
		projectID:  "test-project",
	}

	// Create a mock logging client
	logClient := newMockLogClient()

	// Create the server first
	srv, err := DefaultServerSetup(ctx, cfg, logClient)
	require.NoError(t, err)

	// Run the server in a goroutine
	go func() {
		err := runServer(ctx, srv)
		assert.NoError(t, err)
	}()

	// Wait a bit for the server to start
	time.Sleep(100 * time.Millisecond)

	// Test the health check endpoints
	resp, err := http.Get("http://localhost:8080/readyz")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:8080/livez")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Cancel the context to stop the server
	cancel()

	// Wait a bit for the server to stop
	time.Sleep(100 * time.Millisecond)
}

func TestRunApp_Errors(t *testing.T) {
	// Save original client factories and server setup functions
	originalStorageClientFactory := storageClientFactory
	originalLoggingClientFactory := loggingClientFactory
	originalDefaultServerSetup := DefaultServerSetup
	defer func() {
		storageClientFactory = originalStorageClientFactory
		loggingClientFactory = originalLoggingClientFactory
		DefaultServerSetup = originalDefaultServerSetup
	}()

	// Mock storage client factory to avoid Google Cloud credentials issues
	storageClientFactory = func(ctx context.Context) (StorageClient, error) {
		return &mockStorageClient{objects: make(map[string]mockObject)}, nil
	}

	// Mock logging client factory to return an error
	loggingClientFactory = func(ctx context.Context, projectID string) (LoggingClient, error) {
		return nil, assert.AnError
	}

	// Mock server setup to return an error
	DefaultServerSetup = func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
		return nil, assert.AnError
	}

	// Set a valid bucket name for all tests
	os.Setenv("BUCKET_NAME", "test-bucket")
	defer os.Unsetenv("BUCKET_NAME")

	// Test missing GOOGLE_PROJECT_ID
	os.Unsetenv("GOOGLE_PROJECT_ID")
	err := RunApp(context.Background(), "8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GOOGLE_PROJECT_ID environment variable is required")

	// Test logging client factory error
	os.Setenv("GOOGLE_PROJECT_ID", "test-project")
	err = RunApp(context.Background(), "8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), assert.AnError.Error())

	// Test server setup error
	loggingClientFactory = func(ctx context.Context, projectID string) (LoggingClient, error) {
		return &mockLogClient{}, nil
	}
	err = RunApp(context.Background(), "8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), assert.AnError.Error())

	// Reset flag.CommandLine to avoid test interference
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}
