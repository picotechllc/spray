import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics for cache performance
export const cacheHitRate = new Rate('cache_hit_rate');
export const conditionalRequestRate = new Rate('conditional_request_rate');
export const bandwidthSaved = new Counter('bandwidth_saved_bytes');
export const responseTimeReduction = new Trend('response_time_reduction_ms');
export const etag304Rate = new Rate('etag_304_rate');
export const lastModified304Rate = new Rate('last_modified_304_rate');
export const cacheBypassRate = new Rate('cache_bypass_rate');
export const featureFlagEffectiveness = new Rate('feature_flag_effectiveness');

// Test configuration
export const options = {
  scenarios: {
    // Scenario 1: Baseline - Fresh requests (no cache)
    baseline_fresh_requests: {
      executor: 'constant-vus',
      vus: 10,
      duration: '2m',
      tags: { scenario: 'baseline' },
      exec: 'testFreshRequests',
    },
    
    // Scenario 2: Cache validation with ETag
    cache_etag_validation: {
      executor: 'constant-vus',
      vus: 15,
      duration: '3m',
      startTime: '2m30s',
      tags: { scenario: 'etag_cache' },
      exec: 'testETagCacheValidation',
    },
    
    // Scenario 3: Cache validation with Last-Modified
    cache_lastmod_validation: {
      executor: 'constant-vus',
      vus: 15,
      duration: '3m', 
      startTime: '6m',
      tags: { scenario: 'lastmod_cache' },
      exec: 'testLastModifiedCacheValidation',
    },
    
    // Scenario 4: Mixed content types
    mixed_content_types: {
      executor: 'constant-vus',
      vus: 20,
      duration: '4m',
      startTime: '9m30s',
      tags: { scenario: 'mixed_content' },
      exec: 'testMixedContentTypes',
    },
    
    // Scenario 5: High load cache stress test
    cache_stress_test: {
      executor: 'ramping-vus',
      startVUs: 10,
      stages: [
        { duration: '1m', target: 50 },
        { duration: '2m', target: 100 },
        { duration: '1m', target: 50 },
        { duration: '1m', target: 10 },
      ],
      startTime: '14m',
      tags: { scenario: 'stress_test' },
      exec: 'testCacheStressTest',
    },
    
    // Scenario 6: Feature flag testing
    feature_flag_test: {
      executor: 'constant-vus',
      vus: 10,
      duration: '2m',
      startTime: '19m',
      tags: { scenario: 'feature_flags' },
      exec: 'testFeatureFlags',
    },
  },
  
  thresholds: {
    // Performance thresholds
    'http_req_duration': ['p(95)<500'], // 95% of requests should be under 500ms
    'http_req_duration{status:304}': ['p(95)<100'], // Cached responses should be under 100ms
    'cache_hit_rate': ['rate>0.8'], // Cache hit rate should be above 80%
    'etag_304_rate': ['rate>0.9'], // ETag validation should work 90% of the time
    'http_req_failed': ['rate<0.01'], // Error rate should be below 1%
  },
};

// Base URL configuration
const BASE_URL = __ENV.SPRAY_URL || 'http://localhost:8080';

// Test files with different characteristics
const TEST_FILES = [
  { path: '/', expectedType: 'text/html', cachePolicy: 'short' },
  { path: '/index.html', expectedType: 'text/html', cachePolicy: 'short' },
  { path: '/style.css', expectedType: 'text/css', cachePolicy: 'medium' },
  { path: '/script.js', expectedType: 'application/javascript', cachePolicy: 'medium' },
  { path: '/image.png', expectedType: 'image/png', cachePolicy: 'medium' },
  { path: '/assets/main.min.css', expectedType: 'text/css', cachePolicy: 'long' },
  { path: '/assets/app-v1.2.3.js', expectedType: 'application/javascript', cachePolicy: 'long' },
  { path: '/fonts/roboto.woff2', expectedType: 'font/woff2', cachePolicy: 'long' },
];

