package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
)

func TestLogStructuredWarning(t *testing.T) {
	// Capture stderr output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	testErr := errors.New("access denied")

	// Call the function
	logStructuredWarning("test_operation", "/test/path", testErr)

	// Close writer and read output
	w.Close()
	os.Stderr = oldStderr

	output, _ := io.ReadAll(r)

	// Verify JSON structure
	var logEntry map[string]interface{}
	if err := json.Unmarshal(output, &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	// Check required fields
	if logEntry["severity"] != "WARNING" {
		t.Errorf("Expected severity 'WARNING', got %v", logEntry["severity"])
	}

	if logEntry["operation"] != "test_operation" {
		t.Errorf("Expected operation 'test_operation', got %v", logEntry["operation"])
	}

	if logEntry["path"] != "/test/path" {
		t.Errorf("Expected path '/test/path', got %v", logEntry["path"])
	}

	if logEntry["error"] != "access denied" {
		t.Errorf("Expected error 'access denied', got %v", logEntry["error"])
	}

	if logEntry["error_type"] != "access_denied" {
		t.Errorf("Expected error_type 'access_denied', got %v", logEntry["error_type"])
	}

	expectedMessage := "Cannot access /test/path due to access/authentication error"
	if logEntry["message"] != expectedMessage {
		t.Errorf("Expected message '%s', got %v", expectedMessage, logEntry["message"])
	}
}

func TestLogStructuredWarning_NilError(t *testing.T) {
	// Capture stderr output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Call the function with nil error
	logStructuredWarning("test_operation", "/test/path", nil)

	// Close writer and read output
	w.Close()
	os.Stderr = oldStderr

	output, _ := io.ReadAll(r)

	// Verify JSON structure
	var logEntry map[string]interface{}
	if err := json.Unmarshal(output, &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	// Check that error_type is not set when error is nil
	if _, exists := logEntry["error"]; exists {
		t.Error("Expected no 'error' field when error is nil")
	}
	if _, exists := logEntry["error_type"]; exists {
		t.Error("Expected no 'error_type' field when error is nil")
	}
}

func TestIsPermissionError_VariousCases(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "AccessDenied error",
			err:      errors.New("AccessDenied: user does not have permission"),
			expected: true,
		},
		{
			name:     "Access denied error",
			err:      errors.New("Access denied to resource"),
			expected: true,
		},
		{
			name:     "permission denied error",
			err:      errors.New("permission denied"),
			expected: true,
		},
		{
			name:     "403 error",
			err:      errors.New("HTTP 403 forbidden"),
			expected: true,
		},
		{
			name:     "401 error",
			err:      errors.New("HTTP 401 unauthorized"),
			expected: true,
		},
		{
			name:     "GCE metadata error",
			err:      errors.New("metadata: GCE metadata not defined"),
			expected: true,
		},
		{
			name:     "Credentials error",
			err:      errors.New("invalid credentials provided"),
			expected: true,
		},
		{
			name:     "Token error",
			err:      errors.New("token expired"),
			expected: true,
		},
		{
			name:     "Other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPermissionError(tt.err)
			if result != tt.expected {
				t.Errorf("isPermissionError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestLoadRedirects_PermissionError(t *testing.T) {
	ctx := context.Background()

	// Create a mock store that returns permission error
	store := &mockPermissionErrorStore{}

	// Capture stderr output to test logStructuredWarning call
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Call loadRedirects - should handle permission error gracefully
	redirects, err := loadRedirects(ctx, store)

	// Close writer and read output
	w.Close()
	os.Stderr = oldStderr

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	// Should return empty map and no error when permission denied
	if err != nil {
		t.Errorf("Expected no error when permission denied, got: %v", err)
	}

	if redirects == nil {
		t.Error("Expected non-nil redirects map")
	}

	if len(redirects) != 0 {
		t.Errorf("Expected empty redirects map, got %d entries", len(redirects))
	}

	// Verify that logStructuredWarning was called (JSON output should be present)
	if !strings.Contains(outputStr, "load_redirects") {
		t.Error("Expected logStructuredWarning to be called for permission error")
	}
}

// mockPermissionErrorStore returns permission errors for testing
type mockPermissionErrorStore struct{}

func (s *mockPermissionErrorStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	return nil, nil, errors.New("AccessDenied: permission denied to resource")
}

func TestLoadRedirects_NotFoundError(t *testing.T) {
	ctx := context.Background()

	// Create a mock store that returns not found error
	store := &mockNotFoundStore{}

	redirects, err := loadRedirects(ctx, store)

	// Should return empty map and no error when file not found
	if err != nil {
		t.Errorf("Expected no error when file not found, got: %v", err)
	}

	if redirects == nil {
		t.Error("Expected non-nil redirects map")
	}

	if len(redirects) != 0 {
		t.Errorf("Expected empty redirects map, got %d entries", len(redirects))
	}
}

// mockNotFoundStore returns not found errors for testing
type mockNotFoundStore struct{}

func (s *mockNotFoundStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	return nil, nil, storage.ErrObjectNotExist
}
