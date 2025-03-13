package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Save original env vars
	origBucket := os.Getenv("BUCKET_NAME")
	origProject := os.Getenv("GOOGLE_PROJECT_ID")
	defer func() {
		os.Setenv("BUCKET_NAME", origBucket)
		os.Setenv("GOOGLE_PROJECT_ID", origProject)
	}()

	tests := []struct {
		name        string
		bucket      string
		project     string
		basePort    string
		expectPort  string
		expectError bool
	}{
		{
			name:        "Valid configuration",
			bucket:      "test-bucket",
			project:     "test-project",
			expectPort:  "8080", // default port
			expectError: false,
		},
		{
			name:        "Valid configuration with custom port",
			bucket:      "test-bucket",
			project:     "test-project",
			basePort:    "9090",
			expectPort:  "9090",
			expectError: false,
		},
		{
			name:        "Missing bucket",
			bucket:      "",
			project:     "test-project",
			expectError: true,
		},
		{
			name:        "Missing project",
			bucket:      "test-bucket",
			project:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("BUCKET_NAME", tt.bucket)
			os.Setenv("GOOGLE_PROJECT_ID", tt.project)

			var base *config
			if tt.basePort != "" {
				base = &config{port: tt.basePort}
			}

			cfg, err := loadConfig(base)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cfg.bucketName != tt.bucket {
				t.Errorf("Expected bucket %q, got %q", tt.bucket, cfg.bucketName)
			}
			if cfg.projectID != tt.project {
				t.Errorf("Expected project %q, got %q", tt.project, cfg.projectID)
			}
			if cfg.port != tt.expectPort {
				t.Errorf("Expected port %q, got %q", tt.expectPort, cfg.port)
			}
		})
	}
}

func TestSetupServer(t *testing.T) {
	cfg := &config{
		port:       "8080",
		bucketName: "test-bucket",
		projectID:  "test-project",
	}

	ctx := context.Background()
	srv, err := setupServer(ctx, cfg)
	if err != nil {
		// We expect an error in tests since we can't authenticate with GCP
		if err.Error() == "google: could not find default credentials" {
			return
		}
		t.Errorf("Unexpected error: %v", err)
		return
	}

	if srv.Addr != ":8080" {
		t.Errorf("Expected server address :8080, got %s", srv.Addr)
	}
}

func TestRun(t *testing.T) {
	cfg := &config{
		port:       "0", // Use port 0 to let the OS choose a free port
		bucketName: "test-bucket",
		projectID:  "test-project",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := setupServer(ctx, cfg)
	if err != nil {
		// We expect an error in tests since we can't authenticate with GCP
		if err.Error() == "google: could not find default credentials" {
			return
		}
		t.Errorf("Unexpected error: %v", err)
		return
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, srv)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown
	cancel()

	// Wait for shutdown or timeout
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Error("Server shutdown timed out")
	}
}
