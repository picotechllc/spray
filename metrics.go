package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// requestsTotal counts the total number of requests handled by the server
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_requests_total",
			Help: "Total number of requests handled by the GCS server",
		},
		[]string{"path", "method", "status"},
	)

	// requestDuration tracks request duration in seconds
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_request_duration_seconds",
			Help:    "Duration of requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "method"},
	)

	// bytesTransferred tracks the number of bytes transferred
	bytesTransferred = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_bytes_transferred_total",
			Help: "Total number of bytes transferred",
		},
		[]string{"path", "method", "direction"}, // direction can be "upload" or "download"
	)

	// activeRequests tracks the number of currently active requests
	activeRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gcs_server_active_requests",
			Help: "Number of currently active requests",
		},
	)
)
