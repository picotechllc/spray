package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServerSetup is a function type that sets up the server
type ServerSetup = func(context.Context, *config, LoggingClient) (*http.Server, error)

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
	mux.HandleFunc("/config/redirects", configRedirectsHandler(server))

	return &http.Server{
		Addr:    ":" + cfg.port,
		Handler: mux,
	}, nil
}
