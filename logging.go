package main

import (
	"context"
	"os"

	"cloud.google.com/go/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// createLoggingClient creates a new logging client.
func createLoggingClient(ctx context.Context, projectID string) (LoggingClient, error) {
	// Explicit test mode - always use mock
	if os.Getenv("LOGGING_TEST_MODE") == "true" {
		return newMockLogClient(), nil
	}

	// Explicit offline mode - use zap for debugging
	if os.Getenv("LOGGING_OFFLINE") == "true" {
		return newZapLogClient(), nil
	}

	// Check if we're in a testing context (no credentials available)
	// but want actual debug output
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" && os.Getenv("GCP_PROJECT") == "" {
		// If we're running tests (detected by testing.Testing() or test binary),
		// use mock logger, otherwise use zap for debugging
		if isTestContext() {
			return newMockLogClient(), nil
		}
		return newZapLogClient(), nil
	}

	// In production, create a real client
	client, err := logging.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return newGCPLoggingClient(client), nil
}

// isTestContext checks if we're running in a test context
func isTestContext() bool {
	// Check if we're running as a test binary
	for _, arg := range os.Args {
		if arg == "-test.v" || arg == "-test.run" ||
			(len(arg) > 5 && arg[:5] == "-test") {
			return true
		}
	}
	return false
}

// zapLogger wraps zap.Logger to implement our Logger interface
type zapLogger struct {
	logger *zap.Logger
	name   string
}

func (l *zapLogger) Log(entry logging.Entry) {
	// Convert Cloud Logging severity to zap level
	var level zapcore.Level
	switch entry.Severity {
	case logging.Debug:
		level = zapcore.DebugLevel
	case logging.Info:
		level = zapcore.InfoLevel
	case logging.Warning:
		level = zapcore.WarnLevel
	case logging.Error:
		level = zapcore.ErrorLevel
	case logging.Critical:
		level = zapcore.FatalLevel
	default:
		level = zapcore.InfoLevel
	}

	// Build fields from payload
	var fields []zap.Field
	if entry.Payload != nil {
		if payloadMap, ok := entry.Payload.(map[string]any); ok {
			for key, value := range payloadMap {
				fields = append(fields, zap.Any(key, value))
			}
		} else {
			fields = append(fields, zap.Any("payload", entry.Payload))
		}
	}

	// Add logger name
	fields = append(fields, zap.String("logger", l.name))

	// Log the message
	l.logger.Log(level, "spray-log", fields...)
}

// zapLogClient provides zap-based logging
type zapLogClient struct {
	logger *zap.Logger
}

func (c *zapLogClient) Logger(name string, opts ...logging.LoggerOption) Logger {
	return &zapLogger{
		logger: c.logger,
		name:   name,
	}
}

func (c *zapLogClient) Close() error {
	// Sync can fail in test environments or when stderr is not available
	// We'll try to sync but ignore errors since this is just a cleanup operation
	if err := c.logger.Sync(); err != nil {
		// Ignore the sync error and don't fail the close operation
		// This commonly happens in test environments or when stderr is redirected
		return nil
	}
	return nil
}

// newZapLogClient creates a new zap logging client for debugging
func newZapLogClient() LoggingClient {
	// Create a development config for better debugging output
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.LevelKey = "severity"
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// Output to stderr
	config.OutputPaths = []string{"stderr"}
	config.ErrorOutputPaths = []string{"stderr"}

	logger, err := config.Build()
	if err != nil {
		// Fallback to a basic logger if config fails
		logger = zap.NewNop()
	}

	return &zapLogClient{logger: logger}
}

// mockLogger is a mock implementation of the Logger interface
// Used for tests
type mockLogger struct{}

func (l *mockLogger) Log(entry logging.Entry) {
	// No-op in tests
}

// mockLogClient is a mock implementation of the LoggingClient interface
// Used for tests
type mockLogClient struct{}

func (c *mockLogClient) Logger(name string, opts ...logging.LoggerOption) Logger {
	return &mockLogger{}
}

func (c *mockLogClient) Close() error {
	return nil
}

// newMockLogClient creates a new mock logging client
func newMockLogClient() LoggingClient {
	return &mockLogClient{}
}
