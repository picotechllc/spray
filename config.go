package main

import (
	"fmt"
	"os"
)

type config struct {
	port       string
	bucketName string
	projectID  string
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

// loadConfig loads configuration from environment variables and the provided base config.
func loadConfig(base *config) (*config, error) {
	cfg := &config{
		port: "8080", // default value
	}
	if base != nil {
		*cfg = *base
	}

	cfg.bucketName = os.Getenv("BUCKET_NAME")
	cfg.projectID = os.Getenv("GOOGLE_PROJECT_ID")

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
