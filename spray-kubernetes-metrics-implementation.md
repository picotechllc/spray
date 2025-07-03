# Spray Kubernetes Distributed Metrics: Implementation Plan

## Overview

This document outlines a Kubernetes-focused implementation for distributed metrics aggregation in Spray. The architecture is designed to be extensible for other discovery methods in the future, but prioritizes Kubernetes headless service discovery for initial development.

## Core Architecture

### Discovery Interface Design

Start with a clean interface that can support multiple discovery methods:

```go
// discovery.go
package main

import (
    "context"
    "time"
)

// DiscoveryMethod defines the interface for peer discovery
type DiscoveryMethod interface {
    // Discover returns a list of healthy Spray instances
    Discover(ctx context.Context) ([]PeerInfo, error)
    
    // Watch returns a channel that emits discovery events
    Watch(ctx context.Context) (<-chan DiscoveryEvent, error)
    
    // Close cleans up resources
    Close() error
    
    // Name returns the discovery method name for logging
    Name() string
}

type PeerInfo struct {
    ID          string            `json:"id"`
    Address     string            `json:"address"`     // http://pod-ip:port
    MetricsURL  string            `json:"metrics_url"` // http://pod-ip:port/metrics
    Labels      map[string]string `json:"labels"`
    LastSeen    time.Time         `json:"last_seen"`
    Healthy     bool              `json:"healthy"`
}

type DiscoveryEvent struct {
    Type string    `json:"type"` // "added", "updated", "removed"
    Peer PeerInfo  `json:"peer"`
}

// Factory function for creating discovery methods
func NewDiscoveryMethod(config *DistributedMetricsConfig) (DiscoveryMethod, error) {
    switch config.DiscoveryMethod {
    case "kubernetes":
        return NewKubernetesDiscovery(config.Kubernetes)
    case "memberlist":
        return nil, errors.New("memberlist discovery not implemented yet")
    case "http":
        return nil, errors.New("http discovery not implemented yet")
    default:
        return nil, fmt.Errorf("unknown discovery method: %s", config.DiscoveryMethod)
    }
}
```

## Kubernetes Implementation

### Configuration Structure

```go
// config.go - Add to existing config structure
type DistributedMetricsConfig struct {
    Enabled         bool                     `yaml:"enabled" envconfig:"default=false"`
    DiscoveryMethod string                   `yaml:"discovery_method" envconfig:"default=kubernetes"`
    NodeName        string                   `yaml:"node_name" envconfig:""`
    ScrapeInterval  time.Duration            `yaml:"scrape_interval" envconfig:"default=15s"`
    DashboardPort   int                      `yaml:"dashboard_port" envconfig:"default=8081"`
    
    // Kubernetes-specific config
    Kubernetes      KubernetesDiscoveryConfig `yaml:"kubernetes"`
    
    // Placeholder for future methods
    Memberlist      interface{} `yaml:"memberlist,omitempty"`
    HTTP            interface{} `yaml:"http,omitempty"`
}

type KubernetesDiscoveryConfig struct {
    Namespace       string        `yaml:"namespace" envconfig:""`
    ServiceName     string        `yaml:"service_name" envconfig:"default=spray-headless"`
    Port            int           `yaml:"port" envconfig:"default=8080"`
    ResyncInterval  time.Duration `yaml:"resync_interval" envconfig:"default=30s"`
}
```

### Kubernetes Discovery Implementation

