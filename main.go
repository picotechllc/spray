package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type config struct {
	port       string
	bucketName string
	projectID  string // optional: if not set, will use standard logging
}

// parseFlags parses command line flags and returns a config with just the flags set.
// This should only be called once from main().
func parseFlags() *config {
	var cfg config
	flag.StringVar(&cfg.port, "port", "8080", "Server port")
	flag.Parse()
	return &cfg
}

// loadConfig loads configuration from environment variables.
// It takes an optional base config to extend (e.g., from command line flags).
func loadConfig(base *config) (*config, error) {
	cfg := &config{
		port: "8080", // default value
	}
	if base != nil {
		*cfg = *base
	}

	cfg.bucketName = os.Getenv("BUCKET_NAME")
	if cfg.bucketName == "" {
		return nil, fmt.Errorf("BUCKET_NAME environment variable is required")
	}

	// ProjectID is optional - if not set, we'll use standard logging
	cfg.projectID = os.Getenv("GOOGLE_PROJECT_ID")

	return cfg, nil
}

// ServerSetup is a function type for setting up the HTTP server
type ServerSetup func(context.Context, *config) (*http.Server, error)

// setupServer is the default server setup implementation
var setupServer ServerSetup = func(ctx context.Context, cfg *config) (*http.Server, error) {
	var logger *logging.Logger
	var loggingClient *logging.Client

	// Only attempt to create Cloud Logging client if projectID is set
	if cfg.projectID != "" {
		var err error
		loggingClient, err = logging.NewClient(ctx, cfg.projectID)
		if err != nil {
			log.Printf("Warning: failed to create Cloud Logging client: %v. Falling back to standard logging.", err)
		} else {
			logger = loggingClient.Logger("gcs-server")
		}
	} else {
		log.Printf("Info: GOOGLE_PROJECT_ID not set. Using standard logging.")
	}

	server, err := newGCSServer(ctx, cfg.bucketName, logger)
	if err != nil {
		if loggingClient != nil {
			loggingClient.Close()
		}
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

func run(ctx context.Context, srv *http.Server) error {
	// Channel to listen for errors coming from the listener.
	serverErrors := make(chan error, 1)

	// Start the server
	go func() {
		log.Printf("Server started on port %s", srv.Addr)
		serverErrors <- srv.ListenAndServe()
	}()

	// Channel to listen for an interrupt or terminate signal from the OS.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Blocking select waiting for either a server error or a signal.
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %v", err)
	case <-shutdown:
		log.Println("Starting shutdown...")

		// Give outstanding requests a deadline for completion.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Asking listener to shut down and shed load.
		if err := srv.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout.
			return fmt.Errorf("graceful shutdown failed: %v", err)
		}
	}

	return nil
}

func main() {
	flagCfg := parseFlags()
	cfg, err := loadConfig(flagCfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	srv, err := setupServer(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := run(ctx, srv); err != nil {
		log.Fatal(err)
	}
}
