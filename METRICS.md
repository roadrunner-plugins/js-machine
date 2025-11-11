# JavaScript Plugin Metrics Guide

This guide covers all Prometheus metrics exposed by the JavaScript plugin for monitoring and observability.

## Table of Contents

1. [Available Metrics](#available-metrics)
2. [Configuration](#configuration)
3. [Accessing Metrics](#accessing-metrics)
4. [PromQL Queries](#promql-queries)
5. [Grafana Dashboard](#grafana-dashboard)
6. [Alerting Rules](#alerting-rules)

---

## Available Metrics

### Counter Metrics

#### `js_executions_total`

Total number of JavaScript executions.

**Type**: Counter  
**Labels**:

- `status`: Execution status (`success`, `error`, `timeout`)

**Example values**:

```
js_executions_total{status="success"} 1523
js_executions_total{status="error"} 42
js_executions_total{status="timeout"} 7
```

**Use cases**:

- Track total execution volume
- Calculate success/error rates
- Monitor timeout frequency

---

### Histogram Metrics

#### `js_execution_duration_seconds`

JavaScript execution duration in seconds.

**Type**: Histogram  
**Labels**:

- `status`: Execution status (`success`, `error`, `timeout`)

**Buckets**: `[.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30]`

**Example values**:

```
js_execution_duration_seconds_bucket{status="success",le="0.01"} 856
js_execution_duration_seconds_bucket{status="success",le="0.1"} 1421
js_execution_duration_seconds_sum{status="success"} 142.5
js_execution_duration_seconds_count{status="success"} 1523
```

**Use cases**:

- Calculate percentile latencies (p50, p95, p99)
- Monitor execution performance trends
- Identify slow scripts

---

#### `js_code_size_bytes`

Size of JavaScript code in bytes.

**Type**: Histogram  
**Labels**: None

**Buckets**: `[100, 500, 1000, 5000, 10000, 50000, 100000, 500000]`

**Example values**:

```
js_code_size_bytes_bucket{le="1000"} 456
js_code_size_bytes_bucket{le="10000"} 1234
js_code_size_bytes_sum 15678432
js_code_size_bytes_count 1572
```

**Use cases**:

- Monitor script size distribution
- Identify large scripts that may impact performance
- Track payload size trends

---

### Gauge Metrics

#### `js_pool_size`

Number of JavaScript VMs in the pool (constant).

**Type**: Gauge  
**Labels**: None

**Example value**:

```
js_pool_size 4
```

**Use cases**:

- Monitor configured pool size
- Verify configuration deployment

---

#### `js_pool_available`

Number of available (idle) JavaScript VMs in the pool.

**Type**: Gauge  
**Labels**: None

**Example value**:

```
js_pool_available 2
```

**Use cases**:

- Monitor VM utilization
- Detect pool saturation
- Identify capacity issues

---

#### `js_active_executions`

Number of currently active JavaScript executions.

**Type**: Gauge  
**Labels**: None

**Example value**:

```
js_active_executions 2
```

**Use cases**:

- Monitor concurrent execution load
- Track real-time activity
- Identify burst patterns

---

## Configuration

### Enable Metrics

Add to your `.rr.yaml`:

```yaml
# Metrics plugin configuration
metrics:
  address: 127.0.0.1:2112
  collect:
    app_metric: true

# JavaScript plugin
js:
  pool_size: 4
  default_timeout_ms: 30000
```

### Metrics Endpoint

Metrics are exposed via HTTP at the configured address:

```
http://127.0.0.1:2112/metrics
```

---

## Accessing Metrics

### View All Metrics

```bash
curl http://localhost:2112/metrics
```

### Filter Specific Plugin Metrics

```bash
curl http://localhost:2112/metrics | grep "^js_"
```

### Check Specific Metric

```bash
curl -s http://localhost:2112/metrics | grep js_executions_total
```

---

## PromQL Queries

### Execution Metrics

#### Success Rate

```promql
# Overall success rate (last 5 minutes)
sum(rate(js_executions_total{status="success"}[5m])) 
/ 
sum(rate(js_executions_total[5m])) * 100
```

#### Error Rate

```promql
# Error rate per second
rate(js_executions_total{status="error"}[5m])
```

#### Timeout Rate

```promql
# Timeout rate percentage
sum(rate(js_executions_total{status="timeout"}[5m])) 
/ 
sum(rate(js_executions_total[5m])) * 100
```

#### Total Executions Per Second

```promql
# Execution throughput
sum(rate(js_executions_total[5m]))
```

---

### Performance Metrics

#### P50 Latency (Median)

```promql
# 50th percentile execution duration
histogram_quantile(0.50, 
  sum(rate(js_execution_duration_seconds_bucket{status="success"}[5m])) by (le)
)
```

#### P95 Latency

```promql
# 95th percentile execution duration
histogram_quantile(0.95, 
  sum(rate(js_execution_duration_seconds_bucket{status="success"}[5m])) by (le)
)
```

#### P99 Latency

```promql
# 99th percentile execution duration
histogram_quantile(0.99, 
  sum(rate(js_execution_duration_seconds_bucket{status="success"}[5m])) by (le)
)
```

#### Average Duration

```promql
# Average execution duration
rate(js_execution_duration_seconds_sum{status="success"}[5m]) 
/ 
rate(js_execution_duration_seconds_count{status="success"}[5m])
```

---

### Resource Utilization

#### VM Pool Utilization

```promql
# Percentage of VMs in use
(js_pool_size - js_pool_available) / js_pool_size * 100
```

#### Available VMs

```promql
# Number of idle VMs
js_pool_available
```

#### Active Executions

```promql
# Current active executions
js_active_executions
```

#### Pool Saturation

```promql
# Pool is fully utilized (0 available VMs)
js_pool_available == 0
```

---

### Code Size Metrics

#### Average Code Size

```promql
# Average script size in bytes
rate(js_code_size_bytes_sum[5m]) / rate(js_code_size_bytes_count[5m])
```

#### Large Scripts

```promql
# Scripts larger than 50KB
sum(rate(js_code_size_bytes_bucket{le="50000"}[5m])) 
- 
sum(rate(js_code_size_bytes_bucket{le="10000"}[5m]))
```

---

## Grafana Dashboard

### Sample Dashboard JSON

Create a Grafana dashboard with the following panels:

#### Panel 1: Execution Rate

```json
{
  "title": "Execution Rate",
  "targets": [
    {
      "expr": "sum(rate(js_executions_total[5m])) by (status)",
      "legendFormat": "{{status}}"
    }
  ],
  "type": "graph"
}
```

#### Panel 2: Success Rate

```json
{
  "title": "Success Rate %",
  "targets": [
    {
      "expr": "sum(rate(js_executions_total{status=\"success\"}[5m])) / sum(rate(js_executions_total[5m])) * 100"
    }
  ],
  "type": "singlestat"
}
```

#### Panel 3: Latency Percentiles

```json
{
  "title": "Execution Latency",
  "targets": [
    {
      "expr": "histogram_quantile(0.50, sum(rate(js_execution_duration_seconds_bucket{status=\"success\"}[5m])) by (le))",
      "legendFormat": "p50"
    },
    {
      "expr": "histogram_quantile(0.95, sum(rate(js_execution_duration_seconds_bucket{status=\"success\"}[5m])) by (le))",
      "legendFormat": "p95"
    },
    {
      "expr": "histogram_quantile(0.99, sum(rate(js_execution_duration_seconds_bucket{status=\"success\"}[5m])) by (le))",
      "legendFormat": "p99"
    }
  ],
  "type": "graph"
}
```

#### Panel 4: VM Pool Utilization

```json
{
  "title": "VM Pool Utilization %",
  "targets": [
    {
      "expr": "(js_pool_size - js_pool_available) / js_pool_size * 100"
    }
  ],
  "type": "gauge"
}
```

#### Panel 5: Active Executions

```json
{
  "title": "Active Executions",
  "targets": [
    {
      "expr": "js_active_executions"
    }
  ],
  "type": "graph"
}
```

---

## Alerting Rules

### High Error Rate

```yaml
groups:
  - name: javascript_plugin
    rules:
      - alert: HighJavaScriptErrorRate
        expr: |
          sum(rate(js_executions_total{status="error"}[5m])) 
          / 
          sum(rate(js_executions_total[5m])) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High JavaScript execution error rate"
          description: "Error rate is {{ $value | humanizePercentage }} (threshold: 5%)"
```

### High Timeout Rate

```yaml
      - alert: HighJavaScriptTimeoutRate
        expr: |
          sum(rate(js_executions_total{status="timeout"}[5m])) 
          / 
          sum(rate(js_executions_total[5m])) > 0.01
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High JavaScript execution timeout rate"
          description: "Timeout rate is {{ $value | humanizePercentage }} (threshold: 1%)"
```

### Pool Saturation

```yaml
      - alert: JavaScriptPoolSaturated
        expr: js_pool_available == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "JavaScript VM pool fully saturated"
          description: "No available VMs in pool for {{ $for }}"
```

### High P99 Latency

```yaml
      - alert: HighJavaScriptP99Latency
        expr: |
          histogram_quantile(0.99, 
            sum(rate(js_execution_duration_seconds_bucket{status="success"}[5m])) by (le)
          ) > 5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High JavaScript P99 latency"
          description: "P99 latency is {{ $value }}s (threshold: 5s)"
```

### Low Success Rate

```yaml
      - alert: LowJavaScriptSuccessRate
        expr: |
          sum(rate(js_executions_total{status="success"}[5m])) 
          / 
          sum(rate(js_executions_total[5m])) < 0.95
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "Low JavaScript execution success rate"
          description: "Success rate is {{ $value | humanizePercentage }} (threshold: 95%)"
```

---

## Troubleshooting with Metrics

### High Error Rate

**Symptom**: `js_executions_total{status="error"}` increasing rapidly

**Investigation**:

1. Check error logs for common patterns
2. Verify JavaScript syntax in recent deployments
3. Review timeout configuration
4. Check for resource exhaustion

### Pool Saturation

**Symptom**: `js_pool_available == 0` for extended period

**Solution**:

1. Increase `pool_size` in configuration
2. Optimize slow scripts
3. Implement caching for frequently used scripts
4. Consider horizontal scaling

### High Latency

**Symptom**: P95/P99 latencies increasing

**Investigation**:

1. Check `js_code_size_bytes` for large scripts
2. Review script complexity
3. Verify no CPU/memory contention
4. Consider script optimization

---

## Integration Examples

### Prometheus Configuration

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'roadrunner'
    static_configs:
      - targets: [ 'localhost:2112' ]
    scrape_interval: 15s
```

### Docker Compose

```yaml
version: '3.8'

services:
  roadrunner:
    image: your-roadrunner-image
    ports:
      - "8080:8080"
      - "2112:2112"  # Metrics
    volumes:
      - ./.rr.yaml:/app/.rr.yaml

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - ./alerts.yml:/etc/prometheus/alerts.yml

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
```

---

## Best Practices

1. **Monitor Success Rate**: Track `js_executions_total{status="success"}` to ensure healthy operation
2. **Watch Pool Utilization**: Alert when `js_pool_available` approaches zero
3. **Track Latency**: Use P95/P99 latencies for SLA monitoring
4. **Log Correlation**: Use `request_id` from RPC calls to correlate metrics with logs
5. **Capacity Planning**: Monitor trends in execution rate to plan pool size increases
6. **Cost Optimization**: Use code size metrics to identify optimization opportunities

---

## Metric Retention

Prometheus default retention is 15 days. For longer retention:

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

storage:
  tsdb:
    retention.time: 90d  # Keep 90 days
```

Or use remote write to long-term storage (Thanos, Cortex, VictoriaMetrics).

---

## Related Documentation

- [Prometheus Query Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Grafana Dashboard Guide](https://grafana.com/docs/grafana/latest/dashboards/)
- [PromQL Functions](https://prometheus.io/docs/prometheus/latest/querying/functions/)
