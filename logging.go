package main

import (
	"context"
	"os"

	"cloud.google.com/go/logging"
)

// createLoggingClient creates a new logging client.
func createLoggingClient(ctx context.Context, projectID string) (LoggingClient, error) {
	// Check if we're running in a test environment or offline mode
	if os.Getenv("LOGGING_OFFLINE") == "true" || (os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" && os.Getenv("GCP_PROJECT") == "") {
		// In test environment or offline mode, return a mock LoggingClient
		return newMockLogClient(), nil
	}

	// In production, create a real client
	client, err := logging.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return newGCPLoggingClient(client), nil
}

// mockLogger is a mock implementation of the Logger interface
// Used for tests
type mockLogger struct{}

func (l *mockLogger) Log(entry logging.Entry) {
	// No-op in tests
}

// mockLogClient is a mock implementation of the LoggingClient interface
// Used for tests
type mockLogClient struct{}

func (c *mockLogClient) Logger(name string, opts ...logging.LoggerOption) Logger {
	return &mockLogger{}
}

func (c *mockLogClient) Close() error {
	return nil
}

// newMockLogClient creates a new mock logging client
func newMockLogClient() LoggingClient {
	return &mockLogClient{}
}
