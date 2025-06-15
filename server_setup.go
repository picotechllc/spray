package main

import (
	"context"
	"fmt"
	"net/http"

	"cloud.google.com/go/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// LoggingClient is an interface for logging clients
type LoggingClient interface {
	Logger(name string, opts ...logging.LoggerOption) *logging.Logger
	Close() error
}

// ServerSetup is a function type that sets up the server
type ServerSetup = func(context.Context, *config, LoggingClient) (*http.Server, error)

// defaultServerSetup creates a new server with default configuration
func defaultServerSetup(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
	logger := logClient.Logger("gcs-server")

	server, err := newGCSServer(ctx, cfg.bucketName, logger, nil, cfg.redirects)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS server: %v", err)
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

// DefaultServerSetup is the default server setup function
var DefaultServerSetup ServerSetup = func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
	logger := logClient.Logger("gcs-server")

	// Create a new GCS server
	server, err := newGCSServer(ctx, cfg.bucketName, logger, cfg.store, cfg.redirects)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS server: %v", err)
	}

	// Set up HTTP handlers
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
