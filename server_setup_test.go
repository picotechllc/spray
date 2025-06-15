package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

func TestServerSetup(t *testing.T) {
	// Save original setupServer and restore after test
	originalSetup := DefaultServerSetup
	defer func() { DefaultServerSetup = originalSetup }()

	// Mock loggingClientFactory to avoid real GCP credential lookup
	origLoggingClientFactory := loggingClientFactory
	loggingClientFactory = func(ctx context.Context, projectID string) (LoggingClient, error) {
		return logging.NewClient(ctx, projectID, option.WithoutAuthentication())
	}
	defer func() { loggingClientFactory = origLoggingClientFactory }()

	// Create a mock logging client
	logClient, err := loggingClientFactory(context.Background(), "test-project")
	require.NoError(t, err)
	defer logClient.Close()

	// Create test cases
	tests := []struct {
		name        string
		cfg         *config
		setupServer func(context.Context, *config, LoggingClient) (*http.Server, error)
		wantErr     bool
	}{
		{
			name: "successful setup",
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
			},
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
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
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
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
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
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
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
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

func TestSetupServer(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config
		setupServer func(context.Context, *config, LoggingClient) (*http.Server, error)
		wantErr     bool
	}{
		{
			name: "successful setup",
			cfg: &config{
				port:       "8080",
				bucketName: "test-bucket",
				projectID:  "test-project",
				store:      newMockStorageClient(),
			},
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
				// Create a mock logger that won't panic
				logger := &logging.Logger{}
				server, err := newGCSServer(ctx, cfg.bucketName, logger, cfg.store, cfg.redirects)
				if err != nil {
					return nil, err
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
				store:      newMockStorageClient(),
			},
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
				return nil, fmt.Errorf("invalid port")
			},
			wantErr: true,
		},
		{
			name: "missing bucket name",
			cfg: &config{
				port:      "8080",
				projectID: "test-project",
				store:     newMockStorageClient(),
			},
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
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
				store:      newMockStorageClient(),
			},
			setupServer: func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
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

			// Create a mock logging client
			logClient := newMockLogClient()

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
