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
	headers    *HeaderConfig
}

// newGCSServer creates a new GCS server
func newGCSServer(ctx context.Context, bucketName string, logger Logger, store ObjectStore, redirects map[string]string, headers *HeaderConfig) (*gcsServer, error) {
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}

	return &gcsServer{
		store:      store,
		bucketName: bucketName,
		logger:     logger,
		redirects:  redirects,
		headers:    headers,
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

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	return rw.ResponseWriter.Write(data)
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

		// Add credential context for permission errors
		if isPermissionError(err) {
			credContext := getCredentialContext()
			for k, v := range credContext {
				payload[k] = v
			}
		}
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
	case http.StatusInternalServerError:
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

		// Determine if we should show the homepage link
		var homepageLink string
		if r.URL.Path != "/" && path != "index.html" {
			homepageLink = "\n                <li>Go back to the <a href=\"/\">homepage</a></li>"
		}

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
            margin: 0 auto 2rem auto;
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
        .footer {
            max-width: 600px;
            margin: 0 auto;
            padding: 1rem;
            text-align: center;
            color: #666;
            font-size: 0.9rem;
            border-top: 1px solid #e1e4e8;
        }
        .footer a {
            color: #0366d6;
            text-decoration: none;
        }
        .footer a:hover {
            text-decoration: underline;
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
                <li>Try refreshing the page</li>%s
            </ul>
        </div>
    </div>
    <footer class="footer">
        <a href="https://github.com/picotechllc/spray" target="_blank" rel="noopener">spray</a>/%s
    </footer>
</body>
</html>`, statusCode, http.StatusText(statusCode), statusCode, http.StatusText(statusCode), userMessage, homepageLink, Version)

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

	// Log incoming request
	s.logInfo("incoming_request", r.URL.Path, map[string]any{
		"method":     r.Method,
		"user_agent": r.Header.Get("User-Agent"),
		"remote_ip":  r.RemoteAddr,
		"accept":     r.Header.Get("Accept"),
	})

	// Wrap ResponseWriter to capture status code
	wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

	// Set X-Powered-By header if enabled
	if poweredByValue := resolveXPoweredByHeader(s.headers, Version); poweredByValue != "" {
		wrapped.Header().Set("X-Powered-By", poweredByValue)
	}

	// Panic recovery
	defer func() {
		if err := recover(); err != nil {
			wrapped.statusCode = 500

			// Create detailed error with stack trace
			panicErr := fmt.Errorf("panic: %v", err)
			s.logError(logging.Error, "panic_recovery", r.URL.Path, http.StatusInternalServerError, panicErr)

			// Also log additional panic details
			s.logError(logging.Error, "panic_details", r.URL.Path, http.StatusInternalServerError, fmt.Errorf("panic details - method: %s, path: %s, user_agent: %s", r.Method, r.URL.Path, r.Header.Get("User-Agent")))

			errorTotal.WithLabelValues(s.bucketName, r.URL.Path, "panic").Inc()
			requestsTotal.WithLabelValues(s.bucketName, r.URL.Path, r.Method, "500").Inc()

			// Try to send an error response if headers haven't been written
			if !wrapped.written {
				http.Error(wrapped, "Internal Server Error", http.StatusInternalServerError)
			}
		}

		// Log request completion
		duration := time.Since(start)
		s.logInfo("request_completed", r.URL.Path, map[string]any{
			"method":      r.Method,
			"status":      wrapped.statusCode,
			"duration_ms": duration.Milliseconds(),
		})

		// Record request duration
		requestDuration.WithLabelValues(s.bucketName, r.URL.Path, r.Method).Observe(duration.Seconds())
	}()

	cleanPath, err := cleanRequestPath(r.URL.Path)
	if err != nil {
		s.sendUserFriendlyError(
			wrapped, r, r.URL.Path, http.StatusBadRequest,
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
		wrapped.statusCode = 302
		http.Redirect(wrapped, r, destination, http.StatusFound)
		return
	}

	// Track GCS operations timing
	gcsStart := time.Now()
	reader, attrs, err := s.store.GetObject(ctx, cleanPath)
	gcsLatency.WithLabelValues(s.bucketName, "get_object").Observe(time.Since(gcsStart).Seconds())

	if err != nil {
		if err == storage.ErrObjectNotExist {
			s.sendUserFriendlyError(
				wrapped, r, cleanPath, http.StatusNotFound,
				"The requested resource was not found.",
				err,
			)
			return
		}

		if isPermissionError(err) {
			s.sendUserFriendlyError(
				wrapped, r, cleanPath, http.StatusInternalServerError,
				"The service is temporarily unavailable due to a configuration issue. Please try again later.",
				err,
			)
			return
		}

		// For any other storage error
		s.sendUserFriendlyError(
			wrapped, r, cleanPath, http.StatusInternalServerError,
			"The service is temporarily unavailable. Please try again later.",
			err,
		)
		return
	}
	defer reader.Close()

	// Track object size
	objectSize.WithLabelValues(s.bucketName, cleanPath).Observe(float64(attrs.Size))

	wrapped.Header().Set("Content-Type", attrs.ContentType)

	// Copy the object contents to the response while tracking bytes transferred
	written, err := io.Copy(wrapped, reader)
	if err != nil {
		// If we encounter an error during copy, the response might already be partially written
		// We can't change the status code at this point, but we can log the error
		wrapped.statusCode = 500
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
}

func readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func livezHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// configRedirectsHandler returns the current redirect configuration as JSON
func configRedirectsHandler(server *gcsServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method not allowed"))
			return
		}

		// Create response structure
		response := struct {
			Redirects    map[string]string `json:"redirects"`
			Count        int               `json:"count"`
			ConfigSource string            `json:"config_source"`
			BucketName   string            `json:"bucket_name"`
		}{
			Redirects:    server.redirects,
			Count:        len(server.redirects),
			ConfigSource: ".spray/redirects.toml",
			BucketName:   server.bucketName,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			server.logError(logging.Error, "config_redirects", "/config/redirects", http.StatusInternalServerError, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		server.logInfo("config_redirects", "/config/redirects", map[string]any{
			"redirect_count": len(server.redirects),
		})
	}
}

// createServer creates a new HTTP server with the given configuration.
func createServer(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
	logger := logClient.Logger("gcs-server")

	server, err := newGCSServer(ctx, cfg.bucketName, logger, cfg.store, cfg.redirects, cfg.headers)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS server: %v", err)
	}

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

// getCredentialContext returns information about the current credentials being used
func getCredentialContext() map[string]any {
	credContext := make(map[string]any)

	// Check for Application Default Credentials file
	if credsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credsPath != "" {
		credContext["credentials_source"] = "service_account_file"
		credContext["credentials_file"] = credsPath
	} else {
		// Check for other common credential sources
		credContext["credentials_source"] = "application_default_credentials"

		// Check if running on GCE/GKE
		if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" || os.Getenv("GCLOUD_PROJECT") != "" {
			credContext["gcp_environment"] = "cloud_environment"
		}
	}

	// Add project information
	if projectID := os.Getenv("GOOGLE_PROJECT_ID"); projectID != "" {
		credContext["project_id"] = projectID
	}
	if gcpProject := os.Getenv("GCP_PROJECT"); gcpProject != "" {
		credContext["gcp_project"] = gcpProject
	}

	return credContext
}
