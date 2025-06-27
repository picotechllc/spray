# HTTP Cache Performance Testing with k6

This directory contains comprehensive performance testing tools for measuring the impact of HTTP cache headers in spray's multi-pod Kubernetes deployment.

## 🚀 Quick Start

### Prerequisites

1. **Install k6** (load testing tool):
   ```powershell
   # Using PowerShell script (Windows)
   .\scripts\run-k6-tests.ps1 -InstallK6
   
   # Or manually:
   # Via Chocolatey: choco install k6
   # Via winget: winget install k6
   # Or download from: https://k6.io/docs/get-started/installation/
   ```

2. **Start your spray server** (locally or remote)

### Run Tests

```powershell
# Quick test (2 minutes)
.\scripts\run-k6-tests.ps1 -TestType quick

# Comprehensive test (18 minutes)
.\scripts\run-k6-tests.ps1 -TestType comprehensive

# Load test for production validation (20 minutes)
.\scripts\run-k6-tests.ps1 -TestType load

# Test against custom URL
.\scripts\run-k6-tests.ps1 -SprayUrl "https://your-spray-instance.com"

# Save results to JSON
.\scripts\run-k6-tests.ps1 -Output json
```

## 📊 What Gets Measured

### Cache Performance Metrics
- **Cache Hit Rate**: Percentage of requests returning 304 Not Modified
- **Response Time Reduction**: Speed improvement for cached responses
- **Bandwidth Savings**: Bytes saved by returning 304 instead of full content
- **ETag Validation**: Success rate of ETag-based cache validation
- **Last-Modified Validation**: Success rate of Last-Modified-based validation

### Performance Thresholds
- Cache hit rate: **>80%** (for repeated requests)
- 304 response time: **<100ms**
- Overall 95th percentile: **<500ms**
- Error rate: **<1%**

## 🧪 Test Scenarios

### 1. Quick Test (2 minutes)
- **Purpose**: Fast validation of cache implementation
- **Virtual Users**: 5-10
- **Scenarios**: Baseline + Cache validation
- **Use Case**: Development and CI/CD

### 2. Comprehensive Test (18 minutes)
- **Purpose**: Full cache performance analysis
- **Virtual Users**: 10-100
- **Scenarios**: 
  - Baseline (fresh requests)
  - ETag cache validation
  - Last-Modified validation
  - Mixed content types
  - Stress testing
- **Use Case**: Pre-deployment validation

### 3. Load Test (20 minutes)
- **Purpose**: Production-scale validation
- **Virtual Users**: 50-300 (ramping)
- **Scenarios**: High-load cache stress test
- **Use Case**: Production readiness

## 📈 Performance Expectations

Based on typical HTTP cache implementations:

| Metric | Expected Improvement |
|--------|---------------------|
| **Cache Hit Rate** | 80-95% for repeat requests |
| **Response Time** | 50-90% reduction for 304s |
| **Bandwidth Savings** | 60-85% reduction |
| **GCS Operations** | 70-90% reduction |
| **Cost Savings** | Proportional to GCS reduction |

## 🔍 Monitoring During Tests

### Real-time Monitoring
```bash
# Watch Prometheus metrics during test
curl -s http://localhost:8080/metrics | grep -E "cache_total|requests_total.*304|storage_operations_skipped"

# Monitor cache hit rate
curl -s http://localhost:8080/metrics | grep "cache_total.*hit"
```

### Key Prometheus Metrics
```promql
# Cache hit rate
rate(gcs_server_cache_total{status="hit"}[5m]) / 
(rate(gcs_server_cache_total{status="hit"}[5m]) + rate(gcs_server_cache_total{status="miss"}[5m])) * 100

# Response time comparison
rate(gcs_server_request_duration_seconds_sum{status="304"}[5m]) / 
rate(gcs_server_request_duration_seconds_count{status="304"}[5m])

# Bandwidth savings
rate(gcs_server_storage_operations_skipped_total[5m])
```

