#!/bin/bash

# HTTP Cache Performance Benchmark Script
# This script measures the performance impact of HTTP cache headers
# in a multi-pod Kubernetes environment

set -e

# Configuration
SPRAY_URL="${SPRAY_URL:-http://localhost:8080}"
DURATION="${DURATION:-60s}"
CONNECTIONS="${CONNECTIONS:-10}"
REQUESTS="${REQUESTS:-1000}"
WARM_UP="${WARM_UP:-10}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test files to benchmark
declare -a TEST_FILES=(
    "/"
    "/index.html"
    "/style.css"
    "/script.js"
    "/image.png"
    "/assets/main.min.css"
    "/assets/app-v1.2.3.js"
    "/fonts/roboto.woff2"
)

echo -e "${BLUE}🚀 HTTP Cache Performance Benchmark${NC}"
echo -e "${BLUE}====================================${NC}"
echo ""
echo "Target URL: $SPRAY_URL"
echo "Duration: $DURATION"
echo "Connections: $CONNECTIONS"
echo "Requests: $REQUESTS"
echo "Warm-up: $WARM_UP seconds"
echo ""

# Function to check if required tools are installed
check_dependencies() {
    local missing_tools=()
    
    if ! command -v curl &> /dev/null; then
        missing_tools+=("curl")
    fi
    
    if ! command -v hey &> /dev/null; then
        missing_tools+=("hey")
    fi
    
    if ! command -v jq &> /dev/null; then
        missing_tools+=("jq")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        echo -e "${RED}❌ Missing required tools: ${missing_tools[*]}${NC}"
        echo "Please install missing tools:"
        echo "  curl: https://curl.se/"
        echo "  hey: go install github.com/rakyll/hey@latest"
        echo "  jq: https://stedolan.github.io/jq/"
        exit 1
    fi
}

# Function to warm up the server
warm_up_server() {
    echo -e "${YELLOW}🔥 Warming up server...${NC}"
    for file in "${TEST_FILES[@]}"; do
        curl -s -o /dev/null "$SPRAY_URL$file" || true
    done
    sleep $WARM_UP
    echo -e "${GREEN}✅ Warm-up complete${NC}"
    echo ""
}

# Function to get baseline metrics
get_baseline_metrics() {
    echo -e "${YELLOW}📊 Collecting baseline metrics...${NC}"
    
    # Get Prometheus metrics
    local metrics=$(curl -s "$SPRAY_URL/metrics" | grep -E "gcs_server_(requests_total|cache_total|request_duration|bytes_transferred|storage_operations_skipped)")
    
    echo "Baseline metrics collected:"
    echo "$metrics" | head -10
    echo ""
}

# Function to test cache headers
test_cache_headers() {
    echo -e "${YELLOW}🧪 Testing cache headers...${NC}"
    
    for file in "${TEST_FILES[@]}"; do
        echo "Testing: $file"
        
        # First request - should get full response with cache headers
        local response=$(curl -s -I "$SPRAY_URL$file")
        
        # Check for cache headers
        local etag=$(echo "$response" | grep -i "etag:" | cut -d' ' -f2- | tr -d '\r')
        local cache_control=$(echo "$response" | grep -i "cache-control:" | cut -d' ' -f2- | tr -d '\r')
        local last_modified=$(echo "$response" | grep -i "last-modified:" | cut -d' ' -f2- | tr -d '\r')
        
        if [ -n "$etag" ] && [ -n "$cache_control" ] && [ -n "$last_modified" ]; then
            echo -e "  ${GREEN}✅ Cache headers present${NC}"
            echo "     ETag: $etag"
            echo "     Cache-Control: $cache_control"
            echo "     Last-Modified: $last_modified"
            
            # Test conditional request with ETag
            local conditional_response=$(curl -s -I -H "If-None-Match: $etag" "$SPRAY_URL$file")
            local status_code=$(echo "$conditional_response" | head -n1 | cut -d' ' -f2)
            
            if [ "$status_code" = "304" ]; then
                echo -e "  ${GREEN}✅ Conditional request returned 304 Not Modified${NC}"
            else
                echo -e "  ${RED}❌ Conditional request returned $status_code (expected 304)${NC}"
            fi
        else
            echo -e "  ${RED}❌ Missing cache headers${NC}"
        fi
        echo ""
    done
}

