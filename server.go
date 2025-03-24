package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/storage"
)

// ObjectStore defines the interface for storage operations
type ObjectStore interface {
	GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error)
}

// Logger defines the interface for logging operations
type Logger interface {
	Log(e logging.Entry)
}

// GCSObjectStore implements ObjectStore using Google Cloud Storage
type GCSObjectStore struct {
	bucket *storage.BucketHandle
}

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
}

func newGCSServer(ctx context.Context, bucketName string, logger *logging.Logger) (*gcsServer, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload: map[string]any{
				"error":     err.Error(),
				"operation": "create_storage_client",
			},
		})
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	store := &GCSObjectStore{
		bucket: client.Bucket(bucketName),
	}

	return &gcsServer{
		store:      store,
		bucketName: bucketName,
		logger:     logger,
	}, nil
}

// cleanRequestPath normalizes and validates the request path.
// It handles:
// 1. URL decoding
// 2. Multiple slashes removal
// 3. Directory index handling
// 4. Directory traversal prevention
// Returns:
// - The cleaned path
// - An error if the path is invalid
// - A boolean indicating if a redirect is needed (e.g., for directories without trailing slash)
func cleanRequestPath(path string) (string, error, bool) {
	// URL decode the path
	decodedPath, err := url.PathUnescape(path)
	if err != nil {
		return "", fmt.Errorf("error decoding path: %v", err), false
	}

	// Handle root path
	if decodedPath == "/" {
		return "index.html", nil, false
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
		return "index.html", nil, false
	}

	// Join parts and handle directory paths
	cleanPath := strings.Join(normalizedParts, "/")

	// Prevent directory traversal
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("invalid path: directory traversal attempt"), false
	}

	// Handle directory paths
	if decodedPath[len(decodedPath)-1] == '/' {
		return cleanPath + "/index.html", nil, false
	}

	// Check if this might be a directory without trailing slash
	if !strings.Contains(cleanPath, ".") {
		return cleanPath + "/", nil, true
	}

	return cleanPath, nil, false
}

func (s *gcsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	// Track active requests
	activeRequests.WithLabelValues(s.bucketName).Inc()
	defer activeRequests.WithLabelValues(s.bucketName).Dec()

	cleanPath, err, needsRedirect := cleanRequestPath(r.URL.Path)
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

	if needsRedirect {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		requestsTotal.WithLabelValues(s.bucketName, cleanPath, r.Method, "301").Inc()
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
