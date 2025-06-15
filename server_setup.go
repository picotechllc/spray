package main

import (
	"context"
	"net/http"

	"cloud.google.com/go/logging"
)

// ServerSetup is a function type for setting up the HTTP server
type ServerSetup func(context.Context, *config, *logging.Client) (*http.Server, error)

// DefaultServerSetup is the default server setup implementation
var DefaultServerSetup ServerSetup = createServer