```go
// kubernetes_discovery.go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "time"
    
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/watch"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)

type KubernetesDiscovery struct {
    client      kubernetes.Interface
    config      KubernetesDiscoveryConfig
    namespace   string
    serviceName string
    httpClient  *http.Client
    currentPod  string
}

func NewKubernetesDiscovery(config KubernetesDiscoveryConfig) (*KubernetesDiscovery, error) {
    // Create in-cluster Kubernetes client
    clusterConfig, err := rest.InClusterConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to create k8s in-cluster config: %v", err)
    }
    
    client, err := kubernetes.NewForConfig(clusterConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create k8s client: %v", err)
    }
    
    // Get current namespace from pod environment
    namespace := config.Namespace
    if namespace == "" {
        if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
            namespace = ns
        } else {
            namespace = "default"
        }
    }
    
    // Get current pod name for self-identification
    currentPod := os.Getenv("HOSTNAME") // In k8s, HOSTNAME = pod name
    
    return &KubernetesDiscovery{
        client:      client,
        config:      config,
        namespace:   namespace,
        serviceName: config.ServiceName,
        currentPod:  currentPod,
        httpClient: &http.Client{
            Timeout: 5 * time.Second,
        },
    }, nil
}

func (k *KubernetesDiscovery) Discover(ctx context.Context) ([]PeerInfo, error) {
    // Get endpoints for the headless service
    endpoints, err := k.client.CoreV1().Endpoints(k.namespace).Get(
        ctx, k.serviceName, metav1.GetOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get endpoints for service %s: %v", k.serviceName, err)
    }
    
    var peers []PeerInfo
    for _, subset := range endpoints.Subsets {
        for _, addr := range subset.Addresses {
            // Skip self
            if addr.TargetRef != nil && addr.TargetRef.Name == k.currentPod {
                continue
            }
            
            peerAddr := fmt.Sprintf("http://%s:%d", addr.IP, k.config.Port)
            metricsURL := fmt.Sprintf("%s/metrics", peerAddr)
            
            // Basic health check
            healthy := k.healthCheck(ctx, fmt.Sprintf("%s/livez", peerAddr))
            
            // Extract labels from pod if available
            labels := make(map[string]string)
            if addr.TargetRef != nil {
                labels["pod_name"] = addr.TargetRef.Name
                labels["pod_ip"] = addr.IP
            }
            
            peers = append(peers, PeerInfo{
                ID:         addr.IP, // Use IP as ID for simplicity
                Address:    peerAddr,
                MetricsURL: metricsURL,
                Labels:     labels,
                LastSeen:   time.Now(),
                Healthy:    healthy,
            })
        }
    }
    
    return peers, nil
}

func (k *KubernetesDiscovery) Watch(ctx context.Context) (<-chan DiscoveryEvent, error) {
    eventChan := make(chan DiscoveryEvent, 10)
    
    go func() {
        defer close(eventChan)
        
        // Set up watch on endpoints
        watcher, err := k.client.CoreV1().Endpoints(k.namespace).Watch(ctx, metav1.ListOptions{
            FieldSelector: fmt.Sprintf("metadata.name=%s", k.serviceName),
        })
        if err != nil {
            log.Printf("Failed to watch endpoints: %v", err)
            return
        }
        defer watcher.Stop()
        
        // Also periodically resync
        ticker := time.NewTicker(k.config.ResyncInterval)
        defer ticker.Stop()
        
        for {
            select {
            case event, ok := <-watcher.ResultChan():
                if !ok {
                    log.Printf("Kubernetes watch channel closed, restarting...")
                    return
                }
                
                switch event.Type {
                case watch.Added, watch.Modified:
                    // Full discovery to get updated peer list
                    peers, err := k.Discover(ctx)
                    if err != nil {
                        log.Printf("Failed to discover peers: %v", err)
                        continue
                    }
                    
                    for _, peer := range peers {
                        eventChan <- DiscoveryEvent{
                            Type: "updated",
                            Peer: peer,
                        }
                    }
                    
                case watch.Deleted:
                    // Handle service deletion
                    log.Printf("Headless service %s deleted", k.serviceName)
                }
                
            case <-ticker.C:
                // Periodic resync
                peers, err := k.Discover(ctx)
                if err != nil {
                    log.Printf("Failed to discover peers during resync: %v", err)
                    continue
                }
                
                for _, peer := range peers {
                    eventChan <- DiscoveryEvent{
                        Type: "updated",
                        Peer: peer,
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return eventChan, nil
}

func (k *KubernetesDiscovery) healthCheck(ctx context.Context, url string) bool {
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return false
    }
    
    resp, err := k.httpClient.Do(req)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    
    return resp.StatusCode == http.StatusOK
}

func (k *KubernetesDiscovery) Close() error {
    // No cleanup needed for k8s client
    return nil
}

func (k *KubernetesDiscovery) Name() string {
    return "kubernetes"
}
```

