# JavaScript Plugin Go Bindings

This document describes all Go functions exposed to JavaScript code through the RoadRunner JavaScript plugin. These
bindings allow JavaScript code to interact with RoadRunner's infrastructure including logging, metrics, and more.

## Table of Contents

- [Logging (`log.*`)](#logging-log)
- [Metrics (`metrics.*`)](#metrics-metrics)
- [Usage Examples](#usage-examples)
- [Best Practices](#best-practices)

---

## Logging (`log.*`)

The `log` object provides structured logging capabilities that integrate with RoadRunner's logging infrastructure using
Zap logger.

### Available Methods

#### `log.info(message, fields?)`

Logs an informational message.

**Parameters:**

- `message` (string): The log message
- `fields` (object, optional): Key-value pairs for structured logging

**Example:**

```javascript
log.info("User logged in");
log.info("Order processed", {
    order_id: 12345,
    total: 99.99,
    customer: "john@example.com"
});
```

#### `log.error(message, fields?)`

Logs an error message.

**Parameters:**

- `message` (string): The error message
- `fields` (object, optional): Key-value pairs for structured logging

**Example:**

```javascript
log.error("Payment failed");
log.error("Database connection timeout", {
    error_code: "DB_TIMEOUT",
    retry_count: 3,
    duration_ms: 5000
});
```

#### `log.warn(message, fields?)`

Logs a warning message.

**Parameters:**

- `message` (string): The warning message
- `fields` (object, optional): Key-value pairs for structured logging

**Example:**

```javascript
log.warn("Rate limit approaching");
log.warn("Deprecated API usage", {
    api_endpoint: "/v1/users",
    replacement: "/v2/users",
    deprecation_date: "2024-12-31"
});
```

#### `log.debug(message, fields?)`

Logs a debug message (only visible when debug logging is enabled).

**Parameters:**

- `message` (string): The debug message
- `fields` (object, optional): Key-value pairs for structured logging

**Example:**

```javascript
log.debug("Cache hit");
log.debug("Request details", {
    method: "POST",
    path: "/api/users",
    headers: {"content-type": "application/json"},
    body_size: 1024
});
```

---

## Metrics (`metrics.*`)

The `metrics` object provides Prometheus metrics integration, allowing JavaScript code to record custom metrics that can
be scraped and visualized.

### Metric Types

#### `metrics.increment(name, labels?)`

Increments a counter metric by 1. Counters are cumulative values that only increase (e.g., request counts, error
counts).

**Parameters:**

- `name` (string): Metric name (automatically prefixed with `js_user_`)
- `labels` (object, optional): Label key-value pairs for metric dimensions

**Example:**

```javascript
// Simple counter
metrics.increment("api_requests");

// Counter with labels
metrics.increment("api_requests", {
    method: "POST",
    endpoint: "/users",
    status: "success"
});

// Error tracking
metrics.increment("errors", {
    type: "validation",
    field: "email"
});
```

**Prometheus Query Examples:**

```promql
# Total API requests
sum(js_user_api_requests_total)

# Requests by endpoint
sum by (endpoint) (js_user_api_requests_total)

# Error rate
rate(js_user_errors_total[5m])
```

#### `metrics.gauge(name, value, labels?)`

Sets a gauge metric to a specific value. Gauges represent values that can go up or down (e.g., queue size, memory
usage).

**Parameters:**

- `name` (string): Metric name (automatically prefixed with `js_user_`)
- `value` (number): The value to set
- `labels` (object, optional): Label key-value pairs for metric dimensions

**Example:**

```javascript
// Queue size
metrics.gauge("queue_size", 42);

// Memory usage with labels
metrics.gauge("memory_usage_mb", 256, {
    instance: "worker-1"
});

// Connection pool
metrics.gauge("active_connections", 15, {
    pool: "database",
    host: "db-primary"
});

// Temperature monitoring
metrics.gauge("cpu_temperature", 65.5, {
    core: "0"
});
```

**Prometheus Query Examples:**

```promql
# Current queue size
js_user_queue_size

# Average memory usage
avg(js_user_memory_usage_mb)

# Connection pool utilization
js_user_active_connections / 100 * 100
```

#### `metrics.histogram(name, value, labels?)`

Observes a value in a histogram metric. Histograms track the distribution of values (e.g., request duration, response
sizes).

**Parameters:**

- `name` (string): Metric name (automatically prefixed with `js_user_`)
- `value` (number): The value to observe
- `labels` (object, optional): Label key-value pairs for metric dimensions

**Default Buckets:** `[0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]` seconds

**Example:**

```javascript
// Track request duration
const start = Date.now();
await processRequest();
const duration = (Date.now() - start) / 1000; // Convert to seconds
metrics.histogram("request_duration", duration, {
    endpoint: "/api/users"
});

// Response size distribution
metrics.histogram("response_size_kb", responseSize / 1024, {
    content_type: "application/json"
});

// Database query duration
metrics.histogram("query_duration", 0.025, {
    query_type: "SELECT",
    table: "users"
});
```

**Prometheus Query Examples:**

```promql
# 95th percentile request duration
histogram_quantile(0.95, sum by (le, endpoint) (rate(js_user_request_duration_bucket[5m])))

# Average request duration
rate(js_user_request_duration_sum[5m]) / rate(js_user_request_duration_count[5m])

# Request throughput
rate(js_user_request_duration_count[5m])
```

---

## Usage Examples

### Example 1: Webhook Processing with Logging and Metrics

```javascript
async function processWebhook(payload) {
    const start = Date.now();

    try {
        log.info("Processing webhook", {
            webhook_id: payload.id,
            source: payload.source,
            event_type: payload.type
        });

        // Validate payload
        if (!payload.signature) {
            log.warn("Missing webhook signature", {
                webhook_id: payload.id
            });
            metrics.increment("webhook_validation_failed", {
                reason: "missing_signature"
            });
            return {status: "invalid"};
        }

        // Process webhook
        await enrichWebhookData(payload);
        await updateInventory(payload);
        await sendNotification(payload);

        // Track success
        const duration = (Date.now() - start) / 1000;
        metrics.histogram("webhook_duration", duration, {
            source: payload.source,
            status: "success"
        });
        metrics.increment("webhooks_processed", {
            source: payload.source,
            status: "success"
        });

        log.info("Webhook processed successfully", {
            webhook_id: payload.id,
            duration_ms: Date.now() - start
        });

        return {status: "success"};

    } catch (error) {
        log.error("Webhook processing failed", {
            webhook_id: payload.id,
            error: error.message,
            stack: error.stack
        });

        metrics.increment("webhooks_processed", {
            source: payload.source,
            status: "error"
        });

        throw error;
    }
}
```

### Example 2: API Rate Limiting Monitor

```javascript
const rateLimits = {
    stripe: {used: 0, limit: 100},
    sendgrid: {used: 0, limit: 500},
    shopify: {used: 0, limit: 40}
};

function updateRateLimitMetrics() {
    for (const [service, limits] of Object.entries(rateLimits)) {
        // Track usage as gauge
        metrics.gauge("api_rate_limit_used", limits.used, {
            service: service
        });

        // Track percentage
        const percentage = (limits.used / limits.limit) * 100;
        metrics.gauge("api_rate_limit_percentage", percentage, {
            service: service
        });

        // Warn if approaching limit
        if (percentage > 80) {
            log.warn("API rate limit approaching", {
                service: service,
                used: limits.used,
                limit: limits.limit,
                percentage: percentage.toFixed(2)
            });
        }
    }
}

function incrementApiCall(service) {
    if (rateLimits[service]) {
        rateLimits[service].used++;
        metrics.increment("api_calls", {service: service});
        updateRateLimitMetrics();
    }
}
```

### Example 3: Background Job Processing with Progress Tracking

```javascript
async function processEmailCampaign(campaign) {
    const totalRecipients = campaign.recipients.length;
    let processed = 0;
    let successful = 0;
    let failed = 0;

    log.info("Starting email campaign", {
        campaign_id: campaign.id,
        total_recipients: totalRecipients
    });

    for (const recipient of campaign.recipients) {
        const start = Date.now();

        try {
            await sendEmail(recipient, campaign.template);
            successful++;

            const duration = (Date.now() - start) / 1000;
            metrics.histogram("email_send_duration", duration, {
                status: "success"
            });

        } catch (error) {
            failed++;

            log.error("Email send failed", {
                campaign_id: campaign.id,
                recipient: recipient.email,
                error: error.message
            });

            metrics.increment("email_failures", {
                error_type: error.code || "unknown"
            });
        }

        processed++;

        // Update progress gauge every 10 emails
        if (processed % 10 === 0) {
            const progress = (processed / totalRecipients) * 100;
            metrics.gauge("campaign_progress", progress, {
                campaign_id: campaign.id
            });

            log.debug("Campaign progress update", {
                campaign_id: campaign.id,
                processed: processed,
                successful: successful,
                failed: failed,
                progress_percentage: progress.toFixed(2)
            });
        }
    }

    // Final metrics
    metrics.gauge("campaign_progress", 100, {
        campaign_id: campaign.id
    });

    log.info("Email campaign completed", {
        campaign_id: campaign.id,
        total: totalRecipients,
        successful: successful,
        failed: failed,
        success_rate: ((successful / totalRecipients) * 100).toFixed(2)
    });

    return {
        total: totalRecipients,
        successful: successful,
        failed: failed
    };
}
```

### Example 4: Performance Monitoring

```javascript
class PerformanceMonitor {
    constructor(operation) {
        this.operation = operation;
        this.start = Date.now();
        this.checkpoints = [];
    }

    checkpoint(name) {
        const now = Date.now();
        const duration = now - this.start;

        this.checkpoints.push({
            name: name,
            timestamp: now,
            duration: duration
        });

        log.debug("Performance checkpoint", {
            operation: this.operation,
            checkpoint: name,
            duration_ms: duration
        });

        metrics.histogram("operation_checkpoint_duration", duration / 1000, {
            operation: this.operation,
            checkpoint: name
        });
    }

    complete() {
        const totalDuration = Date.now() - this.start;

        log.info("Operation completed", {
            operation: this.operation,
            total_duration_ms: totalDuration,
            checkpoints: this.checkpoints.length
        });

        metrics.histogram("operation_total_duration", totalDuration / 1000, {
            operation: this.operation
        });

        return {
            duration: totalDuration,
            checkpoints: this.checkpoints
        };
    }
}

// Usage
async function processOrder(order) {
    const perf = new PerformanceMonitor("order_processing");

    await validateOrder(order);
    perf.checkpoint("validation");

    await checkInventory(order);
    perf.checkpoint("inventory_check");

    await processPayment(order);
    perf.checkpoint("payment");

    await createShipment(order);
    perf.checkpoint("shipment");

    return perf.complete();
}
```

### Example 5: Resource Pool Monitoring

```javascript
class ResourcePool {
    constructor(name, size) {
        this.name = name;
        this.size = size;
        this.available = size;
        this.active = 0;

        // Initialize metrics
        metrics.gauge("pool_size", size, {pool: name});
        metrics.gauge("pool_available", size, {pool: name});
        metrics.gauge("pool_active", 0, {pool: name});
    }

    acquire() {
        if (this.available <= 0) {
            log.warn("Resource pool exhausted", {
                pool: this.name,
                size: this.size
            });
            metrics.increment("pool_exhaustion", {pool: this.name});
            throw new Error(`No available resources in pool: ${this.name}`);
        }

        this.available--;
        this.active++;

        metrics.gauge("pool_available", this.available, {pool: this.name});
        metrics.gauge("pool_active", this.active, {pool: this.name});
        metrics.increment("pool_acquisitions", {pool: this.name});

        log.debug("Resource acquired", {
            pool: this.name,
            available: this.available,
            active: this.active
        });
    }

    release() {
        if (this.active <= 0) {
            log.error("Invalid pool release", {
                pool: this.name,
                active: this.active
            });
            return;
        }

        this.available++;
        this.active--;

        metrics.gauge("pool_available", this.available, {pool: this.name});
        metrics.gauge("pool_active", this.active, {pool: this.name});
        metrics.increment("pool_releases", {pool: this.name});

        log.debug("Resource released", {
            pool: this.name,
            available: this.available,
            active: this.active
        });
    }

    getUtilization() {
        return (this.active / this.size) * 100;
    }
}

// Usage
const dbPool = new ResourcePool("database", 10);
const apiPool = new ResourcePool("external_api", 5);

async function makeApiCall() {
    apiPool.acquire();
    try {
        const response = await fetch("https://api.example.com/data");
        return response.json();
    } finally {
        apiPool.release();
    }
}
```

### Example 6: Error Tracking and Classification

```javascript
class ErrorTracker {
    static track(error, context = {}) {
        // Classify error
        const errorType = this.classifyError(error);
        const severity = this.determineSeverity(error);

        // Log with appropriate level
        if (severity === "critical") {
            log.error("Critical error occurred", {
                error_type: errorType,
                message: error.message,
                stack: error.stack,
                ...context
            });
        } else if (severity === "warning") {
            log.warn("Warning-level error", {
                error_type: errorType,
                message: error.message,
                ...context
            });
        } else {
            log.error("Error occurred", {
                error_type: errorType,
                message: error.message,
                stack: error.stack,
                ...context
            });
        }

        // Track metrics
        metrics.increment("errors", {
            type: errorType,
            severity: severity
        });

        // Track error by context if available
        if (context.operation) {
            metrics.increment("operation_errors", {
                operation: context.operation,
                error_type: errorType
            });
        }
    }

    static classifyError(error) {
        if (error.code === "ECONNREFUSED") return "connection_refused";
        if (error.code === "ETIMEDOUT") return "timeout";
        if (error.name === "ValidationError") return "validation";
        if (error.statusCode === 404) return "not_found";
        if (error.statusCode === 403) return "forbidden";
        if (error.statusCode === 500) return "server_error";
        return "unknown";
    }

    static determineSeverity(error) {
        const criticalCodes = ["ECONNREFUSED", "ENOTFOUND"];
        if (criticalCodes.includes(error.code)) return "critical";

        if (error.statusCode >= 500) return "error";
        if (error.statusCode >= 400) return "warning";

        return "error";
    }
}

// Usage
async function fetchUserData(userId) {
    try {
        const response = await fetch(`/api/users/${userId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return await response.json();
    } catch (error) {
        ErrorTracker.track(error, {
            operation: "fetch_user_data",
            user_id: userId,
            timestamp: new Date().toISOString()
        });
        throw error;
    }
}
```

---

## Best Practices

### Logging Best Practices

1. **Use Appropriate Log Levels**
   ```javascript
   // ✅ Good
   log.debug("Cache hit", { key: "user:123" });        // Development info
   log.info("User registered", { user_id: 456 });      // Business events
   log.warn("Rate limit at 80%", { service: "api" });  // Potential issues
   log.error("Payment failed", { order_id: 789 });     // Actual errors
   
   // ❌ Bad
   log.info("Detailed debug info...");  // Wrong level
   log.error("User logged in");         // Not an error
   ```

2. **Include Contextual Information**
   ```javascript
   // ✅ Good
   log.error("Database query failed", {
       query: "SELECT * FROM users",
       error: error.message,
       duration_ms: 5000,
       retry_count: 3
   });
   
   // ❌ Bad
   log.error("Query failed");  // No context
   ```

3. **Avoid Logging Sensitive Data**
   ```javascript
   // ✅ Good
   log.info("Payment processed", {
       order_id: order.id,
       amount: order.total,
       last_4_digits: "****1234"
   });
   
   // ❌ Bad
   log.info("Payment processed", {
       credit_card: "4111111111111111",  // PCI violation!
       cvv: "123"                         // Never log this!
   });
   ```

4. **Use Structured Fields**
   ```javascript
   // ✅ Good
   log.info("Order placed", {
       order_id: 12345,
       customer_id: 67890,
       total: 99.99,
       items: 3
   });
   
   // ❌ Bad
   log.info(`Order 12345 placed by customer 67890 for $99.99 with 3 items`);
   ```

### Metrics Best Practices

1. **Choose the Right Metric Type**
   ```javascript
   // ✅ Good
   metrics.increment("requests");              // Counter for totals
   metrics.gauge("queue_size", size);          // Gauge for current values
   metrics.histogram("duration", duration);    // Histogram for distributions
   
   // ❌ Bad
   metrics.gauge("total_requests", count);     // Use counter instead
   metrics.increment("current_memory");        // Use gauge instead
   ```

2. **Use Consistent Naming**
   ```javascript
   // ✅ Good - snake_case, clear purpose
   metrics.increment("api_requests_total");
   metrics.gauge("active_connections");
   metrics.histogram("request_duration_seconds");
   
   // ❌ Bad - inconsistent naming
   metrics.increment("APIRequests");
   metrics.gauge("connections-active");
   metrics.histogram("reqDuration");
   ```

3. **Label Cardinality**
   ```javascript
   // ✅ Good - low cardinality labels
   metrics.increment("requests", {
       method: "POST",           // Few unique values
       status: "success",        // Few unique values
       endpoint: "/api/users"    // Limited endpoints
   });
   
   // ❌ Bad - high cardinality (will explode metrics)
   metrics.increment("requests", {
       user_id: "12345",         // Thousands of users!
       timestamp: Date.now(),     // Infinite unique values!
       request_id: uuid()         // Every request unique!
   });
   ```

4. **Histogram Values in Seconds**
   ```javascript
   // ✅ Good - use seconds for durations
   const start = Date.now();
   await operation();
   const duration = (Date.now() - start) / 1000;  // Convert to seconds
   metrics.histogram("operation_duration", duration);
   
   // ❌ Bad - milliseconds don't align with default buckets
   const durationMs = Date.now() - start;
   metrics.histogram("operation_duration", durationMs);
   ```

5. **Don't Overuse Metrics**
   ```javascript
   // ✅ Good - meaningful metrics
   metrics.increment("orders_completed");
   metrics.histogram("checkout_duration", duration);
   
   // ❌ Bad - unnecessary granularity
   metrics.increment("button_clicked");           // Too granular
   metrics.histogram("variable_initialized", 0);  // Meaningless
   ```

### Performance Considerations

1. **Batch Logging in Loops**
   ```javascript
   // ✅ Good
   const results = [];
   for (const item of items) {
       results.push(await process(item));
   }
   log.info("Batch processed", {
       total: items.length,
       successful: results.filter(r => r.success).length
   });
   
   // ❌ Bad - logs every iteration
   for (const item of items) {
       log.info("Processing item", { id: item.id });
       await process(item);
   }
   ```

2. **Avoid Expensive Operations in Log/Metric Calls**
   ```javascript
   // ✅ Good - only compute when needed
   if (debugEnabled) {
       const details = computeExpensiveDetails();
       log.debug("Details", details);
   }
   
   // ❌ Bad - always computes even if not logged
   log.debug("Details", computeExpensiveDetails());
   ```

---

## Metric Namespace

All user-defined metrics are automatically prefixed with `js_user_` to avoid conflicts with plugin internal metrics:

- `metrics.increment("requests")` → `js_user_requests_total`
- `metrics.gauge("queue_size", 10)` → `js_user_queue_size`
- `metrics.histogram("duration", 0.5)` → `js_user_duration_bucket`, `js_user_duration_sum`, `js_user_duration_count`

## Integration with RoadRunner Metrics Plugin

All metrics are automatically registered with Prometheus and exposed via RoadRunner's metrics endpoint (typically
`http://localhost:2112/metrics`).

Example Prometheus configuration:

```yaml
scrape_configs:
  - job_name: 'roadrunner'
    static_configs:
      - targets: [ 'localhost:2112' ]
```

Example Grafana dashboard queries:

```promql
# Request rate by endpoint
rate(js_user_api_requests_total[5m])

# Average request duration
rate(js_user_request_duration_sum[5m]) / rate(js_user_request_duration_count[5m])

# Error rate
rate(js_user_errors_total[5m])

# Queue size
js_user_queue_size
```
