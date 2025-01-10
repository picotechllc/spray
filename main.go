package main

import (
	"context"
	"expvar"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
)

type gcsServer struct {
	bucket       *storage.BucketHandle
	bucketName   string
	objectCounts sync.Map
}

var (
	objectsServed = expvar.NewInt("objects_served")
	totalDuration = expvar.NewInt("total_duration_ms")
)

func newGCSServer(ctx context.Context, bucketName string) (*gcsServer, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &gcsServer{
		bucket:     client.Bucket(bucketName),
		bucketName: bucketName,
	}, nil
}

func (s *gcsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()
	path := r.URL.Path

	// Serve index.html for root path
	if path == "/" {
		path = "index.html"
	} else {
		// Remove leading slash
		path = path[1:]
	}

	obj := s.bucket.Object(path)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			log.Printf("Object %s not found: %v", path, err)
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Set content type based on object attributes
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", attrs.ContentType)

	// Copy the object contents to the response
	if _, err := io.Copy(w, reader); err != nil {
		log.Printf("Error copying object contents: %v", err)
	}

	duration := time.Since(start).Milliseconds()
	objectsServed.Add(1)
	totalDuration.Add(duration)

	// Update per-object counter
	metricKey := fmt.Sprintf("%s/%s", s.bucketName, path)
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
		log.Fatal("Bucket name is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("Using bucket: %s", bucketName)

	server, err := newGCSServer(ctx, bucketName)
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