## Metrics Aggregation Engine

```go
// aggregator.go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "sync"
    "time"
    
    "github.com/prometheus/client_golang/prometheus"
    dto "github.com/prometheus/client_model/go"
    "github.com/prometheus/common/expfmt"
)

type MetricsAggregator struct {
    discovery      DiscoveryMethod
    config         *DistributedMetricsConfig
    peers          map[string]PeerInfo
    peersMutex     sync.RWMutex
    
    // Aggregated metrics storage
    aggregated     map[string]*AggregatedMetric
    aggregatedMutex sync.RWMutex
    
    // HTTP client for scraping
    httpClient     *http.Client
    
    // Control channels
    stopChan       chan struct{}
    stopped        bool
}

type AggregatedMetric struct {
    Name          string                 `json:"name"`
    Type          string                 `json:"type"`
    Help          string                 `json:"help"`
    Value         float64                `json:"value"`
    Labels        map[string]string      `json:"labels"`
    InstanceValues map[string]float64    `json:"instance_values"`
    LastUpdate    time.Time              `json:"last_update"`
}

func NewMetricsAggregator(discovery DiscoveryMethod, config *DistributedMetricsConfig) *MetricsAggregator {
    return &MetricsAggregator{
        discovery:      discovery,
        config:         config,
        peers:          make(map[string]PeerInfo),
        aggregated:     make(map[string]*AggregatedMetric),
        httpClient:     &http.Client{Timeout: 10 * time.Second},
        stopChan:       make(chan struct{}),
    }
}

func (a *MetricsAggregator) Start(ctx context.Context) error {
    log.Printf("Starting metrics aggregator with %s discovery", a.discovery.Name())
    
    // Start watching for peer changes
    events, err := a.discovery.Watch(ctx)
    if err != nil {
        return fmt.Errorf("failed to watch for peer changes: %v", err)
    }
    
    // Initial discovery
    peers, err := a.discovery.Discover(ctx)
    if err != nil {
        log.Printf("Initial discovery failed: %v", err)
    } else {
        a.updatePeers(peers)
    }
    
    // Start background goroutines
    go a.watchPeerEvents(ctx, events)
    go a.scrapeLoop(ctx)
    
    return nil
}

func (a *MetricsAggregator) Stop() {
    if !a.stopped {
        close(a.stopChan)
        a.stopped = true
        a.discovery.Close()
    }
}

func (a *MetricsAggregator) watchPeerEvents(ctx context.Context, events <-chan DiscoveryEvent) {
    for {
        select {
        case event, ok := <-events:
            if !ok {
                log.Printf("Peer events channel closed")
                return
            }
            
            log.Printf("Peer event: %s for %s", event.Type, event.Peer.ID)
            
            a.peersMutex.Lock()
            switch event.Type {
            case "added", "updated":
                a.peers[event.Peer.ID] = event.Peer
            case "removed":
                delete(a.peers, event.Peer.ID)
            }
            a.peersMutex.Unlock()
            
        case <-ctx.Done():
            return
        case <-a.stopChan:
            return
        }
    }
}

func (a *MetricsAggregator) scrapeLoop(ctx context.Context) {
    ticker := time.NewTicker(a.config.ScrapeInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            a.scrapeAllPeers(ctx)
            
        case <-ctx.Done():
            return
        case <-a.stopChan:
            return
        }
    }
}

func (a *MetricsAggregator) scrapeAllPeers(ctx context.Context) {
    a.peersMutex.RLock()
    peers := make([]PeerInfo, 0, len(a.peers))
    for _, peer := range a.peers {
        if peer.Healthy {
            peers = append(peers, peer)
        }
    }
    a.peersMutex.RUnlock()
    
    log.Printf("Scraping metrics from %d healthy peers", len(peers))
    
    // Collect metrics from all peers concurrently
    type scrapeResult struct {
        peer    PeerInfo
        metrics map[string]*AggregatedMetric
        err     error
    }
    
    results := make(chan scrapeResult, len(peers))
    
    for _, peer := range peers {
        go func(p PeerInfo) {
            metrics, err := a.scrapePeer(ctx, p)
            results <- scrapeResult{peer: p, metrics: metrics, err: err}
        }(peer)
    }
    
    // Collect results and aggregate
    allMetrics := make(map[string]map[string]*AggregatedMetric) // metricKey -> instanceID -> metric
    
    for i := 0; i < len(peers); i++ {
        result := <-results
        if result.err != nil {
            log.Printf("Failed to scrape peer %s: %v", result.peer.ID, result.err)
            continue
        }
        
        for key, metric := range result.metrics {
            if allMetrics[key] == nil {
                allMetrics[key] = make(map[string]*AggregatedMetric)
            }
            allMetrics[key][result.peer.ID] = metric
        }
    }
    
    // Aggregate metrics
    a.aggregateMetrics(allMetrics)
}

func (a *MetricsAggregator) scrapePeer(ctx context.Context, peer PeerInfo) (map[string]*AggregatedMetric, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", peer.MetricsURL, nil)
    if err != nil {
        return nil, err
    }
    
    resp, err := a.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
    }
    
    return a.parsePrometheusMetrics(resp.Body)
}

func (a *MetricsAggregator) parsePrometheusMetrics(r io.Reader) (map[string]*AggregatedMetric, error) {
    var parser expfmt.TextParser
    metricFamilies, err := parser.TextToMetricFamilies(r)
    if err != nil {
        return nil, err
    }
    
    metrics := make(map[string]*AggregatedMetric)
    
    for familyName, family := range metricFamilies {
        for _, metric := range family.Metric {
            // Create metric key (name + labels)
            labels := make(map[string]string)
            for _, label := range metric.Label {
                labels[*label.Name] = *label.Value
            }
            
            key := a.metricKey(familyName, labels)
            
            var value float64
            switch family.GetType() {
            case dto.MetricType_COUNTER:
                value = metric.Counter.GetValue()
            case dto.MetricType_GAUGE:
                value = metric.Gauge.GetValue()
            case dto.MetricType_HISTOGRAM:
                value = metric.Histogram.GetSampleCount()
            default:
                continue // Skip unsupported types for now
            }
            
            metrics[key] = &AggregatedMetric{
                Name:       familyName,
                Type:       family.GetType().String(),
                Help:       family.GetHelp(),
                Value:      value,
                Labels:     labels,
                LastUpdate: time.Now(),
            }
        }
    }
    
    return metrics, nil
}

func (a *MetricsAggregator) aggregateMetrics(allMetrics map[string]map[string]*AggregatedMetric) {
    a.aggregatedMutex.Lock()
    defer a.aggregatedMutex.Unlock()
    
    // Clear old aggregated metrics
    a.aggregated = make(map[string]*AggregatedMetric)
    
    for metricKey, instanceMetrics := range allMetrics {
        if len(instanceMetrics) == 0 {
            continue
        }
        
        // Get a representative metric for metadata
        var representative *AggregatedMetric
        for _, m := range instanceMetrics {
            representative = m
            break
        }
        
        aggregated := &AggregatedMetric{
            Name:           representative.Name,
            Type:           representative.Type,
            Help:           representative.Help,
            Labels:         representative.Labels,
            InstanceValues: make(map[string]float64),
            LastUpdate:     time.Now(),
        }
        
        // Aggregate based on metric type
        var sum float64
        for instanceID, metric := range instanceMetrics {
            aggregated.InstanceValues[instanceID] = metric.Value
            
            switch representative.Type {
            case "COUNTER", "HISTOGRAM":
                sum += metric.Value
            case "GAUGE":
                // For gauges, we'll show the sum but also provide instance values
                sum += metric.Value
            }
        }
        
        aggregated.Value = sum
        a.aggregated[metricKey] = aggregated
    }
    
    log.Printf("Aggregated %d metrics from %d metric families", len(a.aggregated), len(allMetrics))
}

func (a *MetricsAggregator) metricKey(name string, labels map[string]string) string {
    // Simple key generation - in production, might want something more sophisticated
    key := name
    for k, v := range labels {
        key += fmt.Sprintf(",%s=%s", k, v)
    }
    return key
}

func (a *MetricsAggregator) GetAggregatedMetrics() map[string]*AggregatedMetric {
    a.aggregatedMutex.RLock()
    defer a.aggregatedMutex.RUnlock()
    
    // Return a copy to avoid concurrent access issues
    result := make(map[string]*AggregatedMetric)
    for k, v := range a.aggregated {
        result[k] = v
    }
    return result
}

func (a *MetricsAggregator) GetPeers() []PeerInfo {
    a.peersMutex.RLock()
    defer a.peersMutex.RUnlock()
    
    peers := make([]PeerInfo, 0, len(a.peers))
    for _, peer := range a.peers {
        peers = append(peers, peer)
    }
    return peers
}

func (a *MetricsAggregator) updatePeers(peers []PeerInfo) {
    a.peersMutex.Lock()
    defer a.peersMutex.Unlock()
    
    // Clear existing peers
    a.peers = make(map[string]PeerInfo)
    
    // Add new peers
    for _, peer := range peers {
        a.peers[peer.ID] = peer
    }
    
    log.Printf("Updated peer list: %d peers", len(peers))
}
```

