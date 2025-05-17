package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

func TestRun(t *testing.T) {
	// Save original setupServer and restore after test
	originalSetup := DefaultServerSetup
	defer func() { DefaultServerSetup = originalSetup }()

	// Create a mock server setup function
	DefaultServerSetup = func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
		logger := logClient.Logger("test-logger")

		// Create a mock GCS server
		mockStore := &mockObjectStore{
			objects: make(map[string]mockObject),
		}

		server := &gcsServer{
			store:      mockStore,
			bucketName: cfg.bucketName,
			logger:     logger,
		}

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
	logClient, err := logging.NewClient(ctx, "test-project", option.WithoutAuthentication())
	require.NoError(t, err)
	defer logClient.Close()

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
	ctx := context.Background()

	// --- loadConfig error: missing BUCKET_NAME ---
	os.Unsetenv("BUCKET_NAME")
	os.Setenv("GOOGLE_PROJECT_ID", "test-project")
	err := RunApp(ctx, "8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "BUCKET_NAME environment variable is required")

	// --- loadConfig error: missing GOOGLE_PROJECT_ID ---
	os.Setenv("BUCKET_NAME", "test-bucket")
	os.Unsetenv("GOOGLE_PROJECT_ID")
	err = RunApp(ctx, "8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GOOGLE_PROJECT_ID environment variable is required")

	// --- loggingClientFactory error ---
	os.Setenv("BUCKET_NAME", "test-bucket")
	os.Setenv("GOOGLE_PROJECT_ID", "test-project")
	origLoggingClientFactory := loggingClientFactory
	loggingClientFactory = func(ctx context.Context, projectID string) (*logging.Client, error) {
		return nil, assert.AnError
	}
	t.Cleanup(func() { loggingClientFactory = origLoggingClientFactory })
	err = RunApp(ctx, "8080")
	assert.ErrorIs(t, err, assert.AnError)

	// Restore loggingClientFactory for the rest of the test
	loggingClientFactory = origLoggingClientFactory

	// Save originals
	origDefaultServerSetup := DefaultServerSetup
	origRunServer := runServer

	t.Cleanup(func() {
		DefaultServerSetup = origDefaultServerSetup
		runServer = origRunServer
	})

	// --- Server setup error ---
	DefaultServerSetup = func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
		return nil, assert.AnError
	}
	err = RunApp(ctx, "8080")
	assert.ErrorIs(t, err, assert.AnError)

	// --- Server run error ---
	DefaultServerSetup = origDefaultServerSetup
	runServer = func(ctx context.Context, srv *http.Server) error {
		return assert.AnError
	}

	// Use a valid config and logging client
	DefaultServerSetup = func(ctx context.Context, c *config, l *logging.Client) (*http.Server, error) {
		return &http.Server{}, nil
	}

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	err = RunApp(ctx, "8080")
	assert.ErrorIs(t, err, assert.AnError)
}
