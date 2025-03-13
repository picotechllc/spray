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
	logger     *logging.Logger
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
	activeRequests.Inc()
	defer activeRequests.Dec()

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
		requestsTotal.WithLabelValues(r.URL.Path, r.Method, "400").Inc()
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	reader, attrs, err := s.store.GetObject(ctx, cleanPath)
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
			requestsTotal.WithLabelValues(cleanPath, r.Method, "404").Inc()
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
		requestsTotal.WithLabelValues(cleanPath, r.Method, "500").Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

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
		requestsTotal.WithLabelValues(cleanPath, r.Method, "500").Inc()
	} else {
		requestsTotal.WithLabelValues(cleanPath, r.Method, "200").Inc()
		bytesTransferred.WithLabelValues(cleanPath, r.Method, "download").Add(float64(written))
	}

	// Record request duration
	duration := time.Since(start).Seconds()
	requestDuration.WithLabelValues(cleanPath, r.Method).Observe(duration)
}

func readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func livezHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
