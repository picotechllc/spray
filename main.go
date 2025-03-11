package main

import (
	"context"
	"expvar"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
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
	store        ObjectStore
	bucketName   string
	objectCounts sync.Map
	logger       *logging.Logger
}

var (
	objectsServed = expvar.NewInt("objects_served")
	totalDuration = expvar.NewInt("total_duration_ms")
)

func newGCSServer(ctx context.Context, bucketName string, logger *logging.Logger) (*gcsServer, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload:  fmt.Sprintf("failed to create client: %v", err),
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

	cleanPath, err := cleanRequestPath(r.URL.Path)
	if err != nil {
		s.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload:  fmt.Sprintf("Error cleaning path %s: %v", r.URL.Path, err),
		})
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	reader, attrs, err := s.store.GetObject(ctx, cleanPath)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			s.logger.Log(logging.Entry{
				Severity: logging.Warning,
				Payload:  fmt.Sprintf("Object %s not found: %v", cleanPath, err),
			})
			http.NotFound(w, r)
			return
		}
		s.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload:  fmt.Sprintf("Error opening object %s: %v", cleanPath, err),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", attrs.ContentType)

	// Copy the object contents to the response
	if _, err := io.Copy(w, reader); err != nil {
		s.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload:  fmt.Sprintf("Error copying object contents: %v", err),
		})
	}

	duration := time.Since(start).Milliseconds()
	objectsServed.Add(1)
	totalDuration.Add(duration)

	// Update per-object counter
	metricKey := fmt.Sprintf("%s/%s", s.bucketName, cleanPath)
	count, _ := s.objectCounts.LoadOrStore(metricKey, new(expvar.Int))
	count.(*expvar.Int).Add(1)
}

func readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func livezHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	var bucketName string
	var port string

	flag.StringVar(&port, "port", "8080", "Server port")
	flag.Parse()

	bucketName = os.Getenv("BUCKET_NAME")
	if bucketName == "" {
		log.Fatal("Bucket name is required to serve objects")
	}

	projectID := os.Getenv("GOOGLE_PROJECT_ID")
	if projectID == "" {
		log.Fatal("GOOGLE_PROJECT_ID is required to create logging client")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("Using bucket: %s", bucketName)

	client, err := logging.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create logging client: %v", err)
	}
	defer client.Close()

	logger := client.Logger("gcs-server")

	server, err := newGCSServer(ctx, bucketName, logger)
	if err != nil {
		log.Fatalf("Error creating GCS server: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", server)
	mux.Handle("/metrics", expvar.Handler())
	mux.HandleFunc("/readyz", readyzHandler)
	mux.HandleFunc("/livez", livezHandler)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()
	log.Printf("Server started on port %s", port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