## Kubernetes Deployment Configuration

### Enhanced Spray Deployment

```yaml
# spray-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: spray
  namespace: spray-system
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
      serviceAccountName: spray-metrics
      containers:
      - name: spray
        image: spray:latest
        env:
        # Existing spray config
        - name: BUCKET_NAME
          value: "my-static-content"
        - name: GOOGLE_PROJECT_ID
          value: "my-project"
        - name: PORT
          value: "8080"
        
        # Distributed metrics config
        - name: SPRAY_DISTRIBUTED_METRICS_ENABLED
          value: "true"
        - name: SPRAY_DISTRIBUTED_METRICS_DISCOVERY_METHOD
          value: "kubernetes"
        - name: SPRAY_DISTRIBUTED_METRICS_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: SPRAY_DISTRIBUTED_METRICS_KUBERNETES_SERVICE_NAME
          value: "spray-headless"
        - name: SPRAY_DISTRIBUTED_METRICS_DASHBOARD_PORT
          value: "8081"
        - name: SPRAY_DISTRIBUTED_METRICS_SCRAPE_INTERVAL
          value: "15s"
        
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        - containerPort: 8081
          name: dashboard
          protocol: TCP
        
        livenessProbe:
          httpGet:
            path: /livez
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"

---
# Headless service for discovery
apiVersion: v1
kind: Service
metadata:
  name: spray-headless
  namespace: spray-system
spec:
  clusterIP: None
  selector:
    app: spray
  ports:
  - port: 8080
    targetPort: 8080
    name: metrics

---
# Regular service for external access
apiVersion: v1
kind: Service
metadata:
  name: spray
  namespace: spray-system
spec:
  selector:
    app: spray
  ports:
  - port: 80
    targetPort: 8080
    name: http
  type: LoadBalancer

---
# Dashboard service 
apiVersion: v1
kind: Service
metadata:
  name: spray-dashboard
  namespace: spray-system
spec:
  selector:
    app: spray
  ports:
  - port: 8081
    targetPort: 8081
    name: dashboard
  type: LoadBalancer

---
# Service account for Kubernetes API access
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spray-metrics
  namespace: spray-system

---
# ClusterRole for reading endpoints
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: spray-metrics-reader
rules:
- apiGroups: [""]
  resources: ["endpoints", "pods", "services"]
  verbs: ["get", "list", "watch"]

---
# Bind the service account to the role
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: spray-metrics-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: spray-metrics-reader
subjects:
- kind: ServiceAccount
  name: spray-metrics
  namespace: spray-system
```

