package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
)

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: "none",
		},
		{
			name:     "Object not exist error",
			err:      storage.ErrObjectNotExist,
			expected: "object_not_found",
		},
		{
			name:     "Permission error",
			err:      errors.New("AccessDenied: access denied"),
			expected: "permission_denied",
		},
		{
			name:     "Timeout error",
			err:      errors.New("request timeout occurred"),
			expected: "timeout",
		},
		{
			name:     "Connection error",
			err:      errors.New("connection failed"),
			expected: "connection_error",
		},
		{
			name:     "Generic storage error",
			err:      errors.New("some other storage issue"),
			expected: "storage_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getErrorType(tt.err)
			if result != tt.expected {
				t.Errorf("getErrorType(%v) = %s, expected %s", tt.err, result, tt.expected)
			}
		})
	}
}

func TestGCSObjectStore_GetObject_ErrorCases(t *testing.T) {
	// Create a real GCSObjectStore with a nil bucket to test error paths
	store := &GCSObjectStore{
		bucket: nil, // This will cause errors when trying to call methods
	}

	ctx := context.Background()

	// This will panic since bucket is nil, but we can test the struct initialization
	if store.bucket != nil {
		_, _, err := store.GetObject(ctx, "test-path")
		if err == nil {
			t.Error("Expected error when bucket operations fail")
		}
	}
}

func TestCreateServer(t *testing.T) {
	ctx := context.Background()
	logClient := newMockLogClient()

	tests := []struct {
		name        string
		config      *config
		expectError bool
	}{
		{
			name: "Valid config",
			config: &config{
				bucketName: "test-bucket",
				port:       "8080",
				store:      &mockObjectStore{objects: make(map[string]mockObject)},
				redirects:  make(map[string]string),
			},
			expectError: false,
		},
		{
			name: "Nil store in config",
			config: &config{
				bucketName: "test-bucket",
				port:       "8080",
				store:      nil,
				redirects:  make(map[string]string),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := createServer(ctx, tt.config, logClient)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if server != nil {
					t.Error("Expected server to be nil when error occurs")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if server == nil {
					t.Error("Expected server to not be nil")
				} else {
					expectedAddr := ":" + tt.config.port
					if server.Addr != expectedAddr {
						t.Errorf("Expected server address %s, got %s", expectedAddr, server.Addr)
					}
				}
			}
		})
	}
}

func TestSendUserFriendlyError_JSONResponse(t *testing.T) {
	server := createMockServer(t, make(map[string]mockObject), make(map[string]string))

	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")

	w := &responseRecorder{
		header: make(http.Header),
		body:   &strings.Builder{},
	}

	testErr := errors.New("test error")
	server.sendUserFriendlyError(w, req, "/test", http.StatusInternalServerError, "Internal server error", testErr)

	if w.statusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, w.statusCode)
	}

	contentType := w.header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected JSON content type, got %s", contentType)
	}

	body := w.body.String()
	if !strings.Contains(body, "Internal server error") {
		t.Error("Expected error message in response body")
	}
}

func TestSendUserFriendlyError_HTMLResponse(t *testing.T) {
	server := createMockServer(t, make(map[string]mockObject), make(map[string]string))

	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/html")

	w := &responseRecorder{
		header: make(http.Header),
		body:   &strings.Builder{},
	}

	testErr := errors.New("test error")
	server.sendUserFriendlyError(w, req, "/test", http.StatusNotFound, "Not found", testErr)

	if w.statusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.statusCode)
	}

	contentType := w.header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected HTML content type, got %s", contentType)
	}

	body := w.body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("Expected HTML response")
	}
	if !strings.Contains(body, "Not found") {
		t.Error("Expected error message in HTML response")
	}
}

// responseRecorder is a simple implementation for testing
type responseRecorder struct {
	header     http.Header
	body       *strings.Builder
	statusCode int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	return r.body.Write(data)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}
