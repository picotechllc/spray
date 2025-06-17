//go:build integration
// +build integration

package main

import (
	"context"
	"io"
	"os"
	"testing"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCSIntegration(t *testing.T) {
	ctx := context.Background()

	// Set up authentication for CI environment
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		if ghaCredsPath := os.Getenv("GOOGLE_GHA_CREDS_PATH"); ghaCredsPath != "" {
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", ghaCredsPath)
		} else if cloudsdk := os.Getenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"); cloudsdk != "" {
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", cloudsdk)
		}
	}

	// Create a real GCS client
	client, err := storage.NewClient(ctx)
	require.NoError(t, err, "Failed to create GCS client")
	defer client.Close()

	// Create a real logging client
	logClient, err := logging.NewClient(ctx, "test-project")
	require.NoError(t, err, "Failed to create logging client")
	defer logClient.Close()
	logger := logClient.Logger("gcs-server")

	// Use the specific test bucket created by setup-ci-buckets.sh
	bucketName := "spray-test-bucket-gcsintegration"

	t.Logf("Using test bucket: %s", bucketName)
	bucket := client.Bucket(bucketName)

	// Verify bucket exists and is accessible
	_, err = bucket.Attrs(ctx)
	require.NoError(t, err, "Failed to access test bucket %s. Make sure it exists and the service account has permissions.", bucketName)

	// Create the GCS object store
	store := &GCSObjectStore{
		bucket: bucket,
	}

	// Create a GCS server for testing
	server, err := newGCSServer(ctx, bucketName, &gcpLoggerAdapter{logger: logger}, store, make(map[string]string), &HeaderConfig{
		PoweredBy: PoweredByConfig{Enabled: true},
	})
	require.NoError(t, err, "Failed to create GCS server")

	// Test object retrieval using pre-existing test object
	testObjectName := "test.txt"
	reader, attrs, err := server.store.GetObject(ctx, testObjectName)
	if err == storage.ErrObjectNotExist {
		t.Logf("Test object %s doesn't exist, creating it...", testObjectName)

		// Try to create the test object (this requires storage.objects.create permission)
		obj := bucket.Object(testObjectName)
		w := obj.NewWriter(ctx)
		_, writeErr := io.WriteString(w, "Hello, World!")
		require.NoError(t, writeErr, "Failed to write test object")
		closeErr := w.Close()
		require.NoError(t, closeErr, "Failed to close test object writer")

		t.Logf("Created test object %s", testObjectName)

		// Now try to read it again
		reader, attrs, err = server.store.GetObject(ctx, testObjectName)
	}

	require.NoError(t, err, "Failed to get test object %s", testObjectName)
	defer reader.Close()

	// Verify object attributes
	assert.Equal(t, "text/plain", attrs.ContentType)

	// Verify object content
	content, err := io.ReadAll(reader)
	require.NoError(t, err, "Failed to read test object content")

	// The test object should contain "Hello, World!" without trailing newline
	// (For existing objects that may have newline, we'll be flexible)
	contentStr := string(content)
	if contentStr == "Hello, World!\n" {
		// Existing object has newline - accept it
		assert.Equal(t, int64(14), attrs.Size) // "Hello, World!\n" is 14 bytes
		t.Logf("Note: Test object contains trailing newline (legacy object)")
	} else {
		// New objects should not have newline
		assert.Equal(t, int64(13), attrs.Size) // "Hello, World!" is 13 bytes
		assert.Equal(t, "Hello, World!", contentStr)
	}

	// Test non-existent object
	_, _, err = server.store.GetObject(ctx, "nonexistent.txt")
	assert.ErrorIs(t, err, storage.ErrObjectNotExist, "Expected ErrObjectNotExist for non-existent object")

	t.Logf("✓ Integration test completed successfully using bucket: %s", bucketName)
}
