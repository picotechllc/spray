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
		[]string{"bucket_name", "path", "method", "status"},
	)

	// requestDuration tracks request duration in seconds
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_request_duration_seconds",
			Help:    "Duration of requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucket_name", "path", "method"},
	)

	// bytesTransferred tracks the number of bytes transferred
	bytesTransferred = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_bytes_transferred_total",
			Help: "Total number of bytes transferred",
		},
		[]string{"bucket_name", "path", "method", "direction"}, // direction can be "upload" or "download"
	)

	// activeRequests tracks the number of currently active requests
	activeRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcs_server_active_requests",
			Help: "Number of currently active requests",
		},
		[]string{"bucket_name"},
	)

	// cacheStatus tracks cache hits and misses (for future use)
	cacheStatus = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_cache_total",
			Help: "Total number of cache hits/misses",
		},
		[]string{"bucket_name", "path", "status"}, // status: hit/miss
	)

	// errorTotal tracks specific error types
	errorTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_errors_total",
			Help: "Total number of errors by type",
		},
		[]string{"bucket_name", "path", "error_type"}, // error_type: storage_error, invalid_path, etc.
	)

	// objectSize tracks the size distribution of served objects
	objectSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_object_size_bytes",
			Help:    "Distribution of served object sizes in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB to 1GB
		},
		[]string{"bucket_name", "path"},
	)

	// gcsLatency tracks GCS operation latency
	gcsLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_storage_operation_duration_seconds",
			Help:    "Duration of GCS operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucket_name", "operation"}, // operation: get_object, get_attrs
	)

	// redirectHits tracks the number of redirects served
	redirectHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_redirects_total",
			Help: "Total number of redirects served",
		},
		[]string{"bucket_name", "path", "destination"},
	)

	// redirectLatency tracks the time taken to process redirects
	redirectLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gcs_server_redirect_duration_seconds",
			Help:    "Duration of redirect processing in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucket_name", "path"},
	)

	// redirectConfigErrors tracks errors in redirect configuration
	redirectConfigErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcs_server_redirect_config_errors_total",
			Help: "Total number of redirect configuration errors",
		},
		[]string{"bucket_name", "error_type"}, // error_type: parse_error, invalid_url, etc.
	)
)
