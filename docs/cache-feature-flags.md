# HTTP Cache Feature Flags

This document describes the comprehensive feature flag system for HTTP cache functionality in spray v0.3.0.

## 🎯 Overview

The cache feature flag system allows you to:

- **Safely deploy** cache functionality with a master kill switch
- **Gradually roll out** to a percentage of users
- **A/B test** cache performance impact
- **Target specific paths** or user agents
- **Fine-tune individual features** (ETag, Last-Modified, Cache-Control)
- **Quickly disable** if issues arise

## 📁 Configuration File

Cache settings are configured via `.spray/headers.toml` in your GCS bucket:

```toml
[cache]
enabled = true  # Master switch

[cache.etag]
enabled = true

[cache.last_modified] 
enabled = true

[cache.cache_control]
enabled = true

[cache.policies]
short_max_age = 300      # 5 minutes
medium_max_age = 86400   # 1 day
long_max_age = 31536000  # 1 year

[cache.rollout]
enabled = false
percentage = 100
path_prefixes = []
exclude_prefixes = []
user_agent_rules = []
```

## 🔧 Configuration Options

### Master Switch

```toml
[cache]
enabled = false  # Disables ALL cache functionality
```

**When disabled:**
- No cache headers are set
- No conditional request processing
- All requests tracked as "bypass" in metrics
- Zero performance impact

### Individual Features

```toml
[cache.etag]
enabled = true  # Controls ETag header generation and If-None-Match processing

[cache.last_modified]
enabled = true  # Controls Last-Modified header and If-Modified-Since processing

[cache.cache_control]
enabled = true  # Controls Cache-Control header generation
```

**Granular Control:**
- Disable ETag but keep Last-Modified
- Disable Cache-Control but keep validation headers
- Mix and match based on your needs

### Cache Policies

```toml
[cache.policies]
short_max_age = 300      # HTML files (5 minutes)
medium_max_age = 86400   # CSS, JS, images (1 day) 
long_max_age = 31536000  # Versioned assets (1 year)
```

**Automatic Classification:**
- **Short**: `text/html` content
- **Medium**: `.css`, `.js`, `.png`, `.jpg`, etc.
- **Long**: Files with `.min.`, `-v`, `.hash.` in path

### Rollout Configuration

```toml
[cache.rollout]
enabled = true
percentage = 25          # Only 25% of users get cache
path_prefixes = ["assets/", "static/"]
exclude_prefixes = ["api/", "dynamic/"]
user_agent_rules = [".*Chrome.*", ".*Firefox.*"]
```

## 📊 Rollout Strategies

### 1. Percentage Rollout

```toml
[cache.rollout]
enabled = true
percentage = 10  # Start with 10% of users
```

**How it works:**
- Uses hash of `IP + User-Agent` for consistency
- Same user always gets same treatment
- Gradually increase percentage: 10% → 25% → 50% → 100%

### 2. Path-Based Rollout

```toml
[cache.rollout]
enabled = true
percentage = 100
path_prefixes = ["assets/", "css/", "js/"]  # Only static assets
exclude_prefixes = ["api/"]                 # Never cache API calls
```

**Use cases:**
- Start with static assets only
- Exclude dynamic content
- Test specific file types

### 3. User-Agent Rollout

```toml
[cache.rollout]
enabled = true
percentage = 100
user_agent_rules = [
    ".*Chrome.*",    # Chrome browsers only
    ".*k6.*"         # Include load testing tools
]
```

**Use cases:**
- Test with specific browsers first
- Include/exclude bots and crawlers
- Gradual browser compatibility testing

### 4. Combined Rollout

```toml
[cache.rollout]
enabled = true
percentage = 50                              # 50% of users
path_prefixes = ["assets/"]                 # Only assets
exclude_prefixes = ["assets/dynamic/"]      # Except dynamic assets
user_agent_rules = [".*Chrome.*", ".*Firefox.*"]  # Modern browsers only
```

**Conditions are AND-ed:**
- Must be in the 50% user group
- Must request an asset path
- Must not be dynamic asset
- Must be Chrome or Firefox

## 🚀 Deployment Strategies

### Strategy 1: Conservative Rollout

```toml
# Week 1: Disabled (collect baseline metrics)
[cache]
enabled = false

# Week 2: 10% of static assets for Chrome users
[cache]
enabled = true
[cache.rollout]
enabled = true
percentage = 10
path_prefixes = ["assets/", "css/", "js/"]
user_agent_rules = [".*Chrome.*"]

# Week 3: 25% of static assets for modern browsers  
[cache.rollout]
percentage = 25
user_agent_rules = [".*Chrome.*", ".*Firefox.*", ".*Safari.*"]

# Week 4: 50% of all content
[cache.rollout]
percentage = 50
path_prefixes = []

# Week 5: 100% (full rollout)
[cache.rollout]
enabled = false  # Disable rollout restrictions
```

### Strategy 2: Feature-by-Feature

```toml
# Phase 1: ETag only
[cache]
enabled = true
[cache.etag]
enabled = true
[cache.last_modified]
enabled = false
[cache.cache_control]
enabled = false

# Phase 2: Add Last-Modified
[cache.last_modified]
enabled = true

# Phase 3: Add Cache-Control
[cache.cache_control]
enabled = true
```

