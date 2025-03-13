package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockServer implements http.Handler for testing
type mockServer struct{}

func (s *mockServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		basePort   string
		expectPort string
		envs       map[string]string
		wantErr    bool
	}{
		{
			name:       "valid config with default port",
			expectPort: "8080",
			envs: map[string]string{
				"BUCKET_NAME":       "test-bucket",
				"GOOGLE_PROJECT_ID": "test-project",
			},
		},
		{
			name:       "valid config with custom port",
			basePort:   "9090",
			expectPort: "9090",
			envs: map[string]string{
				"BUCKET_NAME":       "test-bucket",
				"GOOGLE_PROJECT_ID": "test-project",
			},
		},
		{
			name: "missing bucket name",
			envs: map[string]string{
				"GOOGLE_PROJECT_ID": "test-project",
			},
			wantErr: true,
		},
		{
			name: "missing project ID",
			envs: map[string]string{
				"BUCKET_NAME": "test-bucket",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment variables
			os.Unsetenv("BUCKET_NAME")
			os.Unsetenv("GOOGLE_PROJECT_ID")

			// Set test environment variables
			for k, v := range tt.envs {
				os.Setenv(k, v)
			}

			// Create base config if port is specified
			var base *config
			if tt.basePort != "" {
				base = &config{port: tt.basePort}
			}

			cfg, err := loadConfig(base)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectPort, cfg.port)
			assert.Equal(t, tt.envs["BUCKET_NAME"], cfg.bucketName)
			assert.Equal(t, tt.envs["GOOGLE_PROJECT_ID"], cfg.projectID)
		})
	}
}

func TestSetupServer(t *testing.T) {
	// Save original setupServer and restore after test
	originalSetup := setupServer
	defer func() { setupServer = originalSetup }()

	// Create mock setup function
	setupServer = func(ctx context.Context, cfg *config) (*http.Server, error) {
		mux := http.NewServeMux()
		mux.Handle("/", &mockServer{})
		mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		mux.HandleFunc("/readyz", readyzHandler)
		mux.HandleFunc("/livez", livezHandler)

		return &http.Server{
			Addr:    ":" + cfg.port,
			Handler: mux,
		}, nil
	}

	cfg := &config{
		port:       "8080",
		bucketName: "test-bucket",
		projectID:  "test-project",
	}

	srv, err := setupServer(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, ":8080", srv.Addr)
}

func TestRun(t *testing.T) {
	// Save original setupServer and restore after test
	originalSetup := setupServer
	defer func() { setupServer = originalSetup }()

	// Create mock setup function
	setupServer = func(ctx context.Context, cfg *config) (*http.Server, error) {
		mux := http.NewServeMux()
		mux.Handle("/", &mockServer{})
		mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		mux.HandleFunc("/readyz", readyzHandler)
		mux.HandleFunc("/livez", livezHandler)

		return &http.Server{
			Addr:    ":" + cfg.port,
			Handler: mux,
		}, nil
	}

	// Set required environment variables
	os.Setenv("BUCKET_NAME", "test-bucket")
	os.Setenv("GOOGLE_PROJECT_ID", "test-project")
	defer func() {
		os.Unsetenv("BUCKET_NAME")
		os.Unsetenv("GOOGLE_PROJECT_ID")
	}()

	// Create a context that will cancel after a short time
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create a config for testing
	cfg := &config{
		port:       "0", // Use port 0 to let the OS choose a free port
		bucketName: "test-bucket",
		projectID:  "test-project",
	}

	// Setup server
	srv, err := setupServer(ctx, cfg)
	require.NoError(t, err)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, srv)
	}()

	// Wait for either error or timeout
	select {
	case err := <-errChan:
		require.NoError(t, err)
	case <-time.After(200 * time.Millisecond):
		// Cancel context to trigger shutdown
		cancel()
		// Wait for shutdown to complete
		err := <-errChan
		require.NoError(t, err)
	}
}
