package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		basePort   string
		expectPort string
		envs       map[string]string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid config with default port",
			expectPort: "8080",
			envs: map[string]string{
				"BUCKET_NAME":       "test-bucket",
				"GOOGLE_PROJECT_ID": "test-project",
			},
		},
		{
			name:       "valid config with custom port",
			basePort:   "9090",
			expectPort: "9090",
			envs: map[string]string{
				"BUCKET_NAME":       "test-bucket",
				"GOOGLE_PROJECT_ID": "test-project",
			},
		},
		{
			name: "missing bucket name",
			envs: map[string]string{
				"GOOGLE_PROJECT_ID": "test-project",
			},
			wantErr: true,
			errMsg:  "BUCKET_NAME environment variable is required",
		},
		{
			name: "missing project ID",
			envs: map[string]string{
				"BUCKET_NAME": "test-bucket",
			},
			wantErr: true,
			errMsg:  "GOOGLE_PROJECT_ID environment variable is required",
		},
		{
			name:       "empty environment variables",
			expectPort: "8080",
			envs:       map[string]string{},
			wantErr:    true,
			errMsg:     "BUCKET_NAME environment variable is required",
		},
		{
			name:       "all environment variables empty",
			expectPort: "8080",
			envs: map[string]string{
				"BUCKET_NAME":       "",
				"GOOGLE_PROJECT_ID": "",
			},
			wantErr: true,
			errMsg:  "BUCKET_NAME environment variable is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment variables
			os.Unsetenv("BUCKET_NAME")
			os.Unsetenv("GOOGLE_PROJECT_ID")

			// Set test environment variables
			for k, v := range tt.envs {
				os.Setenv(k, v)
			}

			// Create base config if port is specified
			var base *config
			if tt.basePort != "" {
				base = &config{port: tt.basePort}
			}

			cfg, err := loadConfig(context.Background(), base, nil)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Equal(t, tt.errMsg, err.Error())
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectPort, cfg.port)
			assert.Equal(t, tt.envs["BUCKET_NAME"], cfg.bucketName)
			assert.Equal(t, tt.envs["GOOGLE_PROJECT_ID"], cfg.projectID)
		})
	}
}
