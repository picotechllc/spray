package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		basePort   string
		expectPort string
		envs       map[string]string
		wantErr    bool
		errMsg     string
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
			errMsg:  "BUCKET_NAME environment variable is required",
		},
		{
			name: "missing project ID",
			envs: map[string]string{
				"BUCKET_NAME": "test-bucket",
			},
			wantErr: true,
			errMsg:  "GOOGLE_PROJECT_ID environment variable is required",
		},
		{
			name:       "empty environment variables",
			expectPort: "8080",
			envs:       map[string]string{},
			wantErr:    true,
			errMsg:     "BUCKET_NAME environment variable is required",
		},
		{
			name:       "all environment variables empty",
			expectPort: "8080",
			envs: map[string]string{
				"BUCKET_NAME":       "",
				"GOOGLE_PROJECT_ID": "",
			},
			wantErr: true,
			errMsg:  "BUCKET_NAME environment variable is required",
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
				if tt.errMsg != "" {
					assert.Equal(t, tt.errMsg, err.Error())
				}
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
	// Save original setupServer and restore after test
	originalSetup := DefaultServerSetup
	defer func() { DefaultServerSetup = originalSetup }()

	// Create a mock logging client
	logClient, err := logging.NewClient(context.Background(), "test-project", option.WithoutAuthentication())
	require.NoError(t, err)
	defer logClient.Close()

	// Create test cases
	tests := []struct {
		name        string
		cfg         *config
		setupServer func(context.Context, *config, *logging.Client) (*http.Server, error)
		wantErr     bool
	}{
		{
			name: "successful setup",
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
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
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			cfg: &config{
				port:       "invalid",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
				return nil, fmt.Errorf("invalid port")
			},
			wantErr: true,
		},
		{
			name: "missing bucket name",
			cfg: &config{
				port:      "8080",
				projectID: "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
				return nil, fmt.Errorf("bucket name is required")
			},
			wantErr: true,
		},
		{
			name: "logging client error",
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
				return nil, fmt.Errorf("failed to create logging client")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test setup function
			DefaultServerSetup = tt.setupServer

			// Create a context
			ctx := context.Background()

			// Create server with mock setup
			srv, err := DefaultServerSetup(ctx, tt.cfg, logClient)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, srv)

			// Verify the server has the expected handlers
			mux := srv.Handler.(*http.ServeMux)

			// Test root handler
			req := httptest.NewRequest("GET", "/", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusNotFound, rr.Code) // Should be 404 since we have no objects

			// Test metrics endpoint
			req = httptest.NewRequest("GET", "/metrics", nil)
			rr = httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusOK, rr.Code)

			// Test readyz endpoint
			req = httptest.NewRequest("GET", "/readyz", nil)
			rr = httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "ok", rr.Body.String())

			// Test livez endpoint
			req = httptest.NewRequest("GET", "/livez", nil)
			rr = httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "ok", rr.Body.String())
		})
	}
}

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

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPort string
	}{
		{
			name:     "default port",
			args:     []string{},
			wantPort: "8080",
		},
		{
			name:     "custom port",
			args:     []string{"-port", "9090"},
			wantPort: "9090",
		},
		{
			name:     "custom port with equal sign",
			args:     []string{"-port=9091"},
			wantPort: "9091",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original command line arguments
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			// Set test arguments
			os.Args = append([]string{"test"}, tt.args...)

			// Reset flag.CommandLine to clear any previously set flags
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			// Parse flags
			cfg := parseFlags()

			// Verify port
			assert.Equal(t, tt.wantPort, cfg.port)
		})
	}
}

func TestHealthCheckHandlers(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantCode int
		wantBody string
	}{
		{
			name:     "readyz handler",
			handler:  readyzHandler,
			wantCode: http.StatusOK,
			wantBody: "ok",
		},
		{
			name:     "livez handler",
			handler:  livezHandler,
			wantCode: http.StatusOK,
			wantBody: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			rr := httptest.NewRecorder()

			tt.handler(rr, req)

			assert.Equal(t, tt.wantCode, rr.Code)
			assert.Equal(t, tt.wantBody, rr.Body.String())
		})
	}
}

func TestSetupServer(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config
		setupServer ServerSetup
		wantErr     bool
	}{
		{
			name: "successful setup",
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
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
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			cfg: &config{
				port:       "invalid-port",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
				return nil, fmt.Errorf("invalid port")
			},
			wantErr: true,
		},
		{
			name: "missing bucket name",
			cfg: &config{
				port:      "8080",
				projectID: "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
				return nil, fmt.Errorf("missing bucket name")
			},
			wantErr: true,
		},
		{
			name: "logging client error",
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
				return nil, fmt.Errorf("failed to create logging client")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original setupServer and restore after test
			originalSetup := DefaultServerSetup
			defer func() { DefaultServerSetup = originalSetup }()

			// Set test setup function
			DefaultServerSetup = tt.setupServer

			// Create a mock logging client
			logClient := &logging.Client{}

			ctx := context.Background()
			srv, err := DefaultServerSetup(ctx, tt.cfg, logClient)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, srv)
			assert.Equal(t, ":"+tt.cfg.port, srv.Addr)

			// Verify that all expected handlers are registered
			mux, ok := srv.Handler.(*http.ServeMux)
			require.True(t, ok, "Handler should be *http.ServeMux")

			// Check for root handler
			h, pattern := mux.Handler(&http.Request{URL: &url.URL{Path: "/"}})
			assert.NotNil(t, h, "Root handler should be registered")
			assert.Equal(t, "/", pattern)

			// Check for metrics handler
			h, pattern = mux.Handler(&http.Request{URL: &url.URL{Path: "/metrics"}})
			assert.NotNil(t, h, "Metrics handler should be registered")
			assert.Equal(t, "/metrics", pattern)

			// Check for health check handlers
			h, pattern = mux.Handler(&http.Request{URL: &url.URL{Path: "/readyz"}})
			assert.NotNil(t, h, "Readyz handler should be registered")
			assert.Equal(t, "/readyz", pattern)

			h, pattern = mux.Handler(&http.Request{URL: &url.URL{Path: "/livez"}})
			assert.NotNil(t, h, "Livez handler should be registered")
			assert.Equal(t, "/livez", pattern)
		})
	}
}