## Integration with Existing Spray Code

### Modified server.go

```go
// Add to server.go
func createServer(ctx context.Context, cfg *config, logClient LoggingClient) (*http.Server, error) {
    // ... existing code ...
    
    // Initialize distributed metrics if enabled
    if cfg.distributedMetrics != nil && cfg.distributedMetrics.Enabled {
        err := initializeDistributedMetrics(ctx, cfg)
        if err != nil {
            log.Printf("Failed to initialize distributed metrics: %v", err)
            // Don't fail server startup, just disable the feature
        }
    }
    
    // ... rest of existing code ...
}

var globalAggregator *MetricsAggregator

func initializeDistributedMetrics(ctx context.Context, cfg *config) error {
    discovery, err := NewDiscoveryMethod(cfg.distributedMetrics)
    if err != nil {
        return fmt.Errorf("failed to create discovery method: %v", err)
    }
    
    globalAggregator = NewMetricsAggregator(discovery, cfg.distributedMetrics)
    
    err = globalAggregator.Start(ctx)
    if err != nil {
        return fmt.Errorf("failed to start metrics aggregator: %v", err)
    }
    
    // Add dashboard routes
    setupDistributedMetricsRoutes()
    
    log.Printf("Distributed metrics initialized with %s discovery", discovery.Name())
    return nil
}

func setupDistributedMetricsRoutes() {
    http.HandleFunc("/dashboard/", handleDashboard)
    http.HandleFunc("/api/aggregated-metrics", handleAggregatedMetrics)
    http.HandleFunc("/api/peers", handlePeers)
    http.HandleFunc("/ws/metrics", handleMetricsWebSocket)
}

func handleAggregatedMetrics(w http.ResponseWriter, r *http.Request) {
    if globalAggregator == nil {
        http.Error(w, "Distributed metrics not enabled", http.StatusNotFound)
        return
    }
    
    metrics := globalAggregator.GetAggregatedMetrics()
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(metrics)
}

func handlePeers(w http.ResponseWriter, r *http.Request) {
    if globalAggregator == nil {
        http.Error(w, "Distributed metrics not enabled", http.StatusNotFound)
        return
    }
    
    peers := globalAggregator.GetPeers()
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(peers)
}
```

