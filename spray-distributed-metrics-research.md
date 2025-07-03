# Spray Distributed Metrics Aggregation: Research & Implementation Strategy

## Executive Summary

This document explores implementing distributed metrics aggregation for Spray instances, enabling them to automatically discover each other and provide a unified dashboard without requiring external infrastructure like Prometheus or Grafana. The proposed solution leverages existing Spray capabilities while adding lightweight service discovery and aggregation features.

## Current State Analysis

### Spray's Existing Metrics Infrastructure

Spray already provides excellent observability foundations:

- **Comprehensive Prometheus metrics** (`metrics.go`):
  - Request metrics: `gcs_server_requests_total`, `gcs_server_request_duration_seconds`
  - Performance metrics: `gcs_server_bytes_transferred_total`, `gcs_server_active_requests`
  - Error tracking: `gcs_server_errors_total`
  - Cache metrics: `gcs_server_cache_total`
  - Storage operations: `gcs_server_storage_operation_duration_seconds`

- **HTTP endpoints** (`server.go`):
  - `/metrics` - Prometheus metrics endpoint
  - `/readyz` and `/livez` - Health check endpoints
  - Configurable HTTP server with robust error handling

- **Configuration system** (`config.go`):
  - Environment variable support
  - YAML configuration
  - Port and address configuration

## Proposed Architecture

### Core Components

1. **Service Discovery Layer**
   - Automatic peer discovery and membership management
   - Health monitoring and failure detection
   - Support for multiple discovery mechanisms

2. **Metrics Aggregation Engine**
   - Scrape metrics from discovered peers
   - Aggregate and merge metrics data
   - Handle counter aggregation and gauge reconciliation

3. **Embedded Dashboard**
   - Lightweight web UI for metrics visualization
   - Real-time updates and basic charting
   - Responsive design for mobile and desktop

4. **Coordination Protocol**
   - Leader election for aggregation coordination
   - Conflict resolution for overlapping responsibilities
   - Graceful handling of network partitions

## Implementation Approaches

### Option 1: Gossip Protocol with HashiCorp Memberlist

**Benefits:**
- Battle-tested in production (Consul, Serf, Nomad)
- Excellent failure detection and network partition handling
- Minimal configuration required
- Works across network boundaries

**Implementation Strategy:**
```go
// Add to config.go
type DistributedMetricsConfig struct {
    Enabled         bool     `yaml:"enabled" envconfig:"default=false"`
    NodeName        string   `yaml:"node_name" envconfig:""`
    BindAddr        string   `yaml:"bind_addr" envconfig:"default=0.0.0.0:7946"`
    JoinAddresses   []string `yaml:"join_addresses" envconfig:""`
    DashboardPort   int      `yaml:"dashboard_port" envconfig:"default=8081"`
    ScrapeInterval  string   `yaml:"scrape_interval" envconfig:"default=15s"`
}

// New aggregation service
type MetricsAggregator struct {
    memberlist   *memberlist.Memberlist
    peers        map[string]*PeerInfo
    aggregated   map[string]*AggregatedMetric
    dashboard    *DashboardServer
}
```

**Discovery Process:**
1. Start memberlist with configured bind address
2. Join cluster using seed addresses or multicast discovery
3. Maintain peer registry with health status
4. Coordinate metrics collection through gossip events

### Option 2: Kubernetes-Native Service Discovery

**Benefits:**
- Native integration with Kubernetes environments
- Automatic pod discovery through headless services
- Built-in health checks and DNS resolution
- No additional network ports required

**Implementation Strategy:**
```go
// Kubernetes discovery using headless service
type KubernetesDiscovery struct {
    namespace      string
    serviceName    string
    client         kubernetes.Interface
    endpoints      []string
}

func (k *KubernetesDiscovery) DiscoverPeers() ([]PeerInfo, error) {
    // Query headless service for all pod IPs
    endpoints, err := k.client.CoreV1().Endpoints(k.namespace).Get(
        context.TODO(), k.serviceName, metav1.GetOptions{})
    
    // Convert to peer list with health checks
    return k.endpointsToPeers(endpoints)
}
```

