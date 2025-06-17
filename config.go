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
	headersFile   = "headers.toml"
)

type config struct {
	port       string
	bucketName string
	projectID  string
	store      ObjectStore
	redirects  map[string]string // path -> destination URL
	headers    *HeaderConfig     // header configuration
}

// RedirectConfig represents the structure of the redirects.toml file
type RedirectConfig struct {
	Redirects map[string]string `toml:"redirects"`
}

// HeaderConfig represents the structure of the headers.toml file
type HeaderConfig struct {
	PoweredBy PoweredByConfig `toml:"powered_by"`
}

// PoweredByConfig controls the X-Powered-By header behavior
type PoweredByConfig struct {
	Enabled bool `toml:"enabled"`
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

// cleanRedirectPath normalizes a redirect path from the configuration file
// to match the format used by cleanRequestPath (removes leading slash)
func cleanRedirectPath(path string) string {
	// Handle empty path
	if path == "" {
		return ""
	}

	// Remove leading slash to match cleanRequestPath behavior
	if strings.HasPrefix(path, "/") {
		return path[1:]
	}

	return path
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

	// Clean and validate redirects
	cleanedRedirects := make(map[string]string)
	for path, dest := range redirectConfig.Redirects {
		// Validate destination URL
		if _, err := url.ParseRequestURI(dest); err != nil {
			redirectConfigErrors.WithLabelValues("", "invalid_url").Inc()
			return nil, fmt.Errorf("invalid redirect destination URL for path %q: %v", path, err)
		}

		// Clean the redirect path to match request path format
		cleanedPath := cleanRedirectPath(path)
		cleanedRedirects[cleanedPath] = dest
	}

	return cleanedRedirects, nil
}

// loadHeaders loads header configuration from a headers.toml file in the .spray directory
func loadHeaders(ctx context.Context, store ObjectStore) (*HeaderConfig, error) {
	configPath := filepath.Join(configDir, headersFile)
	reader, _, err := store.GetObject(ctx, configPath)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			// No headers file is fine, return default config (enabled)
			return &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			}, nil
		}
		// Handle permission errors gracefully - headers are optional
		if isPermissionError(err) {
			// Use a generic bucket name for metrics when we don't have access to store the config
			redirectConfigErrors.WithLabelValues("", "permission_denied").Inc()
			logStructuredWarning("load_headers", configPath, err)
			return &HeaderConfig{
				PoweredBy: PoweredByConfig{Enabled: true},
			}, nil
		}
		redirectConfigErrors.WithLabelValues("", "read_error").Inc()
		return nil, fmt.Errorf("error reading headers file at %s: %v", configPath, err)
	}
	defer reader.Close()

	var headerConfig HeaderConfig
	if _, err := toml.NewDecoder(reader).Decode(&headerConfig); err != nil {
		redirectConfigErrors.WithLabelValues("", "parse_error").Inc()
		return nil, fmt.Errorf("error parsing headers file at %s: %v", configPath, err)
	}

	return &headerConfig, nil
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

	// Load redirects and headers if store is provided
	if store != nil {
		redirects, err := loadRedirects(ctx, store)
		if err != nil {
			return nil, fmt.Errorf("error loading redirects: %v", err)
		}
		cfg.redirects = redirects

		headers, err := loadHeaders(ctx, store)
		if err != nil {
			return nil, fmt.Errorf("error loading headers: %v", err)
		}
		cfg.headers = headers
	} else {
		cfg.redirects = make(map[string]string)
		cfg.headers = &HeaderConfig{
			PoweredBy: PoweredByConfig{Enabled: true},
		}
	}

	return cfg, nil
}

// resolveXPoweredByHeader determines the final X-Powered-By header value
// based on environment variable and site configuration following the hybrid approach:
// 1. If env var is empty → No header (site owners can't override)
// 2. If env var has value AND headers.toml doesn't exist → Use env var value
// 3. If env var has value AND headers.toml disables it → No header
// 4. If env var has value AND headers.toml enables it → Use env var value
func resolveXPoweredByHeader(headerConfig *HeaderConfig, version string) string {
	// Get the environment variable value
	envValue, envExists := os.LookupEnv("SPRAY_POWERED_BY_HEADER")

	// If explicitly set to empty string, disable entirely (site owners can't override)
	if envExists && envValue == "" {
		return ""
	}

	// If not set, use default format
	if !envExists {
		envValue = fmt.Sprintf("spray/%s", version)
	}

	// Check if site owner has disabled it
	if headerConfig != nil && !headerConfig.PoweredBy.Enabled {
		return ""
	}

	return envValue
}