## Configuration Example

Simple YAML configuration for Kubernetes:

```yaml
# spray-config.yaml
bucket_name: "my-static-content"
google_project_id: "my-project"
port: "8080"

distributed_metrics:
  enabled: true
  discovery_method: "kubernetes"
  scrape_interval: "15s"
  dashboard_port: 8081
  
  kubernetes:
    service_name: "spray-headless"
    port: 8080
    resync_interval: "30s"
```

## Implementation Phases

### Phase 1: Core Framework (Week 1)
- [ ] Implement discovery interface and Kubernetes discovery
- [ ] Basic metrics aggregation engine
- [ ] Configuration integration
- [ ] Unit tests for discovery and aggregation

### Phase 2: Integration (Week 2)
- [ ] Integrate with existing Spray server
- [ ] Add API endpoints for aggregated metrics
- [ ] Kubernetes RBAC setup
- [ ] Integration tests with real k8s cluster

### Phase 3: Dashboard (Week 3)
- [ ] Embedded HTML dashboard
- [ ] Real-time WebSocket updates
- [ ] Basic charts with Chart.js
- [ ] Mobile-responsive design

### Phase 4: Production Ready (Week 4)
- [ ] Error handling and recovery
- [ ] Performance optimization
- [ ] Comprehensive documentation
- [ ] End-to-end testing

## Testing Strategy

### Unit Tests
```bash
go test ./... -v -race
```

### Integration Tests with Kind
```bash
# Create test cluster
kind create cluster --name spray-test

# Deploy test Spray instances
kubectl apply -f test/k8s/

# Run integration tests
go test ./test/integration -v -tags=integration
```

### Manual Testing
```bash
# Deploy to cluster
kubectl apply -f spray-deployment.yaml

# Port forward to dashboard
kubectl port-forward svc/spray-dashboard 8081:8081

# Open dashboard
open http://localhost:8081/dashboard/
```

This Kubernetes-focused approach provides a solid foundation that can be extended to support other discovery methods later, while delivering immediate value in Kubernetes environments where Spray is actually being used.