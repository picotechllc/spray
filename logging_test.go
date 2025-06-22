package main

import (
	"context"
	"os"
	"testing"

	"cloud.google.com/go/logging"
	"github.com/stretchr/testify/assert"
)

func TestCreateLoggingClient_TestMode(t *testing.T) {
	// Test when LOGGING_TEST_MODE is set to true
	os.Setenv("LOGGING_TEST_MODE", "true")
	defer os.Unsetenv("LOGGING_TEST_MODE")

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

func TestCreateLoggingClient_OfflineMode(t *testing.T) {
	// Test when LOGGING_OFFLINE is set to true (should get zap logger)
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

	// Verify it's a zap client by checking its type
	_, ok := client.(*zapLogClient)
	if !ok {
		t.Error("Expected zapLogClient in offline mode, got different type")
	}
}

func TestCreateLoggingClient_OfflineCredentials(t *testing.T) {
	// Test when both GOOGLE_APPLICATION_CREDENTIALS and GCP_PROJECT are empty
	// and we're in test context (should get mock logger)
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
	os.Unsetenv("LOGGING_OFFLINE")   // Make sure this is not set
	os.Unsetenv("LOGGING_TEST_MODE") // Make sure this is not set

	ctx := context.Background()
	client, err := createLoggingClient(ctx, "test-project")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Since we're running as a test, should get mock client
	_, ok := client.(*mockLogClient)
	if !ok {
		t.Error("Expected mockLogClient when credentials are not available in test context")
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

func TestZapLogClient_Basic(t *testing.T) {
	client := newZapLogClient()

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify it's the correct type
	_, ok := client.(*zapLogClient)
	if !ok {
		t.Error("Expected zapLogClient, got different type")
	}

	// Test creating a logger
	logger := client.Logger("test-logger")
	if logger == nil {
		t.Fatal("Expected logger to not be nil")
	}

	// Verify it's a zap logger
	_, ok = logger.(*zapLogger)
	if !ok {
		t.Error("Expected zapLogger, got different type")
	}

	// Test closing (should not error)
	err := client.Close()
	if err != nil {
		t.Errorf("Expected no error from Close(), got: %v", err)
	}
}

func TestZapLogClient_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		wantError bool
	}{
		{
			name:      "Empty project ID",
			projectID: "",
			wantError: false, // Should still create client but may not work properly
		},
		{
			name:      "Valid project ID",
			projectID: "test-project",
			wantError: false,
		},
		{
			name:      "Special characters in project ID",
			projectID: "test-project-123_abc",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test mode to avoid actual GCP logging
			oldTestMode := os.Getenv("TEST_MODE")
			os.Setenv("TEST_MODE", "true")
			defer os.Setenv("TEST_MODE", oldTestMode)

			client, err := createLoggingClient(context.Background(), tt.projectID)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)

				// Test that we can create a logger
				logger := client.Logger("test-logger")
				assert.NotNil(t, logger)

				// Test Close
				err = client.Close()
				assert.NoError(t, err)
			}
		})
	}
}

func TestZapLogger_LogLevels(t *testing.T) {
	// Create a zap logger for testing
	zapClient := newZapLogClient()
	defer zapClient.Close()

	logger := zapClient.Logger("test-logger")
	zapLogger, ok := logger.(*zapLogger)
	assert.True(t, ok, "Expected zapLogger type")

	// Test different log levels
	testCases := []struct {
		name     string
		severity logging.Severity
		message  string
	}{
		{
			name:     "Debug level",
			severity: logging.Debug,
			message:  "Debug message",
		},
		{
			name:     "Info level",
			severity: logging.Info,
			message:  "Info message",
		},
		{
			name:     "Warning level",
			severity: logging.Warning,
			message:  "Warning message",
		},
		{
			name:     "Error level",
			severity: logging.Error,
			message:  "Error message",
		},
		{
			name:     "Critical level",
			severity: logging.Critical,
			message:  "Critical message",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entry := logging.Entry{
				Severity: tc.severity,
				Payload:  tc.message,
			}

			// This should not panic
			assert.NotPanics(t, func() {
				zapLogger.Log(entry)
			})
		})
	}
}

func TestZapLogger_ComplexPayloads(t *testing.T) {
	// Create a zap logger for testing
	zapClient := newZapLogClient()
	defer zapClient.Close()

	logger := zapClient.Logger("test-logger")

	testCases := []struct {
		name    string
		payload interface{}
	}{
		{
			name:    "String payload",
			payload: "Simple string message",
		},
		{
			name: "Map payload",
			payload: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
		},
		{
			name: "Nested map payload",
			payload: map[string]interface{}{
				"operation": "test",
				"details": map[string]interface{}{
					"user":      "test-user",
					"timestamp": "2023-01-01T00:00:00Z",
				},
			},
		},
		{
			name:    "Nil payload",
			payload: nil,
		},
		{
			name:    "Numeric payload",
			payload: 12345,
		},
		{
			name:    "Boolean payload",
			payload: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entry := logging.Entry{
				Severity: logging.Info,
				Payload:  tc.payload,
			}

			// This should not panic
			assert.NotPanics(t, func() {
				logger.Log(entry)
			})
		})
	}
}
