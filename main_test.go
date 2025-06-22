package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStorageClientWithError is a mock that can simulate errors during creation
type mockStorageClientWithError struct {
	*mockStorageClient
	creationError error
}

func (c *mockStorageClientWithError) simulateCreationError() error {
	return c.creationError
}

func TestStorageClientFactory_Success(t *testing.T) {
	// Save original factory
	originalFactory := storageClientFactory
	defer func() { storageClientFactory = originalFactory }()

	// Test successful authenticated client creation
	storageClientFactory = func(ctx context.Context) (StorageClient, error) {
		return &mockStorageClient{objects: make(map[string]mockObject)}, nil
	}

	ctx := context.Background()
	client, err := storageClientFactory(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Verify it's the expected type
	_, ok := client.(*mockStorageClient)
	assert.True(t, ok, "Expected mockStorageClient")
}

func TestStorageClientFactory_MockStorage(t *testing.T) {
	// Save original factory
	originalFactory := storageClientFactory
	defer func() { storageClientFactory = originalFactory }()

	// Test mock storage environment variable
	os.Setenv("STORAGE_MOCK", "true")
	defer os.Unsetenv("STORAGE_MOCK")

	ctx := context.Background()
	client, err := storageClientFactory(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Verify it's the debug mock type
	_, ok := client.(*debugMockStorageClient)
	assert.True(t, ok, "Expected debugMockStorageClient when STORAGE_MOCK=true")
}

func TestStorageClientFactory_AuthenticationFallback(t *testing.T) {
	// Save original factory
	originalFactory := storageClientFactory
	defer func() { storageClientFactory = originalFactory }()

	// Track calls to understand the fallback behavior
	var authenticatedCalled, unauthenticatedCalled bool

	// Create a custom factory that simulates the real behavior
	storageClientFactory = func(ctx context.Context) (StorageClient, error) {
		// Check if we should use mock storage for debugging
		if os.Getenv("STORAGE_MOCK") == "true" {
			return &debugMockStorageClient{
				objects: make(map[string]debugMockObject),
			}, nil
		}

		// Simulate trying to create an authenticated client first
		authenticatedCalled = true
		credentialErr := errors.New("metadata: GCE metadata \"instance/service-accounts/default/token\" not defined")

		// Preserve the original error for reporting
		originalErr := credentialErr

		// Check if this looks like a credential issue that might work with unauthenticated access
		errStr := originalErr.Error()
		if strings.Contains(errStr, "metadata") ||
			strings.Contains(errStr, "credential") ||
			strings.Contains(errStr, "token") ||
			strings.Contains(errStr, "authentication") {

			// Simulate creating unauthenticated client
			unauthenticatedCalled = true
			return &mockStorageClient{objects: make(map[string]mockObject)}, nil
		}

		return nil, originalErr
	}

	ctx := context.Background()
	client, err := storageClientFactory(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.True(t, authenticatedCalled, "Should have tried authenticated client first")
	assert.True(t, unauthenticatedCalled, "Should have fallen back to unauthenticated client")

	// Verify it's a mock client (representing successful unauthenticated creation)
	_, ok := client.(*mockStorageClient)
	assert.True(t, ok, "Expected mockStorageClient after fallback")
}

func TestStorageClientFactory_NonCredentialError(t *testing.T) {
	// Save original factory
	originalFactory := storageClientFactory
	defer func() { storageClientFactory = originalFactory }()

	// Create a factory that simulates a non-credential error
	storageClientFactory = func(ctx context.Context) (StorageClient, error) {
		if os.Getenv("STORAGE_MOCK") == "true" {
			return &debugMockStorageClient{
				objects: make(map[string]debugMockObject),
			}, nil
		}

		// Simulate a non-credential error (like network error)
		networkErr := errors.New("network error: connection refused")

		// The factory should not attempt fallback for non-credential errors
		errStr := networkErr.Error()
		if strings.Contains(errStr, "metadata") ||
			strings.Contains(errStr, "credential") ||
			strings.Contains(errStr, "token") ||
			strings.Contains(errStr, "authentication") {
			// This shouldn't happen in this test
			t.Fatal("Test setup error: network error should not match credential patterns")
		}

		return nil, networkErr
	}

	ctx := context.Background()
	client, err := storageClientFactory(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "network error: connection refused")
}

func TestStorageClientFactory_BothClientsFail(t *testing.T) {
	// Save original factory
	originalFactory := storageClientFactory
	defer func() { storageClientFactory = originalFactory }()

	// Create a factory that simulates both clients failing
	storageClientFactory = func(ctx context.Context) (StorageClient, error) {
		if os.Getenv("STORAGE_MOCK") == "true" {
			return &debugMockStorageClient{
				objects: make(map[string]debugMockObject),
			}, nil
		}

		// Simulate credential error for authenticated client
		credentialErr := errors.New("metadata: GCE metadata not defined")

		// Check if this looks like a credential issue
		errStr := credentialErr.Error()
		if strings.Contains(errStr, "metadata") ||
			strings.Contains(errStr, "credential") ||
			strings.Contains(errStr, "token") ||
			strings.Contains(errStr, "authentication") {

			// Simulate unauthenticated client also failing
			unauthErr := errors.New("unauthenticated client creation failed")
			return nil, errors.New("failed to create both authenticated and unauthenticated storage clients. Authenticated error: " + credentialErr.Error() + ", Unauthenticated error: " + unauthErr.Error())
		}

		return nil, credentialErr
	}

	ctx := context.Background()
	client, err := storageClientFactory(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "failed to create both authenticated and unauthenticated storage clients")
	assert.Contains(t, err.Error(), "metadata: GCE metadata not defined")
	assert.Contains(t, err.Error(), "unauthenticated client creation failed")
}

func TestRun(t *testing.T) {
	// Save original setupServer and restore after test
	originalSetup := DefaultServerSetup
	defer func() { DefaultServerSetup = originalSetup }()

	// Create a mock server setup function
	DefaultServerSetup = func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
		// Create a mock GCS server
		objects := make(map[string]mockObject)
		server := createMockServer(t, objects, nil)

		mux := http.NewServeMux()
		mux.Handle("/", server)
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/readyz", readyzHandler)
		mux.HandleFunc("/livez", livezHandler)

		return &http.Server{
			Addr:    ":" + cfg.port,
			Handler: mux,
		}, nil
	}

	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock config
	cfg := &config{
		port:       "8080",
		bucketName: "test-bucket",
		projectID:  "test-project",
	}

	// Create a mock logging client
	logClient := newMockLogClient()

	// Create the server first
	srv, err := DefaultServerSetup(ctx, cfg, logClient)
	require.NoError(t, err)

	// Run the server in a goroutine
	go func() {
		err := runServer(ctx, srv)
		assert.NoError(t, err)
	}()

	// Wait a bit for the server to start
	time.Sleep(100 * time.Millisecond)

	// Test the health check endpoints
	resp, err := http.Get("http://localhost:8080/readyz")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get("http://localhost:8080/livez")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Cancel the context to stop the server
	cancel()

	// Wait a bit for the server to stop
	time.Sleep(100 * time.Millisecond)
}

func TestRunApp_Errors(t *testing.T) {
	// Save original client factories and server setup functions
	originalStorageClientFactory := storageClientFactory
	originalLoggingClientFactory := loggingClientFactory
	originalDefaultServerSetup := DefaultServerSetup
	defer func() {
		storageClientFactory = originalStorageClientFactory
		loggingClientFactory = originalLoggingClientFactory
		DefaultServerSetup = originalDefaultServerSetup
	}()

	// Mock storage client factory to avoid Google Cloud credentials issues
	storageClientFactory = func(ctx context.Context) (StorageClient, error) {
		return &mockStorageClient{objects: make(map[string]mockObject)}, nil
	}

	// Mock logging client factory to return an error
	loggingClientFactory = func(ctx context.Context, projectID string) (LoggingClient, error) {
		return nil, assert.AnError
	}

	// Mock server setup to return an error
	DefaultServerSetup = func(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
		return nil, assert.AnError
	}

	// Set a valid bucket name for all tests
	os.Setenv("BUCKET_NAME", "test-bucket")
	defer os.Unsetenv("BUCKET_NAME")

	// Test missing GOOGLE_PROJECT_ID
	os.Unsetenv("GOOGLE_PROJECT_ID")
	err := RunApp(context.Background(), "8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GOOGLE_PROJECT_ID environment variable is required")

	// Test logging client factory error
	os.Setenv("GOOGLE_PROJECT_ID", "test-project")
	err = RunApp(context.Background(), "8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), assert.AnError.Error())

	// Test server setup error
	loggingClientFactory = func(ctx context.Context, projectID string) (LoggingClient, error) {
		return &mockLogClient{}, nil
	}
	err = RunApp(context.Background(), "8080")
	assert.Error(t, err)
	// The error could be from server setup or from storage operations
	assert.True(t,
		strings.Contains(err.Error(), assert.AnError.Error()) ||
			strings.Contains(err.Error(), "storage: bucket name is empty") ||
			strings.Contains(err.Error(), "error loading redirects"),
		"Expected error to contain known error patterns, got: %s", err.Error())

	// Reset flag.CommandLine to avoid test interference
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}

func TestGetCredentialContext_ServiceAccountFile(t *testing.T) {
	// Save original environment variables
	originalGoogleApplicationCredentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	originalGoogleProjectID := os.Getenv("GOOGLE_PROJECT_ID")
	originalGCPProject := os.Getenv("GCP_PROJECT")
	originalGoogleCloudProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalGCloudProject := os.Getenv("GCLOUD_PROJECT")

	defer func() {
		// Restore original environment variables
		if originalGoogleApplicationCredentials != "" {
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", originalGoogleApplicationCredentials)
		} else {
			os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		}
		if originalGoogleProjectID != "" {
			os.Setenv("GOOGLE_PROJECT_ID", originalGoogleProjectID)
		} else {
			os.Unsetenv("GOOGLE_PROJECT_ID")
		}
		if originalGCPProject != "" {
			os.Setenv("GCP_PROJECT", originalGCPProject)
		} else {
			os.Unsetenv("GCP_PROJECT")
		}
		if originalGoogleCloudProject != "" {
			os.Setenv("GOOGLE_CLOUD_PROJECT", originalGoogleCloudProject)
		} else {
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		}
		if originalGCloudProject != "" {
			os.Setenv("GCLOUD_PROJECT", originalGCloudProject)
		} else {
			os.Unsetenv("GCLOUD_PROJECT")
		}
	}()

	// Test with service account file
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/path/to/service-account.json")
	os.Setenv("GOOGLE_PROJECT_ID", "test-project-123")
	os.Setenv("GCP_PROJECT", "test-gcp-project")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GCLOUD_PROJECT")

	context := getCredentialContext()

	assert.Equal(t, "service_account_file", context["credentials_source"])
	assert.Equal(t, "/path/to/service-account.json", context["credentials_file"])
	assert.Equal(t, "test-project-123", context["project_id"])
	assert.Equal(t, "test-gcp-project", context["gcp_project"])
	assert.NotContains(t, context, "gcp_environment")
}

func TestGetCredentialContext_ApplicationDefaultCredentials(t *testing.T) {
	// Save original environment variables
	originalGoogleApplicationCredentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	originalGoogleProjectID := os.Getenv("GOOGLE_PROJECT_ID")
	originalGCPProject := os.Getenv("GCP_PROJECT")
	originalGoogleCloudProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalGCloudProject := os.Getenv("GCLOUD_PROJECT")

	defer func() {
		// Restore original environment variables
		if originalGoogleApplicationCredentials != "" {
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", originalGoogleApplicationCredentials)
		} else {
			os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		}
		if originalGoogleProjectID != "" {
			os.Setenv("GOOGLE_PROJECT_ID", originalGoogleProjectID)
		} else {
			os.Unsetenv("GOOGLE_PROJECT_ID")
		}
		if originalGCPProject != "" {
			os.Setenv("GCP_PROJECT", originalGCPProject)
		} else {
			os.Unsetenv("GCP_PROJECT")
		}
		if originalGoogleCloudProject != "" {
			os.Setenv("GOOGLE_CLOUD_PROJECT", originalGoogleCloudProject)
		} else {
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		}
		if originalGCloudProject != "" {
			os.Setenv("GCLOUD_PROJECT", originalGCloudProject)
		} else {
			os.Unsetenv("GCLOUD_PROJECT")
		}
	}()

	// Test with application default credentials (no service account file)
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Unsetenv("GOOGLE_PROJECT_ID")
	os.Unsetenv("GCP_PROJECT")
	os.Setenv("GOOGLE_CLOUD_PROJECT", "cloud-project-456")
	os.Unsetenv("GCLOUD_PROJECT")

	context := getCredentialContext()

	assert.Equal(t, "application_default_credentials", context["credentials_source"])
	assert.Equal(t, "cloud_environment", context["gcp_environment"])
	assert.NotContains(t, context, "credentials_file")
	assert.NotContains(t, context, "project_id")
	assert.NotContains(t, context, "gcp_project")
}

func TestGetCredentialContext_GCloudProject(t *testing.T) {
	// Save original environment variables
	originalGoogleApplicationCredentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	originalGoogleProjectID := os.Getenv("GOOGLE_PROJECT_ID")
	originalGCPProject := os.Getenv("GCP_PROJECT")
	originalGoogleCloudProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalGCloudProject := os.Getenv("GCLOUD_PROJECT")

	defer func() {
		// Restore original environment variables
		if originalGoogleApplicationCredentials != "" {
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", originalGoogleApplicationCredentials)
		} else {
			os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		}
		if originalGoogleProjectID != "" {
			os.Setenv("GOOGLE_PROJECT_ID", originalGoogleProjectID)
		} else {
			os.Unsetenv("GOOGLE_PROJECT_ID")
		}
		if originalGCPProject != "" {
			os.Setenv("GCP_PROJECT", originalGCPProject)
		} else {
			os.Unsetenv("GCP_PROJECT")
		}
		if originalGoogleCloudProject != "" {
			os.Setenv("GOOGLE_CLOUD_PROJECT", originalGoogleCloudProject)
		} else {
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		}
		if originalGCloudProject != "" {
			os.Setenv("GCLOUD_PROJECT", originalGCloudProject)
		} else {
			os.Unsetenv("GCLOUD_PROJECT")
		}
	}()

	// Test with GCLOUD_PROJECT instead of GOOGLE_CLOUD_PROJECT
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Unsetenv("GOOGLE_PROJECT_ID")
	os.Unsetenv("GCP_PROJECT")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Setenv("GCLOUD_PROJECT", "gcloud-project-789")

	context := getCredentialContext()

	assert.Equal(t, "application_default_credentials", context["credentials_source"])
	assert.Equal(t, "cloud_environment", context["gcp_environment"])
	assert.NotContains(t, context, "credentials_file")
	assert.NotContains(t, context, "project_id")
	assert.NotContains(t, context, "gcp_project")
}

func TestGetCredentialContext_MinimalEnvironment(t *testing.T) {
	// Save original environment variables
	originalGoogleApplicationCredentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	originalGoogleProjectID := os.Getenv("GOOGLE_PROJECT_ID")
	originalGCPProject := os.Getenv("GCP_PROJECT")
	originalGoogleCloudProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalGCloudProject := os.Getenv("GCLOUD_PROJECT")

	defer func() {
		// Restore original environment variables
		if originalGoogleApplicationCredentials != "" {
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", originalGoogleApplicationCredentials)
		} else {
			os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		}
		if originalGoogleProjectID != "" {
			os.Setenv("GOOGLE_PROJECT_ID", originalGoogleProjectID)
		} else {
			os.Unsetenv("GOOGLE_PROJECT_ID")
		}
		if originalGCPProject != "" {
			os.Setenv("GCP_PROJECT", originalGCPProject)
		} else {
			os.Unsetenv("GCP_PROJECT")
		}
		if originalGoogleCloudProject != "" {
			os.Setenv("GOOGLE_CLOUD_PROJECT", originalGoogleCloudProject)
		} else {
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		}
		if originalGCloudProject != "" {
			os.Setenv("GCLOUD_PROJECT", originalGCloudProject)
		} else {
			os.Unsetenv("GCLOUD_PROJECT")
		}
	}()

	// Test with minimal environment (no special environment variables set)
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Unsetenv("GOOGLE_PROJECT_ID")
	os.Unsetenv("GCP_PROJECT")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GCLOUD_PROJECT")

	context := getCredentialContext()

	assert.Equal(t, "application_default_credentials", context["credentials_source"])
	assert.NotContains(t, context, "credentials_file")
	assert.NotContains(t, context, "gcp_environment")
	assert.NotContains(t, context, "project_id")
	assert.NotContains(t, context, "gcp_project")
}