**Example headless service configuration:**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: spray-headless
spec:
  clusterIP: None
  selector:
    app: spray
  ports:
  - port: 8080
    name: metrics
```

### Option 3: Simple HTTP-Based Discovery

**Benefits:**
- Minimal dependencies
- Easy to understand and debug
- Works in any environment
- Lightweight implementation

**Implementation Strategy:**
```go
// HTTP-based peer discovery
type HTTPDiscovery struct {
    peers          []string
    healthCheck    string
    client         *http.Client
}

func (h *HTTPDiscovery) DiscoverPeers() ([]PeerInfo, error) {
    var healthy []PeerInfo
    for _, peer := range h.peers {
        if h.isHealthy(peer) {
            healthy = append(healthy, PeerInfo{
                Address: peer,
                Status:  "healthy",
            })
        }
    }
    return healthy, nil
}
```

## Metrics Aggregation Strategy

### Data Structure Design

```go
type AggregatedMetric struct {
    Name        string            `json:"name"`
    Type        string            `json:"type"` // counter, gauge, histogram
    Value       float64           `json:"value"`
    Labels      map[string]string `json:"labels"`
    Instances   map[string]float64 `json:"instances"` // per-instance values
    LastUpdate  time.Time         `json:"last_update"`
}

type MetricsSnapshot struct {
    Timestamp   time.Time                    `json:"timestamp"`
    Instance    string                       `json:"instance"`
    Metrics     map[string]AggregatedMetric  `json:"metrics"`
}
```

### Aggregation Rules

**Counters:**
- Sum across all instances
- Track per-instance contributions
- Handle counter resets gracefully

**Gauges:**
- Provide multiple views: sum, average, min, max
- Show per-instance values
- Handle missing data points

**Histograms:**
- Merge buckets across instances
- Aggregate quantile calculations
- Maintain statistical accuracy

### Collection Process

```go
func (a *MetricsAggregator) collectMetrics() {
    for _, peer := range a.getHealthyPeers() {
        go func(p PeerInfo) {
            metrics, err := a.scrapeMetrics(p.Address + "/metrics")
            if err != nil {
                log.Printf("Failed to scrape %s: %v", p.Address, err)
                return
            }
            a.mergeMetrics(p.Instance, metrics)
        }(peer)
    }
}