// Helper function to get random test file
function getRandomTestFile() {
  return TEST_FILES[Math.floor(Math.random() * TEST_FILES.length)];
}

// Helper function to validate cache headers
function validateCacheHeaders(response, expectedPolicy) {
  const checks = {
    'has ETag header': response.headers['Etag'] !== undefined,
    'has Cache-Control header': response.headers['Cache-Control'] !== undefined,
    'has Last-Modified header': response.headers['Last-Modified'] !== undefined,
  };
  
  // Validate cache policy
  const cacheControl = response.headers['Cache-Control'] || '';
  if (expectedPolicy === 'short') {
    checks['short cache policy'] = cacheControl.includes('max-age=300') || cacheControl.includes('max-age=3600');
  } else if (expectedPolicy === 'medium') {
    checks['medium cache policy'] = cacheControl.includes('max-age=86400') || cacheControl.includes('max-age=3600');
  } else if (expectedPolicy === 'long') {
    checks['long cache policy'] = cacheControl.includes('max-age=31536000');
  }
  
  return checks;
}

// Scenario 1: Test fresh requests (baseline)
export function testFreshRequests() {
  group('Fresh Requests (Baseline)', () => {
    const testFile = getRandomTestFile();
    
    const response = http.get(`${BASE_URL}${testFile.path}`, {
      headers: {
        'Cache-Control': 'no-cache', // Force fresh request
      },
    });
    
    const isSuccess = check(response, {
      'status is 200': (r) => r.status === 200,
      'response time < 2000ms': (r) => r.timings.duration < 2000,
      ...validateCacheHeaders(response, testFile.cachePolicy),
    });
    
    if (isSuccess) {
      // Store baseline metrics for comparison
      response.etag = response.headers['Etag'];
      response.lastModified = response.headers['Last-Modified'];
      response.contentLength = parseInt(response.headers['Content-Length'] || response.body.length);
    }
  });
}

// Scenario 2: Test ETag cache validation
export function testETagCacheValidation() {
  group('ETag Cache Validation', () => {
    const testFile = getRandomTestFile();
    
    // First request to get ETag
    const initialResponse = http.get(`${BASE_URL}${testFile.path}`);
    const etag = initialResponse.headers['Etag'];
    
    if (!etag) {
      console.log(`No ETag found for ${testFile.path}`);
      return;
    }
    
    // Second request with If-None-Match header
    const conditionalResponse = http.get(`${BASE_URL}${testFile.path}`, {
      headers: {
        'If-None-Match': etag,
      },
    });
    
    const isCacheHit = conditionalResponse.status === 304;
    const responseTimeReduced = initialResponse.timings.duration - conditionalResponse.timings.duration;
    
    check(conditionalResponse, {
      'conditional request returns 304': (r) => r.status === 304,
      'ETag matches': (r) => r.headers['Etag'] === etag || r.status === 304,
      'response time improved': (r) => conditionalResponse.timings.duration < initialResponse.timings.duration,
      'no body content on 304': (r) => r.status !== 304 || r.body.length === 0,
    });
    
    // Record metrics
    cacheHitRate.add(isCacheHit ? 1 : 0);
    etag304Rate.add(isCacheHit ? 1 : 0);
    conditionalRequestRate.add(1);
    
    if (isCacheHit) {
      bandwidthSaved.add(initialResponse.body.length);
      responseTimeReduction.add(responseTimeReduced);
    }
  });
}

