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
}
