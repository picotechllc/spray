package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Mock implementations
type mockObject struct {
	data        []byte
	contentType string
}

type mockReader struct {
	data   []byte
	offset int64
}

func (r *mockReader) Read(p []byte) (int, error) {
	if r.offset >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += int64(n)
	return n, nil
}

func (r *mockReader) Close() error {
	return nil
}

type mockObjectStore struct {
	objects map[string]mockObject
}

func (s *mockObjectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	obj, exists := s.objects[path]
	if !exists {
		return nil, nil, storage.ErrObjectNotExist
	}
	return &mockReader{data: obj.data}, &storage.ObjectAttrs{
		ContentType: obj.contentType,
	}, nil
}

// Mock logger for testing
type mockLogger struct {
	entries []logging.Entry
}

func (l *mockLogger) Log(e logging.Entry) {
	l.entries = append(l.entries, e)
}

func (l *mockLogger) Flush() error {
	return nil
}

func (l *mockLogger) Close() error {
	return nil
}

func (l *mockLogger) StandardLogger(severity logging.Severity) *log.Logger {
	return log.New(os.Stdout, "", log.LstdFlags)
}

func (l *mockLogger) Ping(ctx context.Context) error {
	return nil
}

func TestPathHandling(t *testing.T) {
	// Create a new registry for this test
	registry := prometheus.NewRegistry()
	oldRequestsTotal := requestsTotal
	oldRequestDuration := requestDuration
	oldBytesTransferred := bytesTransferred
	oldActiveRequests := activeRequests
	oldErrorTotal := errorTotal
	oldObjectSize := objectSize
	oldGcsLatency := gcsLatency

	// Create new metrics with the test registry
	requestsTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_requests_total",
			Help: "Total number of requests handled by the GCS server",
		},
		[]string{"bucket_name", "path", "method", "status"},
	)
	requestDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucket_name", "path", "method"},
	)
	bytesTransferred = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_bytes_transferred_total",
			Help: "Total number of bytes transferred",
		},
		[]string{"bucket_name", "path", "method", "direction"},
	)
	activeRequests = promauto.With(registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcs_server_active_requests",
			Help: "Number of requests currently being served",
		},
		[]string{"bucket_name"},
	)
	errorTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_errors_total",
			Help: "Total number of errors encountered",
		},
		[]string{"bucket_name", "path", "code"},
	)
	objectSize = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_object_size_bytes",
			Help:    "Size of objects in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
		},
		[]string{"bucket_name", "path"},
	)
	gcsLatency = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_storage_operation_duration_seconds",
			Help:    "Duration of GCS operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucket_name", "operation"},
	)

	// Restore metrics after test
	defer func() {
		requestsTotal = oldRequestsTotal
		requestDuration = oldRequestDuration
		bytesTransferred = oldBytesTransferred
		activeRequests = oldActiveRequests
		errorTotal = oldErrorTotal
		objectSize = oldObjectSize
		gcsLatency = oldGcsLatency
	}()

	tests := []struct {
		name         string
		path         string
		expectedPath string
		expectedCode int
		objectExists bool
		contentType  string
		content      string
	}{
		{
			name:         "Root path",
			path:         "/",
			expectedPath: "index.html",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/html",
			content:      "<html><body>Index</body></html>",
		},
		{
			name:         "Directory path with trailing slash",
			path:         "/docs/",
			expectedPath: "docs/index.html",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/html",
			content:      "<html><body>Docs Index</body></html>",
		},
		{
			name:         "Directory path without trailing slash",
			path:         "/docs",
			expectedPath: "docs/",
			expectedCode: http.StatusMovedPermanently,
			objectExists: false,
		},
		{
			name:         "File in subdirectory",
			path:         "/css/styles.css",
			expectedPath: "css/styles.css",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/css",
			content:      "body { color: blue; }",
		},
		{
			name:         "Non-existent file",
			path:         "/notfound.html",
			expectedPath: "notfound.html",
			expectedCode: http.StatusNotFound,
			objectExists: false,
		},
		{
			name:         "Path with multiple slashes",
			path:         "//multiple///slashes/file.txt",
			expectedPath: "multiple/slashes/file.txt",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/plain",
			content:      "test content",
		},
		{
			name:         "Path with special characters",
			path:         "/path%20with%20spaces.txt",
			expectedPath: "path with spaces.txt",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/plain",
			content:      "content with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock store with test objects
			mockStore := &mockObjectStore{
				objects: make(map[string]mockObject),
			}

			if tt.objectExists {
				mockStore.objects[tt.expectedPath] = mockObject{
					data:        []byte(tt.content),
					contentType: tt.contentType,
				}
			}

			server := &gcsServer{
				store:      mockStore,
				bucketName: "test-bucket",
				logger:     &mockLogger{},
			}

			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if tt.expectedCode == http.StatusMovedPermanently {
				expectedLocation := tt.path + "/"
				if got := w.Header().Get("Location"); got != expectedLocation {
					t.Errorf("Expected Location %q, got %q", expectedLocation, got)
				}
			} else if tt.objectExists {
				if got := w.Header().Get("Content-Type"); got != tt.contentType {
					t.Errorf("Expected Content-Type %q, got %q", tt.contentType, got)
				}

				if got := w.Body.String(); got != tt.content {
					t.Errorf("Expected content %q, got %q", tt.content, got)
				}
			}
		})
	}
}

