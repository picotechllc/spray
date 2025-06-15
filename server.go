package main

import (
	"context"
	"encoding/json"
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

// errorResponse represents a structured error response
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// logError logs an error with structured JSON format
func (s *gcsServer) logError(severity logging.Severity, operation, path string, statusCode int, err error) {
	payload := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"operation": operation,
		"path":      path,
		"status":    statusCode,
		"bucket":    s.bucketName,
	}

	if err != nil {
		payload["error"] = err.Error()
		payload["error_type"] = getErrorType(err)
	}

	entry := logging.Entry{
		Severity: severity,
		Payload:  payload,
	}

	s.logger.Log(entry)
}

// logInfo logs an info message with structured JSON format
func (s *gcsServer) logInfo(operation, path string, extra map[string]any) {
	payload := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"operation": operation,
		"path":      path,
		"bucket":    s.bucketName,
	}

	// Add extra fields
	for k, v := range extra {
		payload[k] = v
	}

	entry := logging.Entry{
		Severity: logging.Info,
		Payload:  payload,
	}

	s.logger.Log(entry)
}

// getErrorType categorizes errors for metrics and logging
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	switch {
	case err == storage.ErrObjectNotExist:
		return "object_not_found"
	case isPermissionError(err):
		return "permission_denied"
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "connection"):
		return "connection_error"
	default:
		return "storage_error"
	}
}

// sendUserFriendlyError sends a user-friendly error response while logging the detailed error
func (s *gcsServer) sendUserFriendlyError(w http.ResponseWriter, r *http.Request, path string, statusCode int, userMessage string, actualError error) {
	// Log the detailed error for debugging
	var severity logging.Severity
	switch statusCode {
	case http.StatusNotFound:
		severity = logging.Warning
	case http.StatusInternalServerError, http.StatusForbidden:
		severity = logging.Error
	default:
		severity = logging.Info
	}

	s.logError(severity, "serve_request", path, statusCode, actualError)

	// Update metrics
	errorType := getErrorType(actualError)
	errorTotal.WithLabelValues(s.bucketName, path, errorType).Inc()
	requestsTotal.WithLabelValues(s.bucketName, path, r.Method, fmt.Sprintf("%d", statusCode)).Inc()

	// Determine response format based on Accept header
	acceptHeader := r.Header.Get("Accept")
	wantsJSON := strings.Contains(acceptHeader, "application/json") ||
		strings.Contains(acceptHeader, "*/*") && !strings.Contains(acceptHeader, "text/html")

	// If the request explicitly accepts HTML or doesn't specify (browser behavior)
	if strings.Contains(acceptHeader, "text/html") || (!wantsJSON && acceptHeader != "") {
		// Send HTML error page for browsers
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(statusCode)

		htmlResponse := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error %d - %s</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            margin: 0;
            padding: 2rem;
            background-color: #f5f5f5;
            color: #333;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
            background: white;
            padding: 2rem;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h1 {
            color: #d73a49;
            margin-bottom: 1rem;
        }
        .error-code {
            font-size: 3rem;
            font-weight: bold;
            color: #d73a49;
            margin-bottom: 0.5rem;
        }
        .message {
            font-size: 1.1rem;
            margin-bottom: 1.5rem;
        }
        .help {
            background: #f8f9fa;
            padding: 1rem;
            border-radius: 4px;
            border-left: 4px solid #0366d6;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="error-code">%d</div>
        <h1>%s</h1>
        <div class="message">%s</div>
        <div class="help">
            <strong>What can you do?</strong>
            <ul>
                <li>Check the URL for typos</li>
                <li>Try refreshing the page</li>
                <li>Go back to the <a href="/">homepage</a></li>
            </ul>
        </div>
    </div>
</body>
</html>`, statusCode, http.StatusText(statusCode), statusCode, http.StatusText(statusCode), userMessage)

		w.Write([]byte(htmlResponse))
		return
	}

	// Send JSON response for API clients
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := errorResponse{
		Error:   http.StatusText(statusCode),
		Message: userMessage,
		Status:  statusCode,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Fallback to plain text if JSON encoding fails
		http.Error(w, userMessage, statusCode)
	}
}

func (s *gcsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	// Track active requests
	activeRequests.WithLabelValues(s.bucketName).Inc()
	defer activeRequests.WithLabelValues(s.bucketName).Dec()

	cleanPath, err := cleanRequestPath(r.URL.Path)
	if err != nil {
		s.sendUserFriendlyError(
			w, r, r.URL.Path, http.StatusBadRequest,
			"The requested path is invalid.",
			err,
		)
		return
	}

	// Check for redirects
	if destination, exists := s.redirects[cleanPath]; exists {
		redirectStart := time.Now()
		s.logInfo("redirect", cleanPath, map[string]any{
			"destination": destination,
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
			s.sendUserFriendlyError(
				w, r, cleanPath, http.StatusNotFound,
				"The requested resource was not found.",
				err,
			)
			return
		}

		if isPermissionError(err) {
			s.sendUserFriendlyError(
				w, r, cleanPath, http.StatusForbidden,
				"Access to this resource is not available at the moment. Please try again later.",
				err,
			)
			return
		}

		// For any other storage error
		s.sendUserFriendlyError(
			w, r, cleanPath, http.StatusInternalServerError,
			"The service is temporarily unavailable. Please try again later.",
			err,
		)
		return
	}
	defer reader.Close()

	// Track object size
	objectSize.WithLabelValues(s.bucketName, cleanPath).Observe(float64(attrs.Size))

	w.Header().Set("Content-Type", attrs.ContentType)

	// Copy the object contents to the response while tracking bytes transferred
	written, err := io.Copy(w, reader)
	if err != nil {
		s.logError(logging.Error, "copy_contents", cleanPath, http.StatusInternalServerError, err)
		errorTotal.WithLabelValues(s.bucketName, cleanPath, "copy_error").Inc()
		requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "500").Inc()
	} else {
		requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "200").Inc()
		bytesTransferred.WithLabelValues(s.bucketName, cleanPath, r.Method, "download").Add(float64(written))

		// Log successful request
		s.logInfo("serve_request", cleanPath, map[string]any{
			"status":       200,
			"bytes_served": written,
			"content_type": attrs.ContentType,
			"duration_ms":  time.Since(start).Milliseconds(),
		})
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