// Scenario 3: Test Last-Modified cache validation
export function testLastModifiedCacheValidation() {
  group('Last-Modified Cache Validation', () => {
    const testFile = getRandomTestFile();
    
    // First request to get Last-Modified
    const initialResponse = http.get(`${BASE_URL}${testFile.path}`);
    const lastModified = initialResponse.headers['Last-Modified'];
    
    if (!lastModified) {
      console.log(`No Last-Modified found for ${testFile.path}`);
      return;
    }
    
    // Second request with If-Modified-Since header
    const conditionalResponse = http.get(`${BASE_URL}${testFile.path}`, {
      headers: {
        'If-Modified-Since': lastModified,
      },
    });
    
    const isCacheHit = conditionalResponse.status === 304;
    const responseTimeReduced = initialResponse.timings.duration - conditionalResponse.timings.duration;
    
    check(conditionalResponse, {
      'conditional request returns 304': (r) => r.status === 304,
      'Last-Modified header present': (r) => r.headers['Last-Modified'] !== undefined || r.status === 304,
      'response time improved': (r) => conditionalResponse.timings.duration < initialResponse.timings.duration,
      'no body content on 304': (r) => r.status !== 304 || r.body.length === 0,
    });
    
    // Record metrics
    cacheHitRate.add(isCacheHit ? 1 : 0);
    lastModified304Rate.add(isCacheHit ? 1 : 0);
    conditionalRequestRate.add(1);
    
    if (isCacheHit) {
      bandwidthSaved.add(initialResponse.body.length);
      responseTimeReduction.add(responseTimeReduced);
    }
  });
}

// Scenario 4: Test mixed content types
export function testMixedContentTypes() {
  group('Mixed Content Types', () => {
    // Test all file types
    TEST_FILES.forEach((testFile) => {
      const response = http.get(`${BASE_URL}${testFile.path}`);
      
      check(response, {
        [`${testFile.path} - status is 200`]: (r) => r.status === 200,
        [`${testFile.path} - correct content type`]: (r) => 
          r.headers['Content-Type'] && r.headers['Content-Type'].includes(testFile.expectedType.split('/')[0]),
        [`${testFile.path} - has cache headers`]: (r) => 
          r.headers['Etag'] && r.headers['Cache-Control'] && r.headers['Last-Modified'],
      });
      
      // Test conditional request for each file type
      const etag = response.headers['Etag'];
      if (etag) {
        const conditionalResponse = http.get(`${BASE_URL}${testFile.path}`, {
          headers: { 'If-None-Match': etag },
        });
        
        const isCacheHit = conditionalResponse.status === 304;
        cacheHitRate.add(isCacheHit ? 1 : 0);
        
        if (isCacheHit) {
          bandwidthSaved.add(response.body.length);
        }
      }
    });
  });
}

// Scenario 5: Cache stress test
export function testCacheStressTest() {
  group('Cache Stress Test', () => {
    const testFile = getRandomTestFile();
    
    // Alternate between fresh and conditional requests
    const useConditionalRequest = Math.random() > 0.3; // 70% conditional requests
    
    let response;
    if (useConditionalRequest) {
      // Get ETag first
      const initialResponse = http.get(`${BASE_URL}${testFile.path}`);
      const etag = initialResponse.headers['Etag'];
      
      if (etag) {
        response = http.get(`${BASE_URL}${testFile.path}`, {
          headers: { 'If-None-Match': etag },
        });
        
        const isCacheHit = response.status === 304;
        cacheHitRate.add(isCacheHit ? 1 : 0);
        
        if (isCacheHit) {
          bandwidthSaved.add(initialResponse.body.length);
        }
      } else {
        response = initialResponse;
      }
    } else {
      response = http.get(`${BASE_URL}${testFile.path}`);
    }
    
    check(response, {
      'status is 200 or 304': (r) => r.status === 200 || r.status === 304,
      'response time acceptable': (r) => r.timings.duration < (r.status === 304 ? 200 : 1000),
    });
  });
}

// Setup function - runs once before all scenarios
export function setup() {
  console.log('🚀 Starting HTTP Cache Performance Tests');
  console.log(`Target URL: ${BASE_URL}`);
  
  // Verify server is accessible
  const healthCheck = http.get(`${BASE_URL}/readyz`);
  if (healthCheck.status !== 200) {
    throw new Error(`Server health check failed: ${healthCheck.status}`);
  }
  
  console.log('✅ Server health check passed');
  return { baseUrl: BASE_URL };
}

