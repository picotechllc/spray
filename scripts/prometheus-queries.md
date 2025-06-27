# Prometheus Queries for HTTP Cache Performance Monitoring

## Cache Hit Rate Metrics

### Overall Cache Hit Rate
```promql
# Cache hit rate percentage
rate(gcs_server_cache_total{status="hit"}[5m]) / 
(rate(gcs_server_cache_total{status="hit"}[5m]) + rate(gcs_server_cache_total{status="miss"}[5m])) * 100
```

### Cache Hit Rate by Bucket
```promql
# Cache hit rate by bucket
rate(gcs_server_cache_total{status="hit"}[5m]) / 
(rate(gcs_server_cache_total{status="hit"}[5m]) + rate(gcs_server_cache_total{status="miss"}[5m])) * 100
```

### Cache Status Over Time
```promql
# Cache hits per second
rate(gcs_server_cache_total{status="hit"}[5m])

# Cache misses per second  
rate(gcs_server_cache_total{status="miss"}[5m])
```

## Response Time Improvements

### Average Response Time by Status Code
```promql
# Average response time for 304 responses (cached)
rate(gcs_server_request_duration_seconds_sum{status="304"}[5m]) / 
rate(gcs_server_request_duration_seconds_count{status="304"}[5m])

# Average response time for 200 responses (full content)
rate(gcs_server_request_duration_seconds_sum{status="200"}[5m]) / 
rate(gcs_server_request_duration_seconds_count{status="200"}[5m])
```

### Response Time Distribution
```promql
# 95th percentile response time
histogram_quantile(0.95, rate(gcs_server_request_duration_seconds_bucket[5m]))

# 50th percentile response time
histogram_quantile(0.50, rate(gcs_server_request_duration_seconds_bucket[5m]))
```

## Bandwidth Savings

### Bytes Transferred Reduction
```promql
# Total bytes transferred per second
rate(gcs_server_bytes_transferred_total[5m])

# Bytes saved by 304 responses (estimate)
rate(gcs_server_cache_total{status="hit"}[5m]) * 
avg_over_time(gcs_server_object_size_bytes[5m])
```

### Request Status Distribution
```promql
# 304 responses per second
rate(gcs_server_requests_total{status="304"}[5m])

# 200 responses per second
rate(gcs_server_requests_total{status="200"}[5m])

# Percentage of 304 responses
rate(gcs_server_requests_total{status="304"}[5m]) / 
rate(gcs_server_requests_total[5m]) * 100
```

## GCS Operations Saved

### Operations Skipped Due to Cache
```promql
# GCS operations skipped per second
rate(gcs_server_storage_operations_skipped_total[5m])

# Total GCS operations per second
rate(gcs_server_storage_operation_duration_seconds_count[5m])

# Percentage of operations saved
rate(gcs_server_storage_operations_skipped_total[5m]) / 
(rate(gcs_server_storage_operations_skipped_total[5m]) + rate(gcs_server_storage_operation_duration_seconds_count[5m])) * 100
```

## Cache Header Effectiveness

### Cache Headers Set by Content Type
```promql
# Cache headers set per second by content type
rate(gcs_server_cache_headers_total[5m])

# Cache headers by policy (short/medium/long)
sum(rate(gcs_server_cache_headers_total[5m])) by (cache_policy)
```

### Conditional Request Performance
```promql
# Conditional requests per second by type
rate(gcs_server_conditional_requests_total[5m])

# ETag validation success rate
rate(gcs_server_conditional_requests_total{type="etag",result="hit"}[5m]) / 
rate(gcs_server_conditional_requests_total{type="etag"}[5m]) * 100
```

## Alert Rules

### Cache Hit Rate Too Low
```promql
# Alert when cache hit rate falls below 70%
rate(gcs_server_cache_total{status="hit"}[5m]) / 
(rate(gcs_server_cache_total{status="hit"}[5m]) + rate(gcs_server_cache_total{status="miss"}[5m])) * 100 < 70
```

### High Response Time
```promql
# Alert when 95th percentile response time exceeds 500ms
histogram_quantile(0.95, rate(gcs_server_request_duration_seconds_bucket[5m])) > 0.5
```

### Low Cache Header Coverage
```promql
# Alert when cache headers are not being set
rate(gcs_server_cache_headers_total[5m]) == 0
```

## Dashboard Panels

### Cache Performance Overview
- Cache hit rate gauge (target: >80%)
- Cache hits/misses timeline
- Response time comparison (304 vs 200)
- Bandwidth savings counter

### Response Time Analysis
- Average response time by status code
- Response time distribution histogram
- 95th percentile response time over time

### GCS Operations Efficiency
- Operations skipped counter
- GCS operation latency
- Cost savings estimate

### Content Type Analysis
- Cache policy distribution
- Cache headers by content type
- Most/least cached content types

## Sample Grafana Dashboard JSON

```json
{
  "dashboard": {
    "title": "Spray HTTP Cache Performance",
    "panels": [
      {
        "title": "Cache Hit Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "rate(gcs_server_cache_total{status=\"hit\"}[5m]) / (rate(gcs_server_cache_total{status=\"hit\"}[5m]) + rate(gcs_server_cache_total{status=\"miss\"}[5m])) * 100"
          }
        ]
      },
      {
        "title": "Response Time Comparison",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(gcs_server_request_duration_seconds_sum{status=\"304\"}[5m]) / rate(gcs_server_request_duration_seconds_count{status=\"304\"}[5m])",
            "legend": "304 Not Modified"
          },
          {
            "expr": "rate(gcs_server_request_duration_seconds_sum{status=\"200\"}[5m]) / rate(gcs_server_request_duration_seconds_count{status=\"200\"}[5m])",
            "legend": "200 OK"
          }
        ]
      }
    ]
  }
}
```

## Expected Performance Improvements

Based on typical HTTP cache implementations, you should expect:

1. **Cache Hit Rate**: 80-95% for repeat requests
2. **Response Time**: 50-90% reduction for cached responses
3. **Bandwidth Savings**: 60-85% reduction in bytes transferred
4. **GCS Operations**: 70-90% reduction in storage operations
5. **Cost Savings**: Proportional to GCS operation reduction

## Monitoring Best Practices

1. **Set up alerts** for cache hit rate < 70%
2. **Monitor response times** during peak traffic
3. **Track bandwidth savings** to quantify cost impact
4. **Review cache policies** based on usage patterns
5. **Correlate with application metrics** for full picture 