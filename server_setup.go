package main

import (
	"context"
	"fmt"
	"net/http"

	"cloud.google.com/go/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServerSetup is a function type that creates a new server
type ServerSetup func(context.Context, *config, *logging.Client) (*http.Server, error)

// defaultServerSetup creates a new server with default configuration
func defaultServerSetup(ctx context.Context, cfg *config, logClient *logging.Client) (*http.Server, error) {
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

// DefaultServerSetup is the default server setup implementation
var DefaultServerSetup ServerSetup = defaultServerSetup
