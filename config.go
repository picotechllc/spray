package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/BurntSushi/toml"
)

const (
	configDir     = ".spray"
	redirectsFile = "redirects.toml"
)

type config struct {
	port       string
	bucketName string
	projectID  string
	store      ObjectStore
	redirects  map[string]string // path -> destination URL
}

// RedirectConfig represents the structure of the redirects.toml file
type RedirectConfig struct {
	Redirects map[string]string `toml:"redirects"`
}

// isPermissionError checks if the error is related to permissions/access denied
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "AccessDenied") ||
		strings.Contains(errStr, "Access denied") ||
		strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "403")
}

// validateConfig checks if the config is valid and returns an error if not.
func validateConfig(cfg *config) error {
	if cfg.bucketName == "" {
		return fmt.Errorf("BUCKET_NAME environment variable is required")
	}
	if cfg.projectID == "" {
		return fmt.Errorf("GOOGLE_PROJECT_ID environment variable is required")
	}
	return nil
}

// logStructuredWarning logs a structured warning message to stderr in JSON format
func logStructuredWarning(operation, path string, err error) {
	warning := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"severity":  "WARNING",
		"operation": operation,
		"path":      path,
		"message":   fmt.Sprintf("Cannot access %s due to permission error", path),
	}

	if err != nil {
		warning["error"] = err.Error()
		warning["error_type"] = "permission_denied"
	}

	// Log to stderr in JSON format for consistency
	jsonBytes, jsonErr := json.Marshal(warning)
	if jsonErr != nil {
		// Fallback to simple log if JSON marshaling fails
		log.Printf("Warning: Cannot access %s due to permission error: %v", path, err)
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", string(jsonBytes))
	}
}

// loadRedirects loads redirects from a redirects.toml file in the .spray directory
func loadRedirects(ctx context.Context, store ObjectStore) (map[string]string, error) {
	configPath := filepath.Join(configDir, redirectsFile)
	reader, _, err := store.GetObject(ctx, configPath)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			// No redirects file is fine, return empty map
			return make(map[string]string), nil
		}
		// Handle permission errors gracefully - redirects are optional
		if isPermissionError(err) {
			redirectConfigErrors.WithLabelValues("", "permission_denied").Inc()
			logStructuredWarning("load_redirects", configPath, err)
			return make(map[string]string), nil
		}
		redirectConfigErrors.WithLabelValues("", "read_error").Inc()
		return nil, fmt.Errorf("error reading redirects file at %s: %v", configPath, err)
	}
	defer reader.Close()

	var redirectConfig RedirectConfig
	if _, err := toml.NewDecoder(reader).Decode(&redirectConfig); err != nil {
		redirectConfigErrors.WithLabelValues("", "parse_error").Inc()
		return nil, fmt.Errorf("error parsing redirects file at %s: %v", configPath, err)
	}

	// Initialize redirects map if it's nil
	if redirectConfig.Redirects == nil {
		redirectConfig.Redirects = make(map[string]string)
	}

	// Validate redirect URLs
	for path, dest := range redirectConfig.Redirects {
		if _, err := url.ParseRequestURI(dest); err != nil {
			redirectConfigErrors.WithLabelValues("", "invalid_url").Inc()
			return nil, fmt.Errorf("invalid redirect destination URL for path %q: %v", path, err)
		}
	}

	return redirectConfig.Redirects, nil
}

// loadConfig loads configuration from environment variables and the provided base config.
func loadConfig(ctx context.Context, base *config, store ObjectStore) (*config, error) {
	cfg := &config{
		port: "8080", // default value
	}
	if base != nil {
		*cfg = *base
	}

	cfg.bucketName = os.Getenv("BUCKET_NAME")
	cfg.projectID = os.Getenv("GOOGLE_PROJECT_ID")
	cfg.store = store // Assign the store to the config

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	// Load redirects if store is provided
	if store != nil {
		redirects, err := loadRedirects(ctx, store)
		if err != nil {
			return nil, fmt.Errorf("error loading redirects: %v", err)
		}
		cfg.redirects = redirects
	} else {
		cfg.redirects = make(map[string]string)
	}

	return cfg, nil
}