### Strategy 3: A/B Testing

```toml
# Group A: 50% with cache enabled
[cache]
enabled = true
[cache.rollout]
enabled = true
percentage = 50

# Group B: 50% with cache disabled (automatic)
# Monitor metrics to compare performance
```

## 📈 Monitoring & Metrics

### Prometheus Metrics

```promql
# Cache bypass rate (feature flag effectiveness)
rate(gcs_server_cache_total{status="bypass"}[5m])

# Cache hit rate (when enabled)
rate(gcs_server_cache_total{status="hit"}[5m]) / 
(rate(gcs_server_cache_total{status="hit"}[5m]) + rate(gcs_server_cache_total{status="miss"}[5m]))

# Feature flag coverage
rate(gcs_server_cache_headers_total[5m]) / rate(gcs_server_requests_total[5m])
```

### k6 Testing

```bash
# Test feature flag effectiveness
.\scripts\run-k6-tests.ps1 -TestType comprehensive

# Monitor bypass rate in output:
# ✓ cache_bypass_rate............: 25.0%   (expected for 75% rollout)
# ✓ feature_flag_effectiveness...: 75.0%   (cache headers present)
```

### Grafana Alerts

```promql
# Alert when cache bypass rate is unexpectedly high
rate(gcs_server_cache_total{status="bypass"}[5m]) / rate(gcs_server_cache_total[5m]) > 0.9

# Alert when feature flags aren't working
rate(gcs_server_cache_headers_total[5m]) == 0 and on() gcs_server_cache_enabled == 1
```

## 🛠️ Configuration Examples

### Example 1: Development Environment

```toml
# Safe defaults for development
[cache]
enabled = false  # Disabled by default

[cache.policies]
short_max_age = 60      # 1 minute for quick testing
medium_max_age = 300    # 5 minutes
long_max_age = 3600     # 1 hour
```

### Example 2: Staging Environment

```toml
# Test cache with real traffic patterns
[cache]
enabled = true

[cache.rollout]
enabled = true
percentage = 100        # Full cache for staging
path_prefixes = []      # All paths
exclude_prefixes = ["api/", "admin/"]  # Exclude dynamic content
```

### Example 3: Production Rollout

```toml
# Conservative production rollout
[cache]
enabled = true

[cache.rollout]
enabled = true
percentage = 25         # Start with 25%
path_prefixes = [
    "assets/",
    "css/", 
    "js/",
    "images/",
    "fonts/"
]
exclude_prefixes = [
    "api/",
    "admin/",
    "dynamic/"
]
user_agent_rules = [
    ".*Chrome.*",
    ".*Firefox.*",
    ".*Safari.*",
    ".*Edge.*"
]
```

### Example 4: Emergency Disable

```toml
# Quick disable if issues arise
[cache]
enabled = false  # Master kill switch

# All other settings ignored when enabled = false
```

## 🔍 Troubleshooting

### Cache Not Working

1. **Check master switch:**
   ```toml
   [cache]
   enabled = true  # Must be true
   ```

2. **Check rollout settings:**
   ```bash
   # Test if your request matches rollout rules
   curl -H "User-Agent: Mozilla/5.0 Chrome/91.0" http://localhost:8080/assets/style.css -v
   ```

3. **Check Prometheus metrics:**
   ```bash
   curl http://localhost:8080/metrics | grep cache_total
   ```

### High Bypass Rate

```promql
# Check which paths are being bypassed
rate(gcs_server_cache_total{status="bypass"}[5m]) by (path)

# Check rollout percentage effectiveness
rate(gcs_server_cache_total{status="bypass"}[5m]) / rate(gcs_server_cache_total[5m])
```

### Configuration Not Loading

1. **Check file location:** `.spray/headers.toml` in GCS bucket
2. **Check TOML syntax:** Use a TOML validator
3. **Check permissions:** Ensure spray can read the config file
4. **Check logs:** Look for configuration parsing errors

## 🚦 Best Practices

### 1. Start Conservative
- Begin with `enabled = false`
- Use small percentages (10-25%)
- Start with static assets only

### 2. Monitor Closely
- Set up Prometheus alerts
- Run k6 tests regularly
- Watch error rates and response times

### 3. Gradual Rollout
- Increase percentage weekly
- Add path prefixes incrementally
- Test each browser separately

### 4. Have a Rollback Plan
- Keep master switch easily accessible
- Document rollback procedure
- Monitor for 24-48 hours after changes

### 5. Test Thoroughly
- Use k6 feature flag tests
- Validate with different User-Agents
- Test edge cases (empty paths, special characters)

## 📋 Rollout Checklist

- [ ] **Baseline metrics** collected (cache disabled)
- [ ] **Configuration file** created and validated
- [ ] **Monitoring** set up (Prometheus + Grafana)
- [ ] **Alerts** configured for bypass rate and errors
- [ ] **k6 tests** passing with feature flags
- [ ] **Rollback plan** documented
- [ ] **Team notification** of rollout schedule
- [ ] **Gradual increase** planned (10% → 25% → 50% → 100%)
- [ ] **Success criteria** defined (hit rate, response time, error rate)
- [ ] **Post-rollout review** scheduled

This feature flag system gives you complete control over your HTTP cache rollout, ensuring a safe and monitored deployment to production! 🎉 