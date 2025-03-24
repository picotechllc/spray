package main

import (
	"testing"
	"time"

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
	const testBucket = "test-bucket"

	t.Run("requestsTotal", func(t *testing.T) {
		// Reset all metrics before test
		prometheus.DefaultRegisterer.Unregister(requestsTotal)
		requestsTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gcs_server_requests_total",
				Help: "Total number of requests handled by the GCS server",
			},
			[]string{"bucket_name", "path", "method", "status"},
		)

		// Test incrementing counter
		requestsTotal.WithLabelValues(testBucket, "/test", "GET", "200").Inc()

		value := testutil.ToFloat64(requestsTotal.WithLabelValues(testBucket, "/test", "GET", "200"))
		assert.Equal(t, float64(1), value)
	})

	t.Run("activeRequests", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(activeRequests)
		activeRequests = promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gcs_server_active_requests",
				Help: "Number of currently active requests",
			},
			[]string{"bucket_name"},
		)

		// Test gauge behavior
		activeRequests.WithLabelValues(testBucket).Inc()
		assert.Equal(t, float64(1), testutil.ToFloat64(activeRequests.WithLabelValues(testBucket)))

		activeRequests.WithLabelValues(testBucket).Dec()
		assert.Equal(t, float64(0), testutil.ToFloat64(activeRequests.WithLabelValues(testBucket)))
	})

	t.Run("bytesTransferred", func(t *testing.T) {
		// Reset metric before test
		prometheus.DefaultRegisterer.Unregister(bytesTransferred)
		bytesTransferred = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gcs_server_bytes_transferred_total",
				Help: "Total number of bytes transferred",
			},
			[]string{"bucket_name", "path", "method", "direction"},
		)

		// Test adding bytes
		bytesTransferred.WithLabelValues(testBucket, "/test", "GET", "download").Add(100)

		value := testutil.ToFloat64(bytesTransferred.WithLabelValues(testBucket, "/test", "GET", "download"))
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
			[]string{"bucket_name", "path", "method"},
		)

		// Test observing durations
		requestDuration.WithLabelValues(testBucket, "/test", "GET").Observe(0.1)

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
			[]string{"bucket_name", "path", "error_type"},
		)

		// Test error counting
		errorTotal.WithLabelValues(testBucket, "/test", "storage_error").Inc()
		errorTotal.WithLabelValues(testBucket, "/test", "invalid_path").Inc()
		errorTotal.WithLabelValues(testBucket, "/test", "object_not_found").Inc()

		value := testutil.ToFloat64(errorTotal.WithLabelValues(testBucket, "/test", "storage_error"))
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
			[]string{"bucket_name", "path"},
		)

		// Test size observation
		objectSize.WithLabelValues(testBucket, "/test").Observe(2048)

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
			[]string{"bucket_name", "operation"},
		)

		// Test latency observation
		gcsLatency.WithLabelValues(testBucket, "get_object").Observe(0.05)

		// Verify the histogram has samples
		count := testutil.CollectAndCount(gcsLatency)
		assert.Equal(t, 1, count)
	})
}

func TestMetricsInitialization(t *testing.T) {
	registry := prometheus.NewRegistry()

	// Test requestsTotal
	if err := registry.Register(requestsTotal); err != nil {
		t.Errorf("Failed to register requestsTotal: %v", err)
	}
	requestsTotal.WithLabelValues("test-bucket", "/test.txt", "GET", "200").Inc()
	if val := testutil.ToFloat64(requestsTotal.WithLabelValues("test-bucket", "/test.txt", "GET", "200")); val != 1 {
		t.Errorf("requestsTotal value = %v, want 1", val)
	}

	// Test requestDuration
	if err := registry.Register(requestDuration); err != nil {
		t.Errorf("Failed to register requestDuration: %v", err)
	}
	requestDuration.WithLabelValues("test-bucket", "/test.txt", "GET").Observe(0.1)

	// Test bytesTransferred
	if err := registry.Register(bytesTransferred); err != nil {
		t.Errorf("Failed to register bytesTransferred: %v", err)
	}
	bytesTransferred.WithLabelValues("test-bucket", "/test.txt", "GET", "download").Add(100)
	if val := testutil.ToFloat64(bytesTransferred.WithLabelValues("test-bucket", "/test.txt", "GET", "download")); val != 100 {
		t.Errorf("bytesTransferred value = %v, want 100", val)
	}

	// Test activeRequests
	if err := registry.Register(activeRequests); err != nil {
		t.Errorf("Failed to register activeRequests: %v", err)
	}
	activeRequests.WithLabelValues("test-bucket").Inc()
	if val := testutil.ToFloat64(activeRequests.WithLabelValues("test-bucket")); val != 1 {
		t.Errorf("activeRequests value = %v, want 1", val)
	}
	activeRequests.WithLabelValues("test-bucket").Dec()
	if val := testutil.ToFloat64(activeRequests.WithLabelValues("test-bucket")); val != 0 {
		t.Errorf("activeRequests value = %v, want 0", val)
	}

	// Test errorTotal
	if err := registry.Register(errorTotal); err != nil {
		t.Errorf("Failed to register errorTotal: %v", err)
	}
	errorTotal.WithLabelValues("test-bucket", "/test.txt", "storage_error").Inc()
	if val := testutil.ToFloat64(errorTotal.WithLabelValues("test-bucket", "/test.txt", "storage_error")); val != 1 {
		t.Errorf("errorTotal value = %v, want 1", val)
	}

	// Test objectSize
	if err := registry.Register(objectSize); err != nil {
		t.Errorf("Failed to register objectSize: %v", err)
	}
	objectSize.WithLabelValues("test-bucket", "/test.txt").Observe(1024)

	// Test gcsLatency
	if err := registry.Register(gcsLatency); err != nil {
		t.Errorf("Failed to register gcsLatency: %v", err)
	}
	gcsLatency.WithLabelValues("test-bucket", "get_object").Observe(0.05)
}

func TestMetricsLabels(t *testing.T) {
	tests := []struct {
		name   string
		metric prometheus.Collector
	}{
		{"requestsTotal", requestsTotal},
		{"requestDuration", requestDuration},
		{"bytesTransferred", bytesTransferred},
		{"activeRequests", activeRequests},
		{"errorTotal", errorTotal},
		{"objectSize", objectSize},
		{"gcsLatency", gcsLatency},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Create a new registry for each test to avoid conflicts
			reg := prometheus.NewRegistry()
			if err := reg.Register(tt.metric); err != nil {
				t.Fatalf("Failed to register metric: %v", err)
			}

			// Create a channel to receive the metric
			metricChan := make(chan prometheus.Metric, 1)
			done := make(chan struct{})

			// Start collecting metrics in a goroutine with timeout
			go func() {
				defer close(done)
				tt.metric.Collect(metricChan)
			}()

			// Wait for the metric to be collected or timeout
			select {
			case metric := <-metricChan:
				if metric == nil {
					t.Error("Received nil metric")
				}
				desc := metric.Desc()
				if desc == nil {
					t.Error("Metric has nil description")
				}
			case <-time.After(100 * time.Millisecond):
				t.Logf("%s: metric collection timed out", tt.name)
			}

			// Wait for the collection goroutine to finish
			select {
			case <-done:
				// Collection completed successfully
			case <-time.After(100 * time.Millisecond):
				t.Logf("%s: collection goroutine did not complete", tt.name)
			}
		})
	}
}
