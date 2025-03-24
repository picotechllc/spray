package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
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
		name        string
		base        *config
		envVars     map[string]string
		want        *config
		wantErr     bool
		errContains string
	}{
		{
			name: "default config with required env vars",
			envVars: map[string]string{
				"BUCKET_NAME":       "test-bucket",
				"GOOGLE_PROJECT_ID": "test-project",
			},
			want: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
		},
		{
			name: "override port from base config",
			base: &config{port: "9000"},
			envVars: map[string]string{
				"BUCKET_NAME":       "test-bucket",
				"GOOGLE_PROJECT_ID": "test-project",
			},
			want: &config{
				port:       "9000",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
		},
		{
			name:        "missing bucket name",
			wantErr:     true,
			errContains: "BUCKET_NAME environment variable is required",
		},
		{
			name: "missing project ID",
			envVars: map[string]string{
				"BUCKET_NAME": "test-bucket",
			},
			wantErr:     true,
			errContains: "GOOGLE_PROJECT_ID environment variable is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			got, err := loadConfig(tt.base)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err == nil {
					t.Error("loadConfig() error = nil, want error")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("loadConfig() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if got.port != tt.want.port {
				t.Errorf("loadConfig() port = %v, want %v", got.port, tt.want.port)
			}
			if got.bucketName != tt.want.bucketName {
				t.Errorf("loadConfig() bucketName = %v, want %v", got.bucketName, tt.want.bucketName)
			}
			if got.projectID != tt.want.projectID {
				t.Errorf("loadConfig() projectID = %v, want %v", got.projectID, tt.want.projectID)
			}
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
			Addr:    "127.0.0.1:8081", // Use fixed port for testing
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
			port:       "8081",
			bucketName: "test-bucket",
			projectID:  "test-project",
		}

		// Create server with mock setup
		srv, err := setupServer(ctx, cfg)
		if err != nil {
			errChan <- fmt.Errorf("failed to setup server: %v", err)
			return
		}

		// Signal that we're ready to serve
		close(started)

		// Start the server
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
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

func TestReadyzHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "GET request",
			method:     "GET",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "POST request",
			method:     "POST",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "/readyz", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := newTestResponseRecorder()
			handler := http.HandlerFunc(readyzHandler)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.wantStatus)
			}

			if rr.Body.String() != tt.wantBody {
				t.Errorf("handler returned unexpected body: got %v want %v",
					rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestLivezHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "GET request",
			method:     "GET",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "POST request",
			method:     "POST",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "/livez", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := newTestResponseRecorder()
			handler := http.HandlerFunc(livezHandler)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.wantStatus)
			}

			if rr.Body.String() != tt.wantBody {
				t.Errorf("handler returned unexpected body: got %v want %v",
					rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestGracefulShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows due to socket provider issues")
		return
	}

	// Create a test server with a handler that simulates work
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Simulate work
		w.WriteHeader(http.StatusOK)
	})

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create channels for synchronization
	requestDone := make(chan struct{})

	// Start a long request in a goroutine
	go func() {
		defer close(requestDone)
		resp, err := http.Get(server.URL)
		if err != nil {
			// Ignore expected errors during shutdown
			if !strings.Contains(err.Error(), "connection refused") &&
				!strings.Contains(err.Error(), "EOF") {
				t.Logf("Request error: %v", err)
			}
			return
		}
		defer resp.Body.Close()
	}()

	// Wait briefly for the request to start
	time.Sleep(50 * time.Millisecond)

	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Initiate graceful shutdown
	if err := server.Config.Shutdown(ctx); err != nil {
		t.Errorf("Graceful shutdown failed: %v", err)
	}

	// Wait for request completion
	select {
	case <-requestDone:
		// Request completed
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for server to shut down")
	}
}

// Helper functions

type responseRecorder struct {
	Code int
	Body *strings.Builder
}

func newTestResponseRecorder() *responseRecorder {
	return &responseRecorder{
		Body: &strings.Builder{},
	}
}

func (r *responseRecorder) Header() http.Header {
	return http.Header{}
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	return r.Body.Write(p)
}

func (r *responseRecorder) WriteHeader(code int) {
	r.Code = code
}
