//go:build integration
// +build integration

package main

import (
	"context"
	"io"
	"testing"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCSIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a real GCS client
	client, err := storage.NewClient(ctx)
	require.NoError(t, err, "Failed to create GCS client")
	defer client.Close()

	// Create a real logging client
	logClient, err := logging.NewClient(ctx, "test-project")
	require.NoError(t, err, "Failed to create logging client")
	defer logClient.Close()
	logger := logClient.Logger("gcs-server")

	// Create a test bucket
	bucketName := "spray-test-bucket-" + t.Name()
	bucket := client.Bucket(bucketName)

	// Create the bucket if it doesn't exist
	_, err = bucket.Attrs(ctx)
	if err == storage.ErrBucketNotExist {
		err = bucket.Create(ctx, "test-project", nil)
		require.NoError(t, err, "Failed to create test bucket")
		defer func() {
			// Clean up: delete the bucket after test
			err := bucket.Delete(ctx)
			if err != nil {
				t.Logf("Warning: failed to delete test bucket: %v", err)
			}
		}()
	} else {
		require.NoError(t, err, "Failed to check bucket existence")
	}

	// Create a test object
	obj := bucket.Object("test.txt")
	w := obj.NewWriter(ctx)
	_, err = io.WriteString(w, "Hello, World!")
	require.NoError(t, err, "Failed to write test object")
	err = w.Close()
	require.NoError(t, err, "Failed to close test object writer")

	// Create the server
	server, err := newGCSServer(ctx, bucketName, &gcpLoggerAdapter{logger: logger}, client)
	require.NoError(t, err, "Failed to create GCS server")

	// Test object retrieval
	reader, attrs, err := server.store.GetObject(ctx, "test.txt")
	require.NoError(t, err, "Failed to get test object")
	defer reader.Close()

	// Verify object attributes
	assert.Equal(t, "text/plain", attrs.ContentType)
	assert.Equal(t, int64(13), attrs.Size) // "Hello, World!" is 13 bytes

	// Verify object content
	content, err := io.ReadAll(reader)
	require.NoError(t, err, "Failed to read test object content")
	assert.Equal(t, "Hello, World!", string(content))

	// Test non-existent object
	_, _, err = server.store.GetObject(ctx, "nonexistent.txt")
	assert.ErrorIs(t, err, storage.ErrObjectNotExist, "Expected ErrObjectNotExist for non-existent object")
}