## 📋 Test Results Analysis

### k6 Output Interpretation

```
✓ cache_hit_rate................: 85.23%  ✓ 1234 ✗ 212
✓ etag_304_rate.................: 92.15%  ✓ 1456 ✗ 124
✓ response_time_reduction_ms....: avg=245ms min=50ms med=200ms max=500ms p(95)=400ms
✓ bandwidth_saved_bytes.........: 15.2MB
```

**Good Results:**
- Cache hit rate >80%
- ETag validation >90%
- Response time reduction >50%
- Low error rates

**Concerning Results:**
- Cache hit rate <70%
- High error rates
- Slow 304 responses (>200ms)

### Grafana Dashboard

Use the provided Prometheus queries to create dashboards:
- Cache hit rate gauge
- Response time comparison
- Bandwidth savings over time
- GCS operations saved

## 🛠️ Troubleshooting

### Common Issues

1. **Low Cache Hit Rate**
   - Check ETag generation consistency
   - Verify Last-Modified headers
   - Ensure cache headers are set correctly

2. **Slow 304 Responses**
   - Check GCS latency
   - Verify conditional request logic
   - Monitor server resource usage

3. **High Error Rates**
   - Check server logs
   - Verify GCS permissions
   - Monitor network connectivity

### Debug Mode
```powershell
# Run with verbose output
.\scripts\run-k6-tests.ps1 -Verbose

# Check specific test file
k6 run scripts/cache-performance-test.js --verbose
```

## 🔧 Customization

### Modify Test Parameters

Edit `scripts/cache-performance-test.js`:
```javascript
// Adjust virtual users
vus: 20,

// Change test duration
duration: '5m',

// Add custom test files
const TEST_FILES = [
  { path: '/your-file.css', expectedType: 'text/css', cachePolicy: 'long' },
  // ...
];
```

### Custom Cache Policies

Update cache policy logic in `server.go`:
```go
func getCachePolicy(contentType, path string) (maxAge int, policy string) {
    // Your custom logic here
}
```

## 📊 Integration with CI/CD

### GitHub Actions Example
```yaml
- name: Run Cache Performance Tests
  run: |
    .\scripts\run-k6-tests.ps1 -TestType quick -Output json
    
- name: Upload Results
  uses: actions/upload-artifact@v3
  with:
    name: k6-results
    path: k6-results-*.json
```

### Performance Regression Detection
```powershell
# Compare results between versions
$baseline = Get-Content "baseline-results.json" | ConvertFrom-Json
$current = Get-Content "current-results.json" | ConvertFrom-Json

# Alert if cache hit rate drops >5%
if ($current.cache_hit_rate -lt ($baseline.cache_hit_rate - 0.05)) {
    Write-Error "Cache hit rate regression detected!"
}
```

## 📚 Additional Resources

- [k6 Documentation](https://k6.io/docs/)
- [Prometheus Queries Reference](./prometheus-queries.md)
- [HTTP Cache Best Practices](https://developer.mozilla.org/en-US/docs/Web/HTTP/Caching)
- [Grafana k6 Cloud](https://k6.io/cloud/)

## 🤝 Contributing

To add new test scenarios:

1. Add scenario to `cache-performance-test.js`
2. Update test configuration in `run-k6-tests.ps1`
3. Add corresponding Prometheus queries
4. Update this documentation

## 📝 Example Commands

```powershell
# Development workflow
.\scripts\run-k6-tests.ps1 -TestType quick -Verbose

# Pre-deployment validation
.\scripts\run-k6-tests.ps1 -TestType comprehensive -Output json

# Production monitoring
.\scripts\run-k6-tests.ps1 -TestType load -Environment production -Output prometheus

# Custom testing
.\scripts\run-k6-tests.ps1 -SprayUrl "http://localhost:3000" -TestType quick
``` 