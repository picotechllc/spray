package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
)

// mockObject represents a mock object in the store
type mockObject struct {
	data        []byte
	contentType string
}

// mockReader implements io.ReadCloser for testing
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

// mockObjectStore implements ObjectStore for testing
type mockObjectStore struct {
	objects map[string]mockObject
}

func (s *mockObjectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	if obj, ok := s.objects[path]; ok {
		return &mockReader{data: obj.data}, &storage.ObjectAttrs{
			ContentType: obj.contentType,
		}, nil
	}
	return nil, nil, storage.ErrObjectNotExist
}

// errorObjectStore implements ObjectStore and always returns an error
type errorObjectStore struct{}

func (s *errorObjectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	return nil, nil, assert.AnError
}

// mockStorageClient implements the StorageClient interface for testing
type mockStorageClient struct {
	objects map[string]mockObject
}

// newMockStorageClient creates a new mock storage client
func newMockStorageClient() *mockStorageClient {
	return &mockStorageClient{
		objects: make(map[string]mockObject),
	}
}

// Bucket returns a mock bucket
func (c *mockStorageClient) Bucket(name string) *storage.BucketHandle {
	if name == "" {
		return nil
	}
	return &storage.BucketHandle{}
}

// GetObject returns a mock object
func (c *mockStorageClient) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	if _, ok := c.objects[path]; ok {
		return io.NopCloser(strings.NewReader("")), &storage.ObjectAttrs{}, nil
	}
	return nil, nil, storage.ErrObjectNotExist
}

// ListObjects returns a mock object iterator
func (c *mockStorageClient) ListObjects(ctx context.Context, prefix string) *storage.ObjectIterator {
	return &storage.ObjectIterator{}
}

// Close closes the client
func (c *mockStorageClient) Close() error {
	return nil
}

// Helper function to create a mock server for testing
func createMockServer(t *testing.T, objects map[string]mockObject, redirects map[string]string) *gcsServer {
	store := &mockObjectStore{
		objects: objects,
	}

	return &gcsServer{
		store:      store,
		bucketName: "test-bucket",
		redirects:  redirects,
		logger:     &mockLogger{},
	}
}

func TestPathHandling(t *testing.T) {
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
			expectedPath: "docs",
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/html",
			content:      "<html><body>Docs</body></html>",
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
			objects := make(map[string]mockObject)
			if tt.objectExists {
				objects[tt.expectedPath] = mockObject{
					data:        []byte(tt.content),
					contentType: tt.contentType,
				}
			}

			server := createMockServer(t, objects, nil)

			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if tt.objectExists {
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
		name          string
		path          string
		expectedPath  string
		expectError   bool
		errorContains string
	}{
		{
			name:         "Root path",
			path:         "/",
			expectedPath: "index.html",
		},
		{
			name:         "Directory path with trailing slash",
			path:         "/docs/",
			expectedPath: "docs/index.html",
		},
		{
			name:         "Directory path without trailing slash",
			path:         "/docs",
			expectedPath: "docs",
		},
		{
			name:         "File in subdirectory",
			path:         "/css/styles.css",
			expectedPath: "css/styles.css",
		},
		{
			name:         "Multiple slashes",
			path:         "//multiple///slashes/file.txt",
			expectedPath: "multiple/slashes/file.txt",
		},
		{
			name:         "URL encoded path",
			path:         "/path%20with%20spaces.txt",
			expectedPath: "path with spaces.txt",
		},
		{
			name:         "Empty path",
			path:         "",
			expectedPath: "index.html",
		},
		{
			name:          "Directory traversal attempt",
			path:          "../secret.txt",
			expectError:   true,
			errorContains: "directory traversal",
		},
		{
			name:          "Directory traversal with encoded slashes",
			path:          "/..%2F..%2Fsecret.txt",
			expectError:   true,
			errorContains: "directory traversal",
		},
		{
			name:         "Deep nested path",
			path:         "/a/b/c/d/file.txt",
			expectedPath: "a/b/c/d/file.txt",
		},
		{
			name:         "All slashes",
			path:         "////",
			expectedPath: "index.html",
		},
		{
			name:          "Invalid percent encoding",
			path:          "/%ZZ",
			expectError:   true,
			errorContains: "invalid URL escape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanPath, err := cleanRequestPath(tt.path)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cleanPath != tt.expectedPath {
				t.Errorf("Expected path %q, got %q", tt.expectedPath, cleanPath)
			}
		})
	}
}

func TestNewGCSServer(t *testing.T) {
	// Test successful server creation
	store := &mockObjectStore{objects: make(map[string]mockObject)}
	server, err := newGCSServer(context.Background(), "test-bucket", &mockLogger{}, store, nil)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, "test-bucket", server.bucketName)
	assert.Equal(t, store, server.store)

	// Test nil store error
	server, err = newGCSServer(context.Background(), "test-bucket", &mockLogger{}, nil, nil)
	assert.Error(t, err)
	assert.Nil(t, server)
}

func TestHealthCheckHandlers(t *testing.T) {
	// Test readyz handler
	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	readyzHandler(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())

	// Test livez handler
	req = httptest.NewRequest("GET", "/livez", nil)
	w = httptest.NewRecorder()
	livezHandler(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

func TestNewGCSServer_ErrorPath(t *testing.T) {
	// Test with nil store
	server, err := newGCSServer(context.Background(), "test-bucket", &mockLogger{}, nil, nil)
	assert.Error(t, err)
	assert.Nil(t, server)
}

func TestRunServerImpl_GracefulShutdown(t *testing.T) {
	// Create a test server
	srv := &http.Server{
		Addr: ":0", // Use port 0 to get a random available port
	}

	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start the server in a goroutine
	go func() {
		err := runServerImpl(ctx, srv)
		assert.NoError(t, err)
	}()

	// Wait a bit for the server to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to trigger graceful shutdown
	cancel()

	// Wait a bit for the server to stop
	time.Sleep(100 * time.Millisecond)
}

func TestServeHTTP_StorageError(t *testing.T) {
	// Create a server with an error store
	server := &gcsServer{
		store:      &errorObjectStore{},
		bucketName: "test-bucket",
		logger:     &mockLogger{},
	}

	// Test with a valid path
	req := httptest.NewRequest("GET", "/test.txt", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGCSObjectStoreGetObject(t *testing.T) {
	// Set a valid bucket name for all tests
	os.Setenv("BUCKET_NAME", "test-bucket")
	defer os.Unsetenv("BUCKET_NAME")

	store := &mockStorageClient{objects: make(map[string]mockObject)}
	store.objects["test-object"] = mockObject{
		data:        []byte("test data"),
		contentType: "text/plain",
	}

	reader, contentType, err := store.GetObject(context.Background(), "test-object")
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	assert.Equal(t, "text/plain", contentType)

	// Test non-existent object
	_, _, err = store.GetObject(context.Background(), "non-existent")
	assert.Error(t, err)
	assert.Equal(t, storage.ErrObjectNotExist, err)
}

func TestServeHTTP_Redirects(t *testing.T) {
	// Create a server with redirects
	redirects := map[string]string{
		"/old-path": "https://example.com/new-path",
	}
	server := &gcsServer{
		store:      &mockObjectStore{objects: make(map[string]mockObject)},
		bucketName: "test-bucket",
		redirects:  redirects,
		logger:     &mockLogger{},
	}

	// Test redirect
	req := httptest.NewRequest("GET", "/old-path", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "https://example.com/new-path", w.Header().Get("Location"))

	// Test non-redirect path
	req = httptest.NewRequest("GET", "/normal-path", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