// Scenario 6: Test feature flag effectiveness
export function testFeatureFlags() {
  group('Feature Flag Testing', () => {
    const testFile = getRandomTestFile();
    
    // Test with different User-Agent strings to test rollout rules
    const userAgents = [
      'Mozilla/5.0 (Chrome/91.0) AppleWebKit/537.36',
      'Mozilla/5.0 (Firefox/89.0) Gecko/20100101',
      'curl/7.68.0',
      'k6-test-bot/1.0',
    ];
    
    const userAgent = userAgents[Math.floor(Math.random() * userAgents.length)];
    
    const response = http.get(`${BASE_URL}${testFile.path}`, {
      headers: {
        'User-Agent': userAgent,
      },
    });
    
    // Check if cache headers are present
    const hasCacheHeaders = response.headers['Etag'] && 
                           response.headers['Cache-Control'] && 
                           response.headers['Last-Modified'];
    
    check(response, {
      'status is 200': (r) => r.status === 200,
      'response time acceptable': (r) => r.timings.duration < 2000,
    });
    
    // Track feature flag effectiveness
    featureFlagEffectiveness.add(hasCacheHeaders ? 1 : 0);
    
    if (!hasCacheHeaders) {
      // Cache is disabled/bypassed for this request
      cacheBypassRate.add(1);
    } else {
      // Test conditional request if cache headers are present
      const etag = response.headers['Etag'];
      if (etag) {
        const conditionalResponse = http.get(`${BASE_URL}${testFile.path}`, {
          headers: {
            'User-Agent': userAgent,
            'If-None-Match': etag,
          },
        });
        
        const isCacheHit = conditionalResponse.status === 304;
        cacheHitRate.add(isCacheHit ? 1 : 0);
        
        if (isCacheHit) {
          bandwidthSaved.add(response.body.length);
        }
      }
    }
  });
}

// Teardown function - runs once after all scenarios
export function teardown(data) {
  console.log('📊 Test Summary:');
  console.log(`- Base URL: ${data.baseUrl}`);
  console.log('- All scenarios completed');
  console.log('- Check the k6 output for detailed metrics');
  
  // Optional: Fetch final Prometheus metrics
  try {
    const metricsResponse = http.get(`${BASE_URL}/metrics`);
    if (metricsResponse.status === 200) {
      console.log('✅ Prometheus metrics endpoint accessible');
      
      // Parse and display cache-related metrics
      const metricsText = metricsResponse.body;
      const cacheHits = (metricsText.match(/gcs_server_cache_total{.*status="hit".*} (\d+)/g) || [])
        .reduce((sum, line) => sum + parseInt(line.split(' ')[1]), 0);
      const cacheMisses = (metricsText.match(/gcs_server_cache_total{.*status="miss".*} (\d+)/g) || [])
        .reduce((sum, line) => sum + parseInt(line.split(' ')[1]), 0);
      const cacheBypasses = (metricsText.match(/gcs_server_cache_total{.*status="bypass".*} (\d+)/g) || [])
        .reduce((sum, line) => sum + parseInt(line.split(' ')[1]), 0);
      
      console.log(`📈 Prometheus Cache Metrics:`);
      console.log(`- Cache Hits: ${cacheHits}`);
      console.log(`- Cache Misses: ${cacheMisses}`);
      console.log(`- Cache Bypasses: ${cacheBypasses}`);
      
      if (cacheHits + cacheMisses > 0) {
        const hitRate = (cacheHits / (cacheHits + cacheMisses) * 100).toFixed(1);
        console.log(`- Cache Hit Rate: ${hitRate}%`);
      }
      
      if (cacheBypasses > 0) {
        console.log(`⚠️  Cache bypasses detected - check feature flag configuration`);
      }
    }
  } catch (e) {
    console.log('⚠️  Could not fetch Prometheus metrics');
  }
} 