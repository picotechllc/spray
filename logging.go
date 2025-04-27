package main

import (
	"context"

	"cloud.google.com/go/logging"
)

// createLoggingClient creates a new logging client.
func createLoggingClient(ctx context.Context, projectID string) (*logging.Client, error) {
	return logging.NewClient(ctx, projectID)
}
