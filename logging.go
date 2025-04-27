package main

import (
	"context"
	"os"
	"strings"

	"cloud.google.com/go/logging"
	"google.golang.org/api/option"
)

// createLoggingClient creates a new logging client.
func createLoggingClient(ctx context.Context, projectID string) (*logging.Client, error) {
	// Support offline/no-auth mode for CI/dev
	if strings.ToLower(os.Getenv("LOGGING_OFFLINE")) == "true" || (os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" && os.Getenv("GOOGLE_CLOUD_PROJECT") == "") {
		return logging.NewClient(ctx, projectID, option.WithoutAuthentication())
	}
	return logging.NewClient(ctx, projectID)
}

// loggingClientFactory allows injection/mocking in tests.
var loggingClientFactory = createLoggingClient
