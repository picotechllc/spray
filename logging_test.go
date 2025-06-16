package main

import (
	"context"
	"os"
	"testing"

	"cloud.google.com/go/logging"
)

func TestCreateLoggingClient_MockMode(t *testing.T) {
	// Test when LOGGING_OFFLINE is set to true
	os.Setenv("LOGGING_OFFLINE", "true")
	defer os.Unsetenv("LOGGING_OFFLINE")

	ctx := context.Background()
	client, err := createLoggingClient(ctx, "test-project")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify it's a mock client by checking its type
	_, ok := client.(*mockLogClient)
	if !ok {
		t.Error("Expected mockLogClient, got different type")
	}
}

func TestCreateLoggingClient_OfflineCredentials(t *testing.T) {
	// Test when both GOOGLE_APPLICATION_CREDENTIALS and GCP_PROJECT are empty
	oldCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	oldProject := os.Getenv("GCP_PROJECT")
	defer func() {
		if oldCreds != "" {
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", oldCreds)
		}
		if oldProject != "" {
			os.Setenv("GCP_PROJECT", oldProject)
		}
	}()

	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Unsetenv("GCP_PROJECT")
	os.Unsetenv("LOGGING_OFFLINE") // Make sure this is not set

	ctx := context.Background()
	client, err := createLoggingClient(ctx, "test-project")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify it's a mock client
	_, ok := client.(*mockLogClient)
	if !ok {
		t.Error("Expected mockLogClient when credentials are not available")
	}
}

func TestMockLogClient_Logger(t *testing.T) {
	client := newMockLogClient()
	logger := client.Logger("test-logger")

	if logger == nil {
		t.Fatal("Expected logger to not be nil")
	}

	// Verify it's a mock logger
	_, ok := logger.(*mockLogger)
	if !ok {
		t.Error("Expected mockLogger, got different type")
	}
}

func TestMockLogClient_Close(t *testing.T) {
	client := newMockLogClient()
	err := client.Close()

	if err != nil {
		t.Errorf("Expected no error from Close(), got: %v", err)
	}
}

func TestMockLogger_Log(t *testing.T) {
	logger := &mockLogger{}

	// This should not panic - it's a no-op function
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Log() panicked: %v", r)
		}
	}()

	entry := logging.Entry{
		Severity: logging.Info,
		Payload:  "test message",
	}

	logger.Log(entry)
	// No assertion needed - we just want to ensure it doesn't panic
}

func TestNewMockLogClient(t *testing.T) {
	client := newMockLogClient()

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify it's the correct type
	_, ok := client.(*mockLogClient)
	if !ok {
		t.Error("Expected mockLogClient, got different type")
	}
}