func TestCleanRequestPath(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		want         string
		wantErr      bool
		errContains  string
		wantRedirect bool
	}{
		{
			name:         "root path",
			path:         "/",
			want:         "index.html",
			wantErr:      false,
			wantRedirect: false,
		},
		{
			name:         "empty path",
			path:         "",
			want:         "index.html",
			wantErr:      false,
			wantRedirect: false,
		},
		{
			name:         "simple file",
			path:         "/file.txt",
			want:         "file.txt",
			wantErr:      false,
			wantRedirect: false,
		},
		{
			name:         "directory with trailing slash",
			path:         "/dir/",
			want:         "dir/index.html",
			wantErr:      false,
			wantRedirect: false,
		},
		{
			name:         "directory without trailing slash",
			path:         "/dir",
			want:         "dir/",
			wantErr:      false,
			wantRedirect: true,
		},
		{
			name:         "multiple slashes",
			path:         "//dir///file.txt",
			want:         "dir/file.txt",
			wantErr:      false,
			wantRedirect: false,
		},
		{
			name:         "directory traversal attempt",
			path:         "/dir/../file.txt",
			want:         "",
			wantErr:      true,
			errContains:  "directory traversal attempt",
			wantRedirect: false,
		},
		{
			name:         "URL encoded path",
			path:         "/dir/file%20name.txt",
			want:         "dir/file name.txt",
			wantErr:      false,
			wantRedirect: false,
		},
		{
			name:         "invalid URL encoding",
			path:         "/dir/file%2",
			want:         "",
			wantErr:      true,
			errContains:  "error decoding path",
			wantRedirect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err, needsRedirect := cleanRequestPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("cleanRequestPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("cleanRequestPath() error = %v, want error containing %q", err, tt.errContains)
				return
			}
			if got != tt.want {
				t.Errorf("cleanRequestPath() = %v, want %v", got, tt.want)
			}
			if needsRedirect != tt.wantRedirect {
				t.Errorf("cleanRequestPath() redirect = %v, want %v", needsRedirect, tt.wantRedirect)
			}
		})
	}
}

func TestGCSServer_ServeHTTP(t *testing.T) {
	// Create a new registry for this test
	oldRegistry := prometheus.DefaultRegisterer
	oldRequestsTotal := requestsTotal
	oldRequestDuration := requestDuration
	oldBytesTransferred := bytesTransferred
	oldActiveRequests := activeRequests
	oldErrorTotal := errorTotal
	oldObjectSize := objectSize
	oldGCSLatency := gcsLatency

	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	// Create new metrics with the test registry
	requestsTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"bucket_name", "path", "method", "code"},
	)

	requestDuration = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucket_name", "path", "method"},
	)

	bytesTransferred = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_bytes_transferred_total",
			Help: "Total number of bytes transferred",
		},
		[]string{"bucket_name", "path", "method", "direction"},
	)

	activeRequests = promauto.With(registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcs_server_active_requests",
			Help: "Number of requests currently being served",
		},
		[]string{"bucket_name"},
	)

	errorTotal = promauto.With(registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_errors_total",
			Help: "Total number of errors encountered",
		},
		[]string{"bucket_name", "path", "code"},
	)

	objectSize = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_object_size_bytes",
			Help:    "Size of objects in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
		},
		[]string{"bucket_name", "path"},
	)

	gcsLatency = promauto.With(registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_storage_operation_duration_seconds",
			Help:    "Duration of GCS operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucket_name", "operation"},
	)

	// Restore metrics after test
	defer func() {
		prometheus.DefaultRegisterer = oldRegistry
		requestsTotal = oldRequestsTotal
		requestDuration = oldRequestDuration
		bytesTransferred = oldBytesTransferred
		activeRequests = oldActiveRequests
		errorTotal = oldErrorTotal
		objectSize = oldObjectSize
		gcsLatency = oldGCSLatency
	}()

	tests := []struct {
		name         string
		path         string
		expectedPath string
		expectedCode int
		objectExists bool
		contentType  string
		content      string
	}{
		{
			name:         "Simple file request",
			path:         "/file.txt",
			expectedPath: "file.txt",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/plain",
			content:      "test content",
		},
		{
			name:         "Directory path with trailing slash",
			path:         "/docs/",
			expectedPath: "docs/index.html",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/html",
			content:      "<html>directory listing</html>",
		},
		{
			name:         "Directory path without trailing slash",
			path:         "/docs",
			expectedPath: "docs/",
			expectedCode: http.StatusMovedPermanently,
			objectExists: true,
			contentType:  "text/html",
			content:      "<html>directory listing</html>",
		},
		{
			name:         "File in subdirectory",
			path:         "/docs/file.txt",
			expectedPath: "docs/file.txt",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/plain",
			content:      "content in subdirectory",
		},
		{
			name:         "Non-existent file",
			path:         "/nonexistent.txt",
			expectedPath: "nonexistent.txt",
			expectedCode: http.StatusNotFound,
			objectExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock store with test objects
			mockStore := &mockObjectStore{
				objects: make(map[string]mockObject),
			}

			if tt.objectExists {
				mockStore.objects[tt.expectedPath] = mockObject{
					data:        []byte(tt.content),
					contentType: tt.contentType,
				}
			}

			server := &gcsServer{
				store:      mockStore,
				bucketName: "test-bucket",
				logger:     &mockLogger{},
			}

			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if tt.expectedCode == http.StatusMovedPermanently {
				expectedLocation := tt.path + "/"
				if got := w.Header().Get("Location"); got != expectedLocation {
					t.Errorf("Expected Location %q, got %q", expectedLocation, got)
				}
			} else if tt.objectExists {
				if got := w.Header().Get("Content-Type"); got != tt.contentType {
					t.Errorf("Expected Content-Type %q, got %q", tt.contentType, got)
				}

				if got := w.Body.String(); got != tt.content {
					t.Errorf("Expected content %q, got %q", tt.content, got)
				}
			}
		})
	}
}
