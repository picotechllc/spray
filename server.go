package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// GCSObjectStore implements ObjectStore using Google Cloud Storage
type GCSObjectStore struct {
	bucket *storage.BucketHandle
}

// GetObject retrieves an object from the GCS bucket
func (s *GCSObjectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	obj := s.bucket.Object(path)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, nil, err
	}

	attrs, err := obj.Attrs(ctx)
	if err != nil {
		reader.Close()
		return nil, nil, err
	}

	return reader, attrs, nil
}

type gcsServer struct {
	store      ObjectStore
	bucketName string
	logger     Logger
	redirects  map[string]string
}

// newGCSServer creates a new GCS server
func newGCSServer(ctx context.Context, bucketName string, logger Logger, store ObjectStore, redirects map[string]string) (*gcsServer, error) {
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}

	return &gcsServer{
		store:      store,
		bucketName: bucketName,
		logger:     logger,
		redirects:  redirects,
	}, nil
}

// cleanRequestPath normalizes and validates the request path.
// It handles:
// 1. URL decoding
// 2. Multiple slashes removal
// 3. Directory index handling
// 4. Directory traversal prevention
// Returns the cleaned path and an error if the path is invalid
func cleanRequestPath(path string) (string, error) {
	// URL decode the path
	decodedPath, err := url.PathUnescape(path)
	if err != nil {
		return "", fmt.Errorf("error decoding path: %v", err)
	}

	// Handle root path
	if decodedPath == "/" {
		return "index.html", nil
	}

	// Remove leading slash and normalize multiple slashes
	parts := strings.Split(decodedPath, "/")
	var normalizedParts []string
	for _, part := range parts {
		if part != "" {
			normalizedParts = append(normalizedParts, part)
		}
	}

	// Handle empty path
	if len(normalizedParts) == 0 {
		return "index.html", nil
	}

	// Join parts and handle directory paths
	cleanPath := strings.Join(normalizedParts, "/")
	if decodedPath[len(decodedPath)-1] == '/' {
		cleanPath += "/index.html"
	}

	// Prevent directory traversal
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("invalid path: directory traversal attempt")
	}

	return cleanPath, nil
}

func (s *gcsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	// Track active requests
	activeRequests.WithLabelValues(s.bucketName).Inc()
	defer activeRequests.WithLabelValues(s.bucketName).Dec()

	cleanPath, err := cleanRequestPath(r.URL.Path)
	if err != nil {
		s.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload: map[string]any{
				"error":     err.Error(),
				"path":      r.URL.Path,
				"operation": "clean_path",
			},
		})
		errorTotal.WithLabelValues(s.bucketName, r.URL.Path, "invalid_path").Inc()
		requestsTotal.WithLabelValues(s.bucketName, r.URL.Path, r.Method, "400").Inc()
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Check for redirects
	if destination, exists := s.redirects[cleanPath]; exists {
		redirectStart := time.Now()
		s.logger.Log(logging.Entry{
			Severity: logging.Info,
			Payload: map[string]any{
				"path":        cleanPath,
				"destination": destination,
				"operation":   "redirect",
			},
		})
		requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "302").Inc()
		redirectHits.WithLabelValues(s.bucketName, cleanPath, destination).Inc()
		redirectLatency.WithLabelValues(s.bucketName, cleanPath).Observe(time.Since(redirectStart).Seconds())
		http.Redirect(w, r, destination, http.StatusFound)
		return
	}

	// Track GCS operations timing
	gcsStart := time.Now()
	reader, attrs, err := s.store.GetObject(ctx, cleanPath)
	gcsLatency.WithLabelValues(s.bucketName, "get_object").Observe(time.Since(gcsStart).Seconds())

	if err != nil {
		if err == storage.ErrObjectNotExist {
			s.logger.Log(logging.Entry{
				Severity: logging.Warning,
				Payload: map[string]any{
					"error":     err.Error(),
					"path":      cleanPath,
					"operation": "get_object",
					"status":    http.StatusNotFound,
				},
			})
			errorTotal.WithLabelValues(s.bucketName, cleanPath, "object_not_found").Inc()
			requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "404").Inc()
			http.NotFound(w, r)
			return
		}
		s.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload: map[string]any{
				"error":     err.Error(),
				"path":      cleanPath,
				"operation": "get_object",
				"status":    http.StatusInternalServerError,
			},
		})
		errorTotal.WithLabelValues(s.bucketName, cleanPath, "storage_error").Inc()
		requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "500").Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Track object size
	objectSize.WithLabelValues(s.bucketName, cleanPath).Observe(float64(attrs.Size))

	w.Header().Set("Content-Type", attrs.ContentType)

	// Copy the object contents to the response while tracking bytes transferred
	written, err := io.Copy(w, reader)
	if err != nil {
		s.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload: map[string]any{
				"error":     err.Error(),
				"path":      cleanPath,
				"operation": "copy_contents",
			},
		})
		errorTotal.WithLabelValues(s.bucketName, cleanPath, "copy_error").Inc()
		requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "500").Inc()
	} else {
		requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "200").Inc()
		bytesTransferred.WithLabelValues(s.bucketName, cleanPath, r.Method, "download").Add(float64(written))
	}

	// Record request duration
	duration := time.Since(start).Seconds()
	requestDuration.WithLabelValues(s.bucketName, cleanPath, r.Method).Observe(duration)
}

func readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func livezHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// createServer creates a new HTTP server with the given configuration.
func createServer(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
	logger := logClient.Logger("gcs-server")

	server, err := newGCSServer(ctx, cfg.bucketName, logger, cfg.store, cfg.redirects)
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

// handleSignals is a package-level variable to allow overriding in tests.
var handleSignals = handleSignalsImpl

// handleSignalsImpl sets up signal handling and returns a channel that will be closed when a signal is received.
func handleSignalsImpl() chan struct{} {
	shutdown := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		close(shutdown)
	}()

	return shutdown
}

// runServer is a package-level variable to allow mocking in tests.
var runServer = runServerImpl

// runServerImpl runs the HTTP server until it is shut down.
func runServerImpl(ctx context.Context, srv *http.Server) error {
	// Start the server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Error running server: %v", err)
		}
	}()

	// Wait for shutdown signal
	shutdown := handleSignals()
	<-shutdown

	// Create a new context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %v", err)
	}

	return nil
}
