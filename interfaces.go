package main

import (
	"context"
	"io"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/storage"
)

// StorageClient defines the interface for storage operations
type StorageClient interface {
	Bucket(name string) *storage.BucketHandle
	Close() error
}

// ObjectStore defines the interface for storage operations
type ObjectStore interface {
	GetObject(ctx context.Context, path string) (io.ReadCloser, *storage.ObjectAttrs, error)
}

// Logger interface defines the logging operations we need
type Logger interface {
	Log(entry logging.Entry)
}

// LoggingClient interface defines the logging operations we need
type LoggingClient interface {
	Logger(name string, opts ...logging.LoggerOption) Logger
	Close() error
}

// gcpLoggerAdapter adapts the GCP Logger to our Logger interface
type gcpLoggerAdapter struct {
	logger *logging.Logger
}

func (a *gcpLoggerAdapter) Log(entry logging.Entry) {
	a.logger.Log(entry)
}

// gcpLoggingClientAdapter adapts the GCP Client to our LoggingClient interface
type gcpLoggingClientAdapter struct {
	client *logging.Client
}

func (a *gcpLoggingClientAdapter) Logger(name string, opts ...logging.LoggerOption) Logger {
	return &gcpLoggerAdapter{
		logger: a.client.Logger(name, opts...),
	}
}

func (a *gcpLoggingClientAdapter) Close() error {
	return a.client.Close()
}

// newGCPLoggingClient creates a new GCP logging client adapter
func newGCPLoggingClient(client *logging.Client) LoggingClient {
	return &gcpLoggingClientAdapter{
		client: client,
	}
}
