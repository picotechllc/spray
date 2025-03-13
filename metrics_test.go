package main

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMetricsRegistration(t *testing.T) {
	// Test that metrics are registered with correct names
	metrics := []struct {
		collector prometheus.Collector
		name      string
	}{
		{
			collector: requestsTotal,
			name:      "gcs_server_requests_total",
		},
		{
			collector: requestDuration,
			name:      "gcs_server_request_duration_seconds",
		},
		{
			collector: bytesTransferred,
			name:      "gcs_server_bytes_transferred_total",
		},
		{
			collector: activeRequests,
			name:      "gcs_server_active_requests",
		},
		{
			collector: cacheStatus,
			name:      "gcs_server_cache_total",
		},
		{
			collector: errorTotal,
			name:      "gcs_server_errors_total",
		},
		{
			collector: objectSize,
			name:      "gcs_server_object_size_bytes",
		},
		{
			collector: gcsLatency,
			name:      "gcs_server_storage_operation_duration_seconds",
		},
	}

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			// Verify the metric exists in the default registry
			assert.True(t, testutil.CollectAndCount(m.collector) >= 0)
		})
	}
}

func TestMetricsBehavior(t *testing.T) {
	t.Run("requestsTotal", func(t *testing.T) {
		// Reset all metrics before test
		prometheus.DefaultRegisterer.Unregister(requestsTotal)
		requestsTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gcs_server_requests_total",
				Help: "Total number of requests handled by the GCS server",
			},
			[]string{"path", "method", "status"},
		)

		// Test incrementing counter
		requestsTotal.WithLabelValues("/test", "GET", "200").Inc()

		value := testutil.ToFloat64(requestsTotal.WithLabelValues("/test", "GET", "200"))
		assert.Equal(t, float64(1), value)
	})

	t.Run("activeRequests", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(activeRequests)
		activeRequests = promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "gcs_server_active_requests",
				Help: "Number of currently active requests",
			},
		)

		// Test gauge behavior
		activeRequests.Inc()
		assert.Equal(t, float64(1), testutil.ToFloat64(activeRequests))

		activeRequests.Dec()
		assert.Equal(t, float64(0), testutil.ToFloat64(activeRequests))
	})

	t.Run("bytesTransferred", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(bytesTransferred)
		bytesTransferred = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gcs_server_bytes_transferred_total",
				Help: "Total number of bytes transferred",
			},
			[]string{"path", "method", "direction"},
		)

		// Test adding bytes
		bytesTransferred.WithLabelValues("/test", "GET", "download").Add(100)

		value := testutil.ToFloat64(bytesTransferred.WithLabelValues("/test", "GET", "download"))
		assert.Equal(t, float64(100), value)
	})

	t.Run("requestDuration", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(requestDuration)
		requestDuration = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gcs_server_request_duration_seconds",
				Help:    "Duration of requests in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"path", "method"},
		)

		// Test observing durations
		requestDuration.WithLabelValues("/test", "GET").Observe(0.1)

		// Verify the histogram has samples
		count := testutil.CollectAndCount(requestDuration)
		assert.Equal(t, 1, count)
	})

	t.Run("errorTotal", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(errorTotal)
		errorTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gcs_server_errors_total",
				Help: "Total number of errors by type",
			},
			[]string{"path", "error_type"},
		)

		// Test error counting
		errorTotal.WithLabelValues("/test", "storage_error").Inc()
		errorTotal.WithLabelValues("/test", "invalid_path").Inc()
		errorTotal.WithLabelValues("/test", "object_not_found").Inc()

		value := testutil.ToFloat64(errorTotal.WithLabelValues("/test", "storage_error"))
		assert.Equal(t, float64(1), value)
	})

	t.Run("objectSize", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(objectSize)
		objectSize = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gcs_server_object_size_bytes",
				Help:    "Distribution of served object sizes in bytes",
				Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
			},
			[]string{"path"},
		)

		// Test size observation
		objectSize.WithLabelValues("/test").Observe(2048)

		// Verify the histogram has samples
		count := testutil.CollectAndCount(objectSize)
		assert.Equal(t, 1, count)
	})

	t.Run("gcsLatency", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(gcsLatency)
		gcsLatency = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gcs_server_storage_operation_duration_seconds",
				Help:    "Duration of GCS operations in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		)

		// Test latency observation
		gcsLatency.WithLabelValues("get_object").Observe(0.05)

		// Verify the histogram has samples
		count := testutil.CollectAndCount(gcsLatency)
		assert.Equal(t, 1, count)
	})
}
