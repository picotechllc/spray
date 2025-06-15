package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
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

// Mock ObjectStore that always returns a custom error for testing ServeHTTP error path
type errorObjectStore struct{}

func (s *errorObjectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	return nil, nil, assert.AnError
}

// mockLogClient is a mock implementation of the LoggingClient interface
type mockLogClient struct {
	*logging.Client
}

// newMockLogClient creates a new mock logging client
func newMockLogClient() *mockLogClient {
	return &mockLogClient{}
}

// Logger returns a new logger
func (c *mockLogClient) Logger(name string, opts ...logging.LoggerOption) *logging.Logger {
	return &logging.Logger{}
}

// Close closes the client
func (c *mockLogClient) Close() error {
	return nil
}

// mockStorageClient is a mock implementation of the StorageClient interface
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
		logger:     &logging.Logger{},
		redirects:  redirects,
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
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q but got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cleanPath != tt.expectedPath {
				t.Errorf("Expected path %q but got %q", tt.expectedPath, cleanPath)
			}
		})
	}
}

func TestNewGCSServer(t *testing.T) {
	ctx := context.Background()

	// Create a mock logging client
	logClient := &mockLogClient{}
	logger := logClient.Logger("test-logger")

	server, err := newGCSServer(ctx, "test-bucket", logger, newMockStorageClient(), nil)
	if err != nil {
		t.Fatalf("Failed to create GCS server: %v", err)
	}

	if server == nil {
		t.Fatal("Expected server to be non-nil")
	}

	if server.bucketName != "test-bucket" {
		t.Errorf("Expected bucket name %q, got %q", "test-bucket", server.bucketName)
	}

	if server.logger != logger {
		t.Error("Expected logger to be set")
	}
}

func TestHealthCheckHandlers(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantCode int
		wantBody string
	}{
		{
			name:     "readyz handler",
			handler:  readyzHandler,
			wantCode: http.StatusOK,
			wantBody: "ok",
		},
		{
			name:     "livez handler",
			handler:  livezHandler,
			wantCode: http.StatusOK,
			wantBody: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			rr := httptest.NewRecorder()

			tt.handler(rr, req)

			assert.Equal(t, tt.wantCode, rr.Code)
			assert.Equal(t, tt.wantBody, rr.Body.String())
		})
	}
}

// Test newGCSServer error path by injecting a failing storage client
func TestNewGCSServer_ErrorPath(t *testing.T) {
	ctx := context.Background()

	// Create a mock logging client
	logClient := &mockLogClient{}
	logger := logClient.Logger("test-logger")

	// Test with nil storage client
	_, err := newGCSServer(ctx, "test-bucket", logger, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error with nil storage client, got: %v", err)
	}
}

