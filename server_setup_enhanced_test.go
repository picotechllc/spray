package main

import (
	"context"
	"testing"
)

func TestDefaultServerSetup_Success(t *testing.T) {
	ctx := context.Background()

	// Create test config
	cfg := &config{
		bucketName: "test-bucket",
		port:       "8080",
		store:      &mockObjectStore{},
		redirects:  make(map[string]string),
	}

	// Create mock logging client
	logClient := newMockLogClient()

	// Test DefaultServerSetup
	server, err := DefaultServerSetup(ctx, cfg, logClient)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if server == nil {
		t.Fatal("Expected server to not be nil")
	}

	if server.Addr != ":8080" {
		t.Errorf("Expected server address to be ':8080', got: %s", server.Addr)
	}

	if server.Handler == nil {
		t.Error("Expected server handler to not be nil")
	}
}

func TestDefaultServerSetup_WithRedirects(t *testing.T) {
	ctx := context.Background()

	// Create test config with redirects
	redirects := map[string]string{
		"/old-path": "/new-path",
		"/docs":     "/documentation",
	}

	cfg := &config{
		bucketName: "test-bucket",
		port:       "9090",
		store:      &mockObjectStore{},
		redirects:  redirects,
	}

	logClient := newMockLogClient()

	server, err := DefaultServerSetup(ctx, cfg, logClient)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if server == nil {
		t.Fatal("Expected server to not be nil")
	}

	if server.Addr != ":9090" {
		t.Errorf("Expected server address to be ':9090', got: %s", server.Addr)
	}
}

func TestDefaultServerSetup_NilStore(t *testing.T) {
	ctx := context.Background()

	cfg := &config{
		bucketName: "test-bucket",
		port:       "8080",
		store:      nil, // This should cause newGCSServer to fail
		redirects:  make(map[string]string),
	}

	logClient := newMockLogClient()

	server, err := DefaultServerSetup(ctx, cfg, logClient)

	// This should fail because store is nil
	if err == nil {
		t.Error("Expected error when store is nil, got none")
	}

	if server != nil {
		t.Error("Expected server to be nil when setup fails")
	}
}
