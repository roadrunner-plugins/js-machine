# JavaScript Machine Plugin

A simple RoadRunner plugin that executes JavaScript code using the [otto](https://github.com/robertkrimen/otto)
JavaScript interpreter.

## Features

- **VM Pool**: Manages a pool of JavaScript VMs for concurrent execution
- **Timeout Control**: Configurable execution timeouts to prevent runaway scripts
- **Simple RPC Interface**: Single RPC method for executing JavaScript code
- **Prometheus Metrics**: Comprehensive observability with execution stats, latency, and pool utilization
- **Graceful Shutdown**: Properly handles shutdown with active execution cleanup

## Installation

```bash
go get github.com/roadrunner-server/js-machine
```

## Configuration

Add to your `.rr.yaml`:

```yaml
js:
  pool_size: 4              # Number of JavaScript VMs in pool (default: 4)
  max_memory_mb: 512        # Memory limit per VM (default: 512)
  default_timeout_ms: 30000 # Default execution timeout in ms (default: 30000)
```

## RPC Interface

### Execute Method

Executes JavaScript code and returns the result.

**Request Structure:**

```go
type ExecuteRequest struct {
    Code       string `json:"code"`       // JavaScript code to execute
    TimeoutMs  int    `json:"timeout_ms"` // Execution timeout (optional)
    RequestID  string `json:"request_id,omitempty"` // Request correlation ID
}
```

**Response Structure:**

```go
type ExecuteResponse struct {
    Result     interface{} `json:"result"`          // Execution result
    DurationMs int64       `json:"duration_ms"`     // Execution time
    Error      string      `json:"error,omitempty"` // Error message if failed
    RequestID  string      `json:"request_id,omitempty"` // Request correlation ID
}
```

## PHP Usage

### Basic Example

```php
<?php

use Spiral\Goridge\RPC\RPC;
use Spiral\Goridge\RPC\Codec\JsonCodec;

$rpc = new RPC(
    RPC::create('tcp://127.0.0.1:6001')
        ->withCodec(new JsonCodec())
);

// Execute JavaScript
$response = $rpc->call('js.Execute', [
    'code' => 'var result = 2 + 2; result;',
    'timeout_ms' => 5000,
    'request_id' => 'req-123'
]);

echo "Result: " . $response['result'] . "\n";        // 4
echo "Duration: " . $response['duration_ms'] . "ms\n";
```

### Complex Calculation

```php
$response = $rpc->call('js.Execute', [
    'code' => '
        function fibonacci(n) {
            if (n <= 1) return n;
            return fibonacci(n - 1) + fibonacci(n - 2);
        }
        fibonacci(10);
    ',
    'timeout_ms' => 1000
]);

echo "Fibonacci(10) = " . $response['result'] . "\n"; // 55
```

### JSON Data Processing

```php
$response = $rpc->call('js.Execute', [
    'code' => '
        var data = [
            { name: "Alice", age: 30 },
            { name: "Bob", age: 25 },
            { name: "Charlie", age: 35 }
        ];
        
        var adults = data.filter(function(person) {
            return person.age >= 30;
        });
        
        JSON.stringify(adults);
    '
]);

$adults = json_decode($response['result'], true);
print_r($adults);
```

### Error Handling

```php
$response = $rpc->call('js.Execute', [
    'code' => 'throw new Error("Something went wrong");'
]);

if (!empty($response['error'])) {
    echo "JavaScript Error: " . $response['error'] . "\n";
}
```

## Laravel Integration

### Service Provider

Create `app/Providers/JavaScriptServiceProvider.php`:

```php
<?php

namespace App\Providers;

use Illuminate\Support\ServiceProvider;
use Spiral\Goridge\RPC\RPC;
use Spiral\Goridge\RPC\Codec\JsonCodec;

class JavaScriptServiceProvider extends ServiceProvider
{
    public function register()
    {
        $this->app->singleton('js', function ($app) {
            $rpc = new RPC(
                RPC::create(config('roadrunner.rpc_address', 'tcp://127.0.0.1:6001'))
                    ->withCodec(new JsonCodec())
            );
            
            return new \App\Services\JavaScriptService($rpc);
        });
    }
}
```

### Service Class

Create `app/Services/JavaScriptService.php`:

```php
<?php

namespace App\Services;

use Spiral\Goridge\RPC\RPCInterface;

class JavaScriptService
{
    private RPCInterface $rpc;
    
    public function __construct(RPCInterface $rpc)
    {
        $this->rpc = $rpc;
    }
    
    public function execute(string $code, int $timeoutMs = 5000, string $requestId = null): array
    {
        return $this->rpc->call('js.Execute', [
            'code' => $code,
            'timeout_ms' => $timeoutMs,
            'request_id' => $requestId ?? uniqid('js-', true)
        ]);
    }
    
    public function eval(string $code): mixed
    {
        $response = $this->execute($code);
        
        if (!empty($response['error'])) {
            throw new \RuntimeException($response['error']);
        }
        
        return $response['result'];
    }
}
```

### Usage in Controllers

```php
<?php

namespace App\Http\Controllers;

use App\Services\JavaScriptService;

class CalculationController extends Controller
{
    public function calculate(JavaScriptService $js)
    {
        $result = $js->eval('
            function calculate(x, y) {
                return (x * y) + (x / y);
            }
            calculate(10, 5);
        ');
        
        return response()->json(['result' => $result]);
    }
}
```

## Architecture

### VM Pool Management

The plugin maintains a pool of otto JavaScript VMs:

```
┌─────────────────────────────────────┐
│      JavaScript Plugin              │
│                                     │
│  ┌───────────────────────────────┐ │
│  │      VM Pool (Channel)        │ │
│  │                               │ │
│  │  ┌──────┐  ┌──────┐          │ │
│  │  │ VM 1 │  │ VM 2 │  ...     │ │
│  │  └──────┘  └──────┘          │ │
│  └───────────────────────────────┘ │
│                                     │
│  PHP Worker ──RPC──> Execute()      │
│                  ↓                  │
│            Acquire VM from Pool     │
│                  ↓                  │
│            Run JavaScript           │
│                  ↓                  │
│            Return VM to Pool        │
│                                     │
│  Prometheus Metrics:                │
│  - js_executions_total              │
│  - js_execution_duration_seconds    │
│  - js_pool_available                │
│  - js_active_executions             │
└─────────────────────────────────────┘
```

### Execution Flow

1. **Request Received**: PHP sends JavaScript code via RPC
2. **VM Acquisition**: Plugin acquires a VM from the pool (blocks if all busy)
3. **Timeout Setup**: Creates context with timeout and watchdog goroutine
4. **Execution**: Runs JavaScript in separate goroutine
5. **Result Return**: Converts otto.Value to Go interface{} and returns
6. **VM Release**: Returns VM to pool for reuse

### Timeout Mechanism

```go
// Watchdog goroutine monitors execution
go func () {
select {
case <-execCtx.Done():
// Timeout occurred, interrupt VM
vm.Interrupt <- func () {
panic("execution timeout")
}
case <-watchdogDone:
// Execution completed normally
}
}()
```

## Limitations

### Otto Engine Limitations

- **ECMAScript 5.1**: Does not support ES6+ features (let, const, arrow functions, classes)
- **No Async/Await**: Promises and async patterns not supported
- **Limited Stdlib**: No Node.js modules or browser APIs
- **Regexp Limitations**: Uses Go's regexp engine (no lookaheads/lookbehinds)

### Performance Considerations

- **Single-threaded**: Each VM executes one script at a time
- **Pool Size**: Adjust `pool_size` based on CPU cores and workload
- **Memory**: Each VM consumes ~20MB base memory
- **Timeout**: Always set reasonable timeouts to prevent resource exhaustion

## Security Considerations

### Code Execution Risks

⚠️ **Warning**: This plugin executes arbitrary JavaScript code. Only execute trusted code.

**Recommendations**:

- Run RoadRunner in isolated environment (container, VM)
- Set strict resource limits (memory, timeout)
- Validate/sanitize input before execution
- Monitor execution metrics for anomalies

### Future Enhancements (Out of Scope)

The following features are intentionally excluded from this minimal implementation:

- **Metrics**: Prometheus integration for execution stats
- **Go Bindings**: HTTP client, logging, cache access from JavaScript
- **Script Registry**: Pre-loaded named functions
- **Async Execution**: Fire-and-forget mode with job tracking
- **Sandboxing**: Restricted filesystem/network access
- **ES6+ Support**: Requires different JavaScript engine (V8, QuickJS)

## Troubleshooting

### VM Pool Exhaustion

**Symptom**: Requests timeout waiting for available VM

**Metrics**: Check `js_pool_available` gauge (should be > 0)

**Solution**: Increase `pool_size` in configuration

```yaml
js:
  pool_size: 8  # Increase from default 4
```

### Memory Issues

**Symptom**: RoadRunner OOM or high memory usage

**Metrics**: Monitor `js_pool_size * max_memory_mb` total

**Solution**: Reduce pool size or implement VM rotation

```yaml
js:
  pool_size: 2
  max_memory_mb: 256
```

### High Error Rate

**Symptom**: Many failed executions

**Metrics**: Check `js_executions_total{status="error"}`

**Investigation**:
- Review error logs
- Verify JavaScript syntax
- Check timeout configuration

### Performance Degradation

**Symptom**: Slow execution times

**Metrics**: Monitor `js_execution_duration_seconds` percentiles

**Investigation**:
- Check `js_code_size_bytes` for large scripts
- Review `js_active_executions` for high concurrency
- Verify no resource contention

## Metrics & Monitoring

The plugin exposes comprehensive Prometheus metrics. See [METRICS.md](METRICS.md) for detailed documentation including:

- All available metrics (counters, histograms, gauges)
- PromQL query examples
- Grafana dashboard templates
- Alerting rules for production monitoring

**Quick Start**: Enable metrics in `.rr.yaml`:

```yaml
metrics:
  address: 127.0.0.1:2112

js:
  pool_size: 4
```

Access metrics at `http://localhost:2112/metrics`

Key metrics:
- `js_executions_total` - Total executions by status
- `js_execution_duration_seconds` - Latency distribution
- `js_pool_available` - Available VMs
- `js_active_executions` - Current concurrency

### Syntax Errors

**Symptom**: `SyntaxError` in response

**Cause**: Invalid JavaScript or unsupported ES6+ syntax

**Solution**: Use ES5.1 syntax only

```javascript
// ❌ ES6 - Not supported
const result = (x) => x * 2;

// ✅ ES5 - Supported
var result = function (x) {
    return x * 2;
};
```

## License

MIT

## Contributing

This is a minimal reference implementation. For production use, consider:

- Adding comprehensive metrics
- Implementing Go function bindings
- Adding script caching
- Supporting async execution modes
