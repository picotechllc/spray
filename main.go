package main

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/storage"
)

type config struct {
	BucketName string `json:"bucketName"`
	Port       string `json:"port"`
}

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

func loadConfig(filename string) (*config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	var cfg config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %v", err)
	}

	return &cfg, nil
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
	ctx := context.Background()

	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Using bucket: %s", cfg.BucketName)

	server, err := newGCSServer(ctx, cfg.BucketName)
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/", server)
	http.Handle("/metrics", expvar.Handler())
	http.HandleFunc("/readyz", readyzHandler)
	http.HandleFunc("/livez", livezHandler)
	log.Printf("Starting server on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatal(err)
	}
}
