package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
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

func TestServerSetup(t *testing.T) {
	tests := []struct {
		name      string
		setup     ServerSetup
		cfg       *config
		wantPort  string
		wantPaths []string
		wantErr   bool
	}{
		{
			name: "successful setup",
			setup: func(ctx context.Context, cfg *config) (*http.Server, error) {
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
			},
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			wantPort: ":8080",
			wantPaths: []string{
				"/",
				"/metrics",
				"/readyz",
				"/livez",
			},
			wantErr: false,
		},
		{
			name: "setup error",
			setup: func(ctx context.Context, cfg *config) (*http.Server, error) {
				return nil, fmt.Errorf("mock setup error")
			},
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original setupServer and restore after test
			originalSetup := setupServer
			defer func() { setupServer = originalSetup }()

			// Set test setup function
			setupServer = tt.setup

			// Run the setup
			srv, err := setupServer(context.Background(), tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPort, srv.Addr)

			// Test that all expected paths are registered
			if len(tt.wantPaths) > 0 {
				mux, ok := srv.Handler.(*http.ServeMux)
				require.True(t, ok, "Handler should be *http.ServeMux")

				for _, path := range tt.wantPaths {
					h, pattern := mux.Handler(&http.Request{URL: &url.URL{Path: path}})
					assert.NotNil(t, h, "Handler should be registered for path %s", path)
					assert.Equal(t, path, pattern, "Path %s should be registered", path)
				}
			}
		})
	}
}

func TestRun(t *testing.T) {
	// Save original setupServer and restore after test
	originalSetup := setupServer
	defer func() { setupServer = originalSetup }()

	// Create mock setup function
	setupServer = func(ctx context.Context, cfg *config) (*http.Server, error) {
		mux := http.NewServeMux()
		mux.Handle("/", &mockServer{})
		return &http.Server{
			Addr:    ":0",
			Handler: mux,
		}, nil
	}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Create a channel to signal server start
	started := make(chan struct{})

	// Create a channel to receive any server errors
	errChan := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		cfg := &config{
			port:       ":0",
			bucketName: "test-bucket",
			projectID:  "test-project",
		}

		// Create server with mock setup
		srv, err := setupServer(ctx, cfg)
		if err != nil {
			errChan <- fmt.Errorf("failed to setup server: %v", err)
			return
		}

		// Get the actual port assigned by the OS
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			errChan <- fmt.Errorf("failed to listen: %v", err)
			return
		}
		defer listener.Close()

		// Update server address with actual listener address
		srv.Addr = listener.Addr().String()

		// Signal that we're ready to serve
		close(started)

		// Serve with the listener
		if err := srv.Serve(listener); err != http.ErrServerClosed {
			errChan <- fmt.Errorf("server error: %v", err)
		}
	}()

	// Wait for server to start or timeout
	select {
	case err := <-errChan:
		t.Fatalf("server failed to start: %v", err)
	case <-started:
		// Server started successfully
		cancel() // Trigger shutdown after successful start
	case <-ctx.Done():
		t.Fatal("timeout waiting for server to start")
	}
}
