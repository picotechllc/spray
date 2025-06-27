package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
)

// mockHeaderStore implements ObjectStore for testing headers
type mockHeaderStore struct {
	content string
}

func (m *mockHeaderStore) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	// Handle the specific path that loadHeaders expects
	expectedPath := filepath.Join(".spray", "headers.toml")
	if path == expectedPath {
		return io.NopCloser(strings.NewReader(m.content)), &storage.ObjectAttrs{
			ContentType: "application/toml",
		}, nil
	}
	return nil, nil, storage.ErrObjectNotExist
}

func TestLoadHeaders(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expected      *HeaderConfig
		expectedError bool
	}{
		{
			name:    "valid_headers_disabled",
			content: "[powered_by]\nenabled = false",
			expected: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: false},
			},
			expectedError: false,
		},
		{
			name:    "valid_headers_enabled",
			content: "[powered_by]\nenabled = true",
			expected: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			},
			expectedError: false,
		},
		{
			name:          "invalid_toml",
			content:       "invalid toml content",
			expected:      nil,
			expectedError: true,
		},
		{
			name:    "empty_file",
			content: "",
			expected: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: false}, // Default value when not specified
			},
			expectedError: false,
		},
		{
			name:    "partial_config",
			content: "# No powered_by section",
			expected: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: false}, // Default value when not specified
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockHeaderStore{content: tt.content}
			headers, err := loadHeaders(context.Background(), store)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, headers)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, headers)
			}
		})
	}
}

func TestLoadHeaders_FileNotExists(t *testing.T) {
	// Test when headers.toml doesn't exist - should return default config (enabled)
	store := &mockObjectStore{objects: make(map[string]mockObject)}
	headers, err := loadHeaders(context.Background(), store)

	assert.NoError(t, err)
	expected := getDefaultHeaderConfig()
	assert.Equal(t, expected, headers)
}

func TestResolveXPoweredByHeader(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		envExists    bool
		headerConfig *HeaderConfig
		version      string
		expected     string
	}{
		{
			name:      "default_no_env_var",
			envExists: false,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			},
			version:  "1.0.0",
			expected: "spray/1.0.0",
		},
		{
			name:      "custom_env_var",
			envValue:  "MyCompany-CDN/spray",
			envExists: true,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			},
			version:  "1.0.0",
			expected: "MyCompany-CDN/spray",
		},
		{
			name:      "env_var_disabled",
			envValue:  "",
			envExists: true,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			},
			version:  "1.0.0",
			expected: "",
		},
		{
			name:      "site_owner_disabled",
			envExists: false,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: false},
			},
			version:  "1.0.0",
			expected: "",
		},
		{
			name:      "env_var_set_but_site_disabled",
			envValue:  "MyCompany-CDN/spray",
			envExists: true,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: false},
			},
			version:  "1.0.0",
			expected: "",
		},
		{
			name:         "nil_header_config",
			envExists:    false,
			headerConfig: nil,
			version:      "1.0.0",
			expected:     "spray/1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable
			if tt.envExists {
				os.Setenv("SPRAY_POWERED_BY_HEADER", tt.envValue)
			} else {
				os.Unsetenv("SPRAY_POWERED_BY_HEADER")
			}
			defer os.Unsetenv("SPRAY_POWERED_BY_HEADER")

			result := resolveXPoweredByHeader(tt.headerConfig, tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadHeaders_PermissionError(t *testing.T) {
	ctx := context.Background()

	// Create a mock store that returns permission error
	store := &mockPermissionErrorStore{}

	// Capture stderr output to test logStructuredWarning call
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Call loadHeaders - should handle permission error gracefully
	headers, err := loadHeaders(ctx, store)

	// Close writer and read output
	w.Close()
	os.Stderr = oldStderr

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	// Should return default config and no error when permission denied
	assert.NoError(t, err)
	assert.NotNil(t, headers)
	expected := getDefaultHeaderConfig()
	assert.Equal(t, expected, headers)

	// Verify that logStructuredWarning was called (JSON output should be present)
	assert.Contains(t, outputStr, "load_headers")
}

func TestXPoweredByHeaderIntegration(t *testing.T) {
	tests := []struct {
		name                string
		envValue            string
		envExists           bool
		headerConfig        *HeaderConfig
		expectedHeaderValue string
		expectHeader        bool
	}{
		{
			name:      "default_enabled",
			envExists: false,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			},
			expectedHeaderValue: "spray/dev", // Using default Version
			expectHeader:        true,
		},
		{
			name:      "custom_env_value",
			envValue:  "MyCompany-CDN/spray",
			envExists: true,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			},
			expectedHeaderValue: "MyCompany-CDN/spray",
			expectHeader:        true,
		},
		{
			name:      "disabled_by_env",
			envValue:  "",
			envExists: true,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			},
			expectHeader: false,
		},
		{
			name:      "disabled_by_site_owner",
			envExists: false,
			headerConfig: &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: false},
			},
			expectHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable
			if tt.envExists {
				os.Setenv("SPRAY_POWERED_BY_HEADER", tt.envValue)
			} else {
				os.Unsetenv("SPRAY_POWERED_BY_HEADER")
			}
			defer os.Unsetenv("SPRAY_POWERED_BY_HEADER")

			// Create a test server with the header config
			objects := map[string]mockObject{
				"test.txt": {
					data:        []byte("Hello, World!"),
					contentType: "text/plain",
				},
			}

			store := &mockObjectStore{objects: objects}
			server := &gcsServer{
				store:      store,
				bucketName: "test-bucket",
				redirects:  make(map[string]string),
				headers:    tt.headerConfig,
				logger:     &mockLogger{},
			}

			// Make a test request
			req := httptest.NewRequest("GET", "/test.txt", nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			// Check the response
			assert.Equal(t, http.StatusOK, w.Code)

			// Check the X-Powered-By header
			if tt.expectHeader {
				assert.Equal(t, tt.expectedHeaderValue, w.Header().Get("X-Powered-By"))
			} else {
				assert.Empty(t, w.Header().Get("X-Powered-By"))
			}
		})
	}
}
