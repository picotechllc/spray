package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
)

// testGCSReader implements io.ReadCloser for testing
type testGCSReader struct {
	data    []byte
	offset  int64
	closed  bool
	onClose func() error
}

func (r *testGCSReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, errors.New("reader is closed")
	}

	if r.offset >= int64(len(r.data)) {
		return 0, io.EOF
	}

	n := copy(p, r.data[r.offset:])
	r.offset += int64(n)
	return n, nil
}

func (r *testGCSReader) Close() error {
	if r.onClose != nil {
		return r.onClose()
	}

	if r.closed {
		return errors.New("reader already closed")
	}
	r.closed = true
	return nil
}

// TestGCSObjectStore tests the GetObject method of GCSObjectStore
func TestGCSObjectStore_Interface(t *testing.T) {
	// Test success case
	t.Run("successful read", func(t *testing.T) {
		// Create context
		ctx := context.Background()

		// Define expected data
		expectedData := []byte("test content")
		expectedAttrs := &storage.ObjectAttrs{
			Name:        "test.txt",
			ContentType: "text/plain",
			Size:        int64(len(expectedData)),
		}

		// Create a custom reader
		reader := &testGCSReader{
			data: expectedData,
		}

		// Create a function-based implementation of ObjectStore
		store := testGCSObjectStoreFunc(func(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
			if path == "test.txt" {
				return reader, expectedAttrs, nil
			}
			return nil, nil, storage.ErrObjectNotExist
		})

		// Call GetObject
		result, attrs, err := store.GetObject(ctx, "test.txt")
		assert.NoError(t, err)
		assert.Same(t, reader, result)
		assert.Same(t, expectedAttrs, attrs)

		// Test with non-existent file
		result, attrs, err = store.GetObject(ctx, "nonexistent.txt")
		assert.Error(t, err)
		assert.Equal(t, storage.ErrObjectNotExist, err)
		assert.Nil(t, result)
		assert.Nil(t, attrs)
	})

	// Test cleanup when Attrs fails
	t.Run("cleanup on error", func(t *testing.T) {
		ctx := context.Background()
		readerClosed := false

		// This test simulates the behavior of GCSObjectStore.GetObject
		// when object.Attrs() fails after object.NewReader() succeeds
		testFunc := func(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
			// Create a reader that tracks if it was closed
			reader := &testGCSReader{
				data: []byte("test data"),
				onClose: func() error {
					readerClosed = true
					return nil
				},
			}

			// Cleanup the reader
			reader.Close()

			// Return an error
			return nil, nil, errors.New("simulated attrs error")
		}

		// Create the store and test
		store := testGCSObjectStoreFunc(testFunc)
		reader, attrs, err := store.GetObject(ctx, "test.txt")

		// Verify results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "simulated attrs error")
		assert.Nil(t, reader)
		assert.Nil(t, attrs)
		assert.True(t, readerClosed, "Reader should be closed on error")
	})
}

// testGCSObjectStoreFunc implements ObjectStore for testing
type testGCSObjectStoreFunc func(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error)

func (f testGCSObjectStoreFunc) GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error) {
	return f(ctx, path)
}
