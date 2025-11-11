package jsmachine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/robertkrimen/otto"
	"go.uber.org/zap"
)

const (
	PluginName = "js"
)

// Plugin represents the JavaScript execution plugin
type Plugin struct {
	log *zap.Logger
	cfg *Config

	// VM pool management
	vmPool     chan *otto.Otto
	vmPoolSize int
	mu         sync.RWMutex

	// Graceful shutdown
	stopCh chan struct{}
	wg     sync.WaitGroup

	// Prometheus metrics
	executionsTotal   *prometheus.CounterVec
	executionDuration *prometheus.HistogramVec
	poolSizeGauge     prometheus.Gauge
	poolAvailable     prometheus.Gauge
	activeExecutions  prometheus.Gauge
	codeSize          prometheus.Histogram
}

// Configurer interface for configuration access
type Configurer interface {
	UnmarshalKey(name string, out interface{}) error
	Has(name string) bool
}

// Logger interface for dependency injection
type Logger interface {
	NamedLogger(name string) *zap.Logger
}

// Init initializes the plugin
func (p *Plugin) Init(cfg Configurer, log Logger) error {
	const op = "js_plugin_init"

	// Check if plugin is configured
	if !cfg.Has(PluginName) {
		return fmt.Errorf("%s: plugin not configured", op)
	}

	// Initialize configuration
	p.cfg = &Config{}
	if err := cfg.UnmarshalKey(PluginName, p.cfg); err != nil {
		return fmt.Errorf("%s: failed to unmarshal config: %w", op, err)
	}

	// Set defaults
	p.cfg.InitDefaults()

	// Validate configuration
	if err := p.cfg.Validate(); err != nil {
		return fmt.Errorf("%s: config validation failed: %w", op, err)
	}

	// Initialize logger
	p.log = log.NamedLogger(PluginName)

	// Initialize metrics
	p.initMetrics()

	p.log.Info("JavaScript plugin initialized",
		zap.Int("pool_size", p.cfg.PoolSize),
		zap.Int("max_memory_mb", p.cfg.MaxMemoryMB),
		zap.Int("default_timeout_ms", p.cfg.DefaultTimeout),
	)

	return nil
}

// Name returns plugin name
func (p *Plugin) Name() string {
	return PluginName
}

// Serve starts the plugin (initializes VM pool)
func (p *Plugin) Serve() chan error {
	errCh := make(chan error, 1)

	p.vmPoolSize = p.cfg.PoolSize
	p.vmPool = make(chan *otto.Otto, p.vmPoolSize)
	p.stopCh = make(chan struct{})

	// Initialize VM pool
	for i := 0; i < p.vmPoolSize; i++ {
		vm := otto.New()

		// Set up interrupt channel for timeout handling
		vm.Interrupt = make(chan func(), 1)

		p.vmPool <- vm
	}

	p.log.Info("JavaScript plugin started",
		zap.Int("pool_size", p.vmPoolSize),
		zap.Int("default_timeout_ms", p.cfg.DefaultTimeout),
	)

	return errCh
}

// Stop gracefully shuts down the plugin
func (p *Plugin) Stop(ctx context.Context) error {
	p.log.Info("Stopping JavaScript plugin...")

	// Signal shutdown
	close(p.stopCh)

	// Wait for active executions with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.log.Info("All JavaScript executions completed")
	case <-ctx.Done():
		p.log.Warn("Timeout waiting for JavaScript executions, forcing shutdown")
	}

	// Close VM pool
	close(p.vmPool)

	return nil
}

// RPC returns the RPC interface
func (p *Plugin) RPC() interface{} {
	return &rpc{
		plugin: p,
		log:    p.log,
	}
}

// acquireVM gets a VM from the pool
func (p *Plugin) acquireVM(ctx context.Context) (*otto.Otto, error) {
	select {
	case vm := <-p.vmPool:
		return vm, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.stopCh:
		return nil, fmt.Errorf("plugin is shutting down")
	}
}

// releaseVM returns a VM to the pool
func (p *Plugin) releaseVM(vm *otto.Otto) {
	select {
	case p.vmPool <- vm:
	case <-p.stopCh:
		// Plugin is shutting down, don't return to pool
	}
}

// execute runs JavaScript code with timeout
func (p *Plugin) execute(ctx context.Context, script string, timeout time.Duration) (interface{}, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	start := time.Now()
	var status string
	defer func() {
		duration := time.Since(start)
		p.executionDuration.WithLabelValues(status).Observe(duration.Seconds())
		p.executionsTotal.WithLabelValues(status).Inc()
	}()

	// Track code size
	p.codeSize.Observe(float64(len(script)))

	// Track active executions
	p.activeExecutions.Inc()
	defer p.activeExecutions.Dec()

	// Acquire VM from pool
	p.poolAvailable.Dec()
	vm, err := p.acquireVM(ctx)
	if err != nil {
		status = "error"
		p.poolAvailable.Inc()
		return nil, fmt.Errorf("failed to acquire VM: %w", err)
	}
	defer func() {
		p.releaseVM(vm)
		p.poolAvailable.Inc()
	}()

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Result channels
	resultCh := make(chan otto.Value, 1)
	errCh := make(chan error, 1)

	// Watchdog for timeout
	watchdogDone := make(chan struct{})
	defer close(watchdogDone)

	go func() {
		select {
		case <-execCtx.Done():
			vm.Interrupt <- func() {
				panic("execution timeout")
			}
		case <-watchdogDone:
		}
	}()

	// Execute JavaScript
	go func() {
		defer func() {
			if caught := recover(); caught != nil {
				errCh <- fmt.Errorf("execution panic: %v", caught)
			}
		}()

		value, err := vm.Run(script)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- value
	}()

	// Wait for result or timeout
	select {
	case value := <-resultCh:
		// Convert otto.Value to Go interface{}
		exported, err := value.Export()
		if err != nil {
			status = "error"
			return nil, fmt.Errorf("failed to export result: %w", err)
		}
		status = "success"
		return exported, nil

	case err := <-errCh:
		status = "error"
		return nil, fmt.Errorf("execution error: %w", err)

	case <-execCtx.Done():
		status = "timeout"
		return nil, fmt.Errorf("execution timeout after %v", timeout)
	}
}