func (a *MetricsAggregator) scrapeMetrics(url string) (map[string]interface{}, error) {
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    // Parse Prometheus format
    return prometheus.ParseResponse(resp.Body)
}
```

## Dashboard Implementation

### Embedded Web UI

Create a lightweight, responsive dashboard using modern web technologies embedded in the Go binary:

**Technology Stack:**
- HTML5 with embedded CSS and JavaScript
- Chart.js for visualizations
- WebSocket for real-time updates
- Responsive design with CSS Grid

**Key Features:**
- Real-time metrics display
- Instance health overview
- Historical trending (memory-based)
- Drill-down capabilities

**Dashboard Routes:**
```go
// Add to server.go
func (s *gcsServer) setupDashboardRoutes() {
    if s.distributedMetrics != nil && s.distributedMetrics.Enabled {
        http.HandleFunc("/dashboard/", s.handleDashboard)
        http.HandleFunc("/api/metrics", s.handleAggregatedMetrics)
        http.HandleFunc("/api/instances", s.handleInstanceStatus)
        http.HandleFunc("/ws/metrics", s.handleMetricsWebSocket)
    }
}
```

### Sample Dashboard Layout

```html
<!DOCTYPE html>
<html>
<head>
    <title>Spray Cluster Dashboard</title>
    <style>
        .dashboard-grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
            padding: 20px;
        }
        .metric-card {
            background: #f8f9fa;
            border-radius: 8px;
            padding: 16px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
    </style>
</head>
<body>
    <div class="dashboard-grid">
        <div class="metric-card">
            <h3>Request Rate</h3>
            <canvas id="requestRate"></canvas>
        </div>
        <div class="metric-card">
            <h3>Error Rate</h3>
            <canvas id="errorRate"></canvas>
        </div>
        <div class="metric-card">
            <h3>Response Time</h3>
            <canvas id="responseTime"></canvas>
        </div>
        <div class="metric-card">
            <h3>Instance Health</h3>
            <div id="instanceHealth"></div>
        </div>
    </div>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <script>/* WebSocket and charting logic */</script>
</body>
</html>
```

## Configuration Integration

### Enhanced Configuration

```yaml
# Enhanced spray configuration
distributed_metrics:
  enabled: true
  node_name: "spray-node-1"
  discovery_method: "memberlist"  # memberlist, kubernetes, http
  
  # Memberlist configuration
  memberlist:
    bind_addr: "0.0.0.0:7946"
    join_addresses:
      - "spray-node-2:7946"
      - "spray-node-3:7946"
  
  # Kubernetes configuration
  kubernetes:
    namespace: "default"
    service_name: "spray-headless"
  
  # HTTP discovery configuration  
  http:
    peers:
      - "http://spray-node-2:8080"
      - "http://spray-node-3:8080"
  
  # Aggregation settings
  scrape_interval: "15s"
  retention_period: "1h"
  dashboard_port: 8081
  
  # Dashboard configuration
  dashboard:
    enabled: true
    refresh_interval: "5s"
    max_history_points: 100
```

### Environment Variable Support

```bash
# Enable distributed metrics
SPRAY_DISTRIBUTED_METRICS_ENABLED=true
SPRAY_DISTRIBUTED_METRICS_NODE_NAME=spray-prod-1
SPRAY_DISTRIBUTED_METRICS_DISCOVERY_METHOD=kubernetes
SPRAY_DISTRIBUTED_METRICS_KUBERNETES_NAMESPACE=production
SPRAY_DISTRIBUTED_METRICS_DASHBOARD_PORT=8081
```

## Deployment Scenarios

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: spray
spec:
  replicas: 3
  selector:
    matchLabels:
      app: spray
  template:
    metadata:
      labels:
        app: spray
    spec:
      containers:
      - name: spray
        image: spray:latest
        env:
        - name: SPRAY_DISTRIBUTED_METRICS_ENABLED
          value: "true"
        - name: SPRAY_DISTRIBUTED_METRICS_DISCOVERY_METHOD
          value: "kubernetes"
        - name: SPRAY_DISTRIBUTED_METRICS_KUBERNETES_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 8081
          name: dashboard
---
apiVersion: v1
kind: Service
metadata:
  name: spray-headless
spec:
  clusterIP: None
  selector:
    app: spray
  ports:
  - port: 8080
    name: metrics
---
apiVersion: v1
kind: Service
metadata:
  name: spray-dashboard
spec:
  selector:
    app: spray
  ports:
  - port: 8081
    targetPort: 8081
    name: dashboard
  type: LoadBalancer
```

### Docker Compose Deployment

```yaml
version: '3.8'
services:
  spray-1:
    image: spray:latest
    environment:
      - SPRAY_DISTRIBUTED_METRICS_ENABLED=true
      - SPRAY_DISTRIBUTED_METRICS_DISCOVERY_METHOD=memberlist
      - SPRAY_DISTRIBUTED_METRICS_NODE_NAME=spray-1
      - SPRAY_DISTRIBUTED_METRICS_MEMBERLIST_BIND_ADDR=0.0.0.0:7946
      - SPRAY_DISTRIBUTED_METRICS_MEMBERLIST_JOIN_ADDRESSES=spray-2:7946,spray-3:7946
    ports:
      - "8080:8080"
      - "8081:8081"  # Dashboard
      - "7946:7946"  # Memberlist
  
  spray-2:
    image: spray:latest
    environment:
      - SPRAY_DISTRIBUTED_METRICS_ENABLED=true
      - SPRAY_DISTRIBUTED_METRICS_DISCOVERY_METHOD=memberlist
      - SPRAY_DISTRIBUTED_METRICS_NODE_NAME=spray-2
      - SPRAY_DISTRIBUTED_METRICS_MEMBERLIST_BIND_ADDR=0.0.0.0:7946
      - SPRAY_DISTRIBUTED_METRICS_MEMBERLIST_JOIN_ADDRESSES=spray-1:7946,spray-3:7946
    ports:
      - "8082:8080"
      - "8083:8081"
      - "7947:7946"
  
  spray-3:
    image: spray:latest
    environment:
      - SPRAY_DISTRIBUTED_METRICS_ENABLED=true
      - SPRAY_DISTRIBUTED_METRICS_DISCOVERY_METHOD=memberlist
      - SPRAY_DISTRIBUTED_METRICS_NODE_NAME=spray-3
      - SPRAY_DISTRIBUTED_METRICS_MEMBERLIST_BIND_ADDR=0.0.0.0:7946
      - SPRAY_DISTRIBUTED_METRICS_MEMBERLIST_JOIN_ADDRESSES=spray-1:7946,spray-2:7946
    ports:
      - "8084:8080"
      - "8085:8081"
      - "7948:7946"
```

## Implementation Phases

### Phase 1: Core Infrastructure (Weeks 1-2)
- [ ] Add distributed metrics configuration structure
- [ ] Implement basic service discovery (HTTP-based)
- [ ] Create metrics aggregation engine
- [ ] Add health check integration

### Phase 2: Advanced Discovery (Weeks 3-4)
- [ ] Implement memberlist-based discovery
- [ ] Add Kubernetes service discovery
- [ ] Create peer management and failure detection
- [ ] Add configuration validation

### Phase 3: Dashboard & UI (Weeks 5-6)
- [ ] Create embedded dashboard server
- [ ] Implement WebSocket real-time updates
- [ ] Design responsive UI with Chart.js
- [ ] Add metric drill-down capabilities

### Phase 4: Production Features (Weeks 7-8)
- [ ] Add leader election for coordination
- [ ] Implement graceful shutdown and failover
- [ ] Create comprehensive documentation
- [ ] Add integration tests and benchmarks

## Testing Strategy

### Unit Tests
- Service discovery mechanisms
- Metrics aggregation logic
- Configuration parsing and validation
- Dashboard API endpoints

### Integration Tests
- Multi-instance deployment scenarios
- Network partition and recovery
- Failover and leader election
- Cross-platform compatibility

### Performance Tests
- Memory usage with large instance counts
- Aggregation latency under load
- Dashboard responsiveness
- Network bandwidth utilization

## Security Considerations

### Authentication & Authorization
- Optional TLS for peer communication
- Dashboard access controls
- API key-based peer authentication
- Rate limiting for scraping endpoints

### Network Security
- Configurable bind addresses
- Firewall-friendly port allocation
- Optional VPN/private network support
- Audit logging for administrative actions

## Migration Path

### Backward Compatibility
- Feature is opt-in via configuration
- No changes to existing metrics endpoints
- Maintains current operational patterns
- Graceful degradation when disabled

### Gradual Rollout
1. Deploy with distributed metrics disabled
2. Enable discovery on subset of instances
3. Activate aggregation for non-critical metrics
4. Full deployment with complete feature set

## Conclusion

This distributed metrics aggregation system would provide Spray users with powerful observability capabilities without requiring external infrastructure. The modular design allows for different deployment scenarios while maintaining the simplicity and reliability that Spray is known for.

The proposed implementation leverages proven patterns from the container orchestration and service mesh ecosystems while remaining lightweight and focused on Spray's core use case as a static file server with excellent observability features.

Key benefits:
- **Zero external dependencies** for basic observability
- **Flexible deployment options** across different environments
- **Minimal operational overhead** with automatic discovery
- **Rich visualization** through embedded dashboard
- **Production-ready** with proper error handling and monitoring

This solution would significantly enhance Spray's value proposition for users running multiple instances across different environments, providing enterprise-grade observability in a simple, self-contained package.