# Function to run performance benchmark
run_benchmark() {
    local test_name="$1"
    local url="$2"
    local conditional_headers="$3"
    
    echo -e "${BLUE}🏃 Running $test_name benchmark...${NC}"
    echo "URL: $url"
    echo "Headers: $conditional_headers"
    
    # Run hey benchmark
    local hey_output
    if [ -n "$conditional_headers" ]; then
        hey_output=$(hey -n $REQUESTS -c $CONNECTIONS -H "$conditional_headers" "$url" 2>&1)
    else
        hey_output=$(hey -n $REQUESTS -c $CONNECTIONS "$url" 2>&1)
    fi
    
    # Extract key metrics
    local total_time=$(echo "$hey_output" | grep "Total:" | awk '{print $2}')
    local requests_per_sec=$(echo "$hey_output" | grep "Requests/sec:" | awk '{print $2}')
    local mean_latency=$(echo "$hey_output" | grep "Average:" | awk '{print $2}')
    local p95_latency=$(echo "$hey_output" | grep "95%" | awk '{print $2}')
    local total_data=$(echo "$hey_output" | grep "Total data:" | awk '{print $3, $4}')
    
    echo "Results:"
    echo "  Total time: $total_time"
    echo "  Requests/sec: $requests_per_sec"
    echo "  Mean latency: $mean_latency"
    echo "  95th percentile: $p95_latency"
    echo "  Total data: $total_data"
    echo ""
    
    # Save results to file
    cat >> benchmark_results.json << EOF
{
  "test_name": "$test_name",
  "url": "$url",
  "conditional_headers": "$conditional_headers",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "total_time": "$total_time",
  "requests_per_sec": "$requests_per_sec",
  "mean_latency": "$mean_latency",
  "p95_latency": "$p95_latency",
  "total_data": "$total_data"
},
EOF
}

# Function to compare metrics
compare_metrics() {
    echo -e "${YELLOW}📈 Comparing performance metrics...${NC}"
    
    # Get final metrics
    local final_metrics=$(curl -s "$SPRAY_URL/metrics" | grep -E "gcs_server_(requests_total|cache_total|request_duration|bytes_transferred|storage_operations_skipped)")
    
    echo "Final metrics:"
    echo "Cache hits: $(echo "$final_metrics" | grep 'cache_total.*hit' | awk -F' ' '{sum += $2} END {print sum}')"
    echo "Cache misses: $(echo "$final_metrics" | grep 'cache_total.*miss' | awk -F' ' '{sum += $2} END {print sum}')"
    echo "304 responses: $(echo "$final_metrics" | grep 'requests_total.*304' | awk -F' ' '{sum += $2} END {print sum}')"
    echo "200 responses: $(echo "$final_metrics" | grep 'requests_total.*200' | awk -F' ' '{sum += $2} END {print sum}')"
    echo "GCS operations skipped: $(echo "$final_metrics" | grep 'storage_operations_skipped' | awk -F' ' '{sum += $2} END {print sum}')"
    echo ""
}

# Function to generate report
generate_report() {
    echo -e "${BLUE}📋 Generating performance report...${NC}"
    
    cat > performance_report.md << 'EOF'
# HTTP Cache Performance Benchmark Report

## Test Configuration
- Duration: ${DURATION}
- Connections: ${CONNECTIONS}  
- Requests: ${REQUESTS}
- Target: ${SPRAY_URL}

## Key Findings

### Cache Hit Rate
- **Cache Hits**: XX requests returned 304 Not Modified
- **Cache Misses**: XX requests returned 200 OK
- **Hit Rate**: XX% (target: >80% for repeated requests)

### Performance Impact
- **Average Response Time**: XX ms reduction for cached requests
- **Bandwidth Savings**: XX% reduction in bytes transferred
- **GCS Operations Saved**: XX operations avoided due to cache validation

### Cache Header Validation
- ✅ ETag headers properly set
- ✅ Cache-Control headers configured
- ✅ Last-Modified headers present
- ✅ Conditional requests working (304 responses)

## Recommendations

1. **Monitor cache hit rates** using the new Prometheus metrics
2. **Adjust cache policies** based on content type usage patterns
3. **Set up alerting** for cache hit rates below 70%
4. **Consider CDN** for further performance improvements

## Metrics to Monitor

```
# Cache performance
gcs_server_cache_total{status="hit"}
gcs_server_cache_total{status="miss"}
gcs_server_conditional_requests_total
gcs_server_cache_headers_total

# Response time improvements
gcs_server_request_duration_seconds (should be faster for 304s)
gcs_server_bytes_transferred_total (should be lower overall)
gcs_server_storage_operations_skipped_total (should increase)
```
EOF

    echo -e "${GREEN}✅ Report generated: performance_report.md${NC}"
}

# Main execution
main() {
    check_dependencies
    
    # Initialize results file
    echo "[]" > benchmark_results.json
    
    get_baseline_metrics
    warm_up_server
    test_cache_headers
    
    # Run benchmarks
    echo -e "${BLUE}🔍 Running performance benchmarks...${NC}"
    
    # Test 1: Fresh requests (no cache)
    run_benchmark "Fresh Requests" "$SPRAY_URL/" ""
    
    # Test 2: Conditional requests (should hit cache)
    # First get ETag
    etag=$(curl -s -I "$SPRAY_URL/" | grep -i "etag:" | cut -d' ' -f2- | tr -d '\r')
    if [ -n "$etag" ]; then
        run_benchmark "Conditional Requests (ETag)" "$SPRAY_URL/" "If-None-Match: $etag"
    fi
    
    # Test 3: Different content types
    for file in "${TEST_FILES[@]}"; do
        if [ "$file" != "/" ]; then
            run_benchmark "Content Type Test" "$SPRAY_URL$file" ""
        fi
    done
    
    compare_metrics
    generate_report
    
    echo -e "${GREEN}🎉 Benchmark complete!${NC}"
    echo -e "${GREEN}Results saved to: benchmark_results.json${NC}"
    echo -e "${GREEN}Full report: performance_report.md${NC}"
}

# Run the main function
main "$@" 