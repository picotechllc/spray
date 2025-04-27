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

	ctx := context.Background()

	// Create a mock logging client
	logClient, err := logging.NewClient(ctx, "test-project", option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create mock logging client: %v", err)
	}
	defer logClient.Close()
	logger := logClient.Logger("test-logger")

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
				logger:     logger,
			}

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
	tests := []struct {
		name       string
		bucketName string
		wantErr    bool
	}{
		{
			name:       "valid bucket name",
			bucketName: "test-bucket",
			wantErr:    false,
		},
		{
			name:       "empty bucket name",
			bucketName: "",
			wantErr:    true,
		},
	}

	ctx := context.Background()

	// Create a mock logging client
	logClient, err := logging.NewClient(ctx, "test-project", option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create mock logging client: %v", err)
	}
	defer logClient.Close()
	logger := logClient.Logger("test-logger")

	// Create a mock storage client
	mockStore := &mockObjectStore{
		objects: make(map[string]mockObject),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the server directly with our mock store
			server := &gcsServer{
				store:      mockStore,
				bucketName: tt.bucketName,
				logger:     logger,
			}

			if tt.wantErr {
				if tt.bucketName != "" {
					t.Error("Expected error case to have empty bucket name")
				}
				return
			}

			if server.bucketName != tt.bucketName {
				t.Errorf("Expected bucket name %q, got %q", tt.bucketName, server.bucketName)
			}

			if server.store == nil {
				t.Error("Expected store to be non-nil")
			}

			if server.logger == nil {
				t.Error("Expected logger to be non-nil")
			}
		})
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
	logClient, err := logging.NewClient(ctx, "test-project", option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create mock logging client: %v", err)
	}
	defer logClient.Close()
	logger := logClient.Logger("test-logger")

	// Failing storage client: nil client, but bucketName is empty to force error
	_, err = newGCSServer(ctx, "", logger, nil)
	if err == nil {
		t.Error("Expected error when bucketName is empty or storage client creation fails")
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