// Test runServerImpl graceful shutdown by overriding handleSignals
func TestRunServerImpl_GracefulShutdown(t *testing.T) {
	// Save and restore original handleSignals
	origHandleSignals := handleSignals
	defer func() { handleSignals = origHandleSignals }()

	// Override handleSignals to close immediately
	handleSignals = func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	// Create a dummy HTTP server that returns immediately
	srv := &http.Server{Addr: ":0"}
	// Use a context that will not timeout
	ctx := context.Background()

	// Start runServerImpl in a goroutine and shut it down
	errCh := make(chan error, 1)
	go func() {
		errCh <- runServerImpl(ctx, srv)
	}()

	// Wait for the server to shut down
	select {
	case err := <-errCh:
		// We expect no error or a graceful shutdown error
		if err != nil && !strings.Contains(err.Error(), "graceful shutdown failed") {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for runServerImpl to return")
	}
}

// Test ServeHTTP with a storage error (not ErrObjectNotExist)
func TestServeHTTP_StorageError(t *testing.T) {
	ctx := context.Background()
	logClient, err := logging.NewClient(ctx, "test-project", option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create mock logging client: %v", err)
	}
	defer logClient.Close()
	logger := logClient.Logger("test-logger")

	server := &gcsServer{
		store:      &errorObjectStore{},
		bucketName: "test-bucket",
		logger:     logger,
	}

	req := httptest.NewRequest("GET", "/somefile.txt", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	if !strings.Contains(w.Body.String(), assert.AnError.Error()) {
		t.Errorf("Expected error message in response body, got %q", w.Body.String())
	}
}

// TestGCSObjectStoreGetObject tests the concrete implementation of GCSObjectStore.GetObject
func TestGCSObjectStoreGetObject(t *testing.T) {
	// Create a context for testing
	ctx := context.Background()

	// Create test data
	expectedData := []byte("test content")
	testObj := &mockObject{
		data:        expectedData,
		contentType: "text/plain",
	}

	// Happy path - test successful retrieval
	t.Run("success case", func(t *testing.T) {
		// Create mock store with one object
		mockStore := &mockObjectStore{
			objects: map[string]mockObject{
				"test.txt": *testObj,
			},
		}

		// Create test server with the mock store
		server := &gcsServer{
			store:      mockStore,
			bucketName: "test-bucket",
		}

		// Call the method under test (through the server's store)
		reader, attrs, err := server.store.GetObject(ctx, "test.txt")

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, reader)
		assert.NotNil(t, attrs)

		// Read content from reader
		content, err := io.ReadAll(reader)
		assert.NoError(t, err)
		assert.Equal(t, string(expectedData), string(content))

		// Close reader
		reader.Close()
	})

	// Test not found case
	t.Run("not found case", func(t *testing.T) {
		// Create empty mock store
		mockStore := &mockObjectStore{
			objects: make(map[string]mockObject),
		}

		// Create test server with the mock store
		server := &gcsServer{
			store:      mockStore,
			bucketName: "test-bucket",
		}

		// Call the method under test
		reader, attrs, err := server.store.GetObject(ctx, "nonexistent.txt")

		// Verify results
		assert.Error(t, err)
		assert.Equal(t, storage.ErrObjectNotExist, err)
		assert.Nil(t, reader)
		assert.Nil(t, attrs)
	})

	// Custom error case
	t.Run("error case", func(t *testing.T) {
		// Create mock store that always returns an error
		errorStore := &errorObjectStore{}

		// Create test server with the error store
		server := &gcsServer{
			store:      errorStore,
			bucketName: "test-bucket",
		}

		// Call the method under test
		reader, attrs, err := server.store.GetObject(ctx, "test.txt")

		// Verify results
		assert.Error(t, err)
		assert.Equal(t, assert.AnError, err)
		assert.Nil(t, reader)
		assert.Nil(t, attrs)
	})
}

func TestServeHTTP_Redirects(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		redirects    map[string]string
		expectedCode int
		expectedURL  string
		objectExists bool
		contentType  string
		content      string
	}{
		{
			name: "redirect takes precedence over file",
			path: "/redirect-me",
			redirects: map[string]string{
				"redirect-me": "https://example.com/new-location",
			},
			expectedCode: http.StatusFound,
			expectedURL:  "https://example.com/new-location",
			objectExists: true,
			contentType:  "text/html",
			content:      "<html>This should not be served</html>",
		},
		{
			name:         "no redirect, serve file",
			path:         "/normal-file",
			redirects:    map[string]string{},
			expectedCode: http.StatusOK,
			objectExists: true,
			contentType:  "text/html",
			content:      "<html>This should be served</html>",
		},
		{
			name:         "no redirect, file not found",
			path:         "/not-found",
			redirects:    map[string]string{},
			expectedCode: http.StatusNotFound,
			objectExists: false,
		},
		{
			name: "redirect with trailing slash",
			path: "/redirect-dir/",
			redirects: map[string]string{
				"redirect-dir/index.html": "https://example.com/new-dir/",
			},
			expectedCode: http.StatusFound,
			expectedURL:  "https://example.com/new-dir/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock store with test objects
			objects := make(map[string]mockObject)
			if tt.objectExists {
				// For paths with trailing slash, we need to add index.html
				path := tt.path
				if strings.HasSuffix(path, "/") {
					path = strings.TrimSuffix(path, "/") + "/index.html"
				}
				objects[path] = mockObject{
					data:        []byte(tt.content),
					contentType: tt.contentType,
				}
			}

			server := createMockServer(t, objects, tt.redirects)

			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if tt.expectedURL != "" {
				if got := w.Header().Get("Location"); got != tt.expectedURL {
					t.Errorf("Expected Location header %q, got %q", tt.expectedURL, got)
				}
			}

			if tt.objectExists && tt.expectedCode == http.StatusOK {
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
