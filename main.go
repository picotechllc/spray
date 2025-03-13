package main

import (
	"context"
	"expvar"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/logging"
)

type config struct {
	port       string
	bucketName string
	projectID  string
}

func loadConfig() (*config, error) {
	var cfg config
	flag.StringVar(&cfg.port, "port", "8080", "Server port")
	flag.Parse()

	cfg.bucketName = os.Getenv("BUCKET_NAME")
	if cfg.bucketName == "" {
		return nil, fmt.Errorf("BUCKET_NAME environment variable is required")
	}

	cfg.projectID = os.Getenv("GOOGLE_PROJECT_ID")
	if cfg.projectID == "" {
		return nil, fmt.Errorf("GOOGLE_PROJECT_ID environment variable is required")
	}

	return &cfg, nil
}

func setupServer(ctx context.Context, cfg *config) (*http.Server, error) {
	client, err := logging.NewClient(ctx, cfg.projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create logging client: %v", err)
	}

	logger := client.Logger("gcs-server")

	server, err := newGCSServer(ctx, cfg.bucketName, logger)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create GCS server: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", server)
	mux.Handle("/metrics", expvar.Handler())
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
	cfg, err := loadConfig()
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
