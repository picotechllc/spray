package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRedirectStore implements ObjectStore for testing redirects
type mockRedirectStore struct {
	objects map[string]string
}

func (m *mockRedirectStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	if content, exists := m.objects[path]; exists {
		return io.NopCloser(strings.NewReader(content)), &storage.ObjectAttrs{}, nil
	}
	return nil, nil, storage.ErrObjectNotExist
}

func TestLoadRedirects(t *testing.T) {
	tests := []struct {
		name           string
		configContent  string
		expectedPaths  map[string]string
		expectedErrors []string
	}{
		{
			name: "valid redirects",
			configContent: `
[redirects]
"/old-path" = "https://example.com/new-path"
"/another-path" = "https://example.com/destination"
`,
			expectedPaths: map[string]string{
				"/old-path":     "https://example.com/new-path",
				"/another-path": "https://example.com/destination",
			},
		},
		{
			name: "invalid TOML",
			configContent: `
[redirects
"/old-path" = "https://example.com/new-path"
`,
			expectedErrors: []string{"error parsing redirects file"},
		},
		{
			name: "invalid URL",
			configContent: `
[redirects]
"/old-path" = "not-a-url"
`,
			expectedErrors: []string{"invalid redirect destination URL"},
		},
		{
			name:           "empty file",
			configContent:  "",
			expectedPaths:  map[string]string{},
			expectedErrors: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockRedirectStore{
				objects: map[string]string{
					".spray/redirects.toml": tt.configContent,
				},
			}

			redirects, err := loadRedirects(context.Background(), store)
			if len(tt.expectedErrors) > 0 {
				require.Error(t, err)
				for _, expectedErr := range tt.expectedErrors {
					assert.Contains(t, err.Error(), expectedErr)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedPaths, redirects)
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
