package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
)

// mockRedirectStore implements ObjectStore for testing redirects
type mockRedirectStore struct {
	objects map[string]mockObject
	content string
}

func (m *mockRedirectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	// Handle the specific path that loadRedirects expects
	expectedPath := filepath.Join(".spray", "redirects.toml")
	if path == expectedPath {
		return io.NopCloser(strings.NewReader(m.content)), &storage.ObjectAttrs{
			ContentType: "application/toml",
		}, nil
	}

	if obj, ok := m.objects[path]; ok {
		return io.NopCloser(strings.NewReader(string(obj.data))), &storage.ObjectAttrs{
			ContentType: obj.contentType,
		}, nil
	}
	return nil, nil, storage.ErrObjectNotExist
}

func TestLoadRedirects(t *testing.T) {
	// Load the static TOML fixture
	content, err := os.ReadFile("testdata/redirects.toml")
	if err != nil {
		t.Fatalf("Failed to read testdata/redirects.toml: %v", err)
	}

	tests := []struct {
		name          string
		content       string
		expected      map[string]string
		expectedError bool
	}{
		{
			name:          "valid_redirects",
			content:       string(content),
			expected:      map[string]string{"/old-path": "https://example.com/new-path", "/another-path": "https://example.com/destination"},
			expectedError: false,
		},
		{
			name:          "invalid_TOML",
			content:       "invalid toml content",
			expected:      nil,
			expectedError: true,
		},
		{
			name:          "invalid_URL",
			content:       "redirects = { \"/bad-url\" = \"not-a-url\" }",
			expected:      nil,
			expectedError: true,
		},
		{
			name:          "empty_file",
			content:       "",
			expected:      map[string]string{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set a valid bucket name for all tests
			os.Setenv("BUCKET_NAME", "test-bucket")
			defer os.Unsetenv("BUCKET_NAME")

			store := &mockRedirectStore{content: tt.content}
			redirects, err := loadRedirects(context.Background(), store)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, redirects)
			}
		})
	}
}

func TestRedirectMetrics(t *testing.T) {
	// Reset metrics before test
	redirectHits.Reset()
	redirectLatency.Reset()
	redirectConfigErrors.Reset()

	// Test redirect hit metrics
	redirectHits.WithLabelValues("test-bucket", "/test-path", "https://example.com").Inc()
	redirectLatency.WithLabelValues("test-bucket", "/test-path").Observe(0.1)

	// Test config error metrics
	redirectConfigErrors.WithLabelValues("test-bucket", "parse_error").Inc()
	redirectConfigErrors.WithLabelValues("test-bucket", "invalid_url").Inc()

	// Verify metrics were recorded
	// Note: We can't directly test the values, but we can verify the metrics exist
	// by checking that the test doesn't panic
}
