package jsmachine

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/robertkrimen/otto"
	"go.uber.org/zap"
)

// Bindings represents all Go functions exposed to JavaScript
type Bindings struct {
	log     *LogBinding
	metrics *MetricsBinding
}

// newBindings creates a new bindings instance
func newBindings(logger *zap.Logger, plugin *Plugin) *Bindings {
	return &Bindings{
		log:     newLogBinding(logger),
		metrics: newMetricsBinding(plugin),
	}
}

// injectIntoVM injects all bindings into the Otto VM
func (b *Bindings) injectIntoVM(vm *otto.Otto) error {
	// Inject log binding
	if err := b.log.inject(vm); err != nil {
		return fmt.Errorf("failed to inject log binding: %w", err)
	}

	// Inject metrics binding
	if err := b.metrics.inject(vm); err != nil {
		return fmt.Errorf("failed to inject metrics binding: %w", err)
	}

	return nil
}

// LogBinding provides logging functions to JavaScript
type LogBinding struct {
	logger *zap.Logger
}

// newLogBinding creates a new log binding
func newLogBinding(logger *zap.Logger) *LogBinding {
	return &LogBinding{
		logger: logger,
	}
}

// inject injects the log object into the VM
func (l *LogBinding) inject(vm *otto.Otto) error {
	logObj, err := vm.Object(`({})`)
	if err != nil {
		return err
	}

	// log.info(message, fields)
	if err := logObj.Set("info", l.info); err != nil {
		return err
	}

	// log.error(message, fields)
	if err := logObj.Set("error", l.error); err != nil {
		return err
	}

	// log.warn(message, fields)
	if err := logObj.Set("warn", l.warn); err != nil {
		return err
	}

	// log.debug(message, fields)
	if err := logObj.Set("debug", l.debug); err != nil {
		return err
	}

	return vm.Set("log", logObj)
}

// info logs an info message
func (l *LogBinding) info(call otto.FunctionCall) otto.Value {
	message := l.getMessage(call)
	fields := l.getFields(call)
	l.logger.Info(message, fields...)
	return otto.UndefinedValue()
}

// error logs an error message
func (l *LogBinding) error(call otto.FunctionCall) otto.Value {
	message := l.getMessage(call)
	fields := l.getFields(call)
	l.logger.Error(message, fields...)
	return otto.UndefinedValue()
}

// warn logs a warning message
func (l *LogBinding) warn(call otto.FunctionCall) otto.Value {
	message := l.getMessage(call)
	fields := l.getFields(call)
	l.logger.Warn(message, fields...)
	return otto.UndefinedValue()
}

// debug logs a debug message
func (l *LogBinding) debug(call otto.FunctionCall) otto.Value {
	message := l.getMessage(call)
	fields := l.getFields(call)
	l.logger.Debug(message, fields...)
	return otto.UndefinedValue()
}

// getMessage extracts the message from the function call
func (l *LogBinding) getMessage(call otto.FunctionCall) string {
	if len(call.ArgumentList) == 0 {
		return ""
	}
	return call.Argument(0).String()
}

// getFields extracts structured fields from the function call
func (l *LogBinding) getFields(call otto.FunctionCall) []zap.Field {
	if len(call.ArgumentList) < 2 {
		return nil
	}

	// Second argument should be an object with fields
	fieldsValue := call.Argument(1)
	if !fieldsValue.IsObject() {
		return nil
	}

	fieldsObj := fieldsValue.Object()
	keys := fieldsObj.Keys()

	fields := make([]zap.Field, 0, len(keys))
	for _, key := range keys {
		value, err := fieldsObj.Get(key)
		if err != nil {
			continue
		}

		// Convert value to appropriate zap field
		exported, err := value.Export()
		if err != nil {
			continue
		}

		fields = append(fields, zap.Any(key, exported))
	}

	return fields
}

// MetricsBinding provides metrics functions to JavaScript
// Following the metrics plugin pattern: metrics must be pre-registered via metrics plugin
// JavaScript code can only manipulate existing metrics through the metrics plugin's collectors sync.Map
type MetricsBinding struct {
	plugin *Plugin
	mu     sync.RWMutex

	// Cache of collectors loaded from metrics plugin
	// These are fetched from the metrics plugin's collectors sync.Map
	cachedCollectors sync.Map // map[string]prometheus.Collector
}

// newMetricsBinding creates a new metrics binding
func newMetricsBinding(plugin *Plugin) *MetricsBinding {
	return &MetricsBinding{
		plugin: plugin,
	}
}

// inject injects the metrics object into the VM
func (m *MetricsBinding) inject(vm *otto.Otto) error {
	metricsObj, err := vm.Object(`({})`)
	if err != nil {
		return err
	}

	// metrics.add(name, value, labels) - for counters and gauges
	if err := metricsObj.Set("add", m.add); err != nil {
		return err
	}

	// metrics.set(name, value, labels) - for gauges only
	if err := metricsObj.Set("set", m.set); err != nil {
		return err
	}

	// metrics.observe(name, value, labels) - for histograms
	if err := metricsObj.Set("observe", m.observe); err != nil {
		return err
	}

	return vm.Set("metrics", metricsObj)
}

// getCollector retrieves a collector from the metrics plugin
// This follows the same pattern as metrics plugin's rpc.go: c, exist := r.p.collectors.Load(m.Name)
func (m *MetricsBinding) getCollector(name string) (prometheus.Collector, bool) {
	// Check cache first
	if cached, exists := m.cachedCollectors.Load(name); exists {
		return cached.(prometheus.Collector), true
	}

	// Get from metrics plugin
	if m.plugin.metricsPlugin == nil {
		m.plugin.log.Warn("metrics plugin not available", zap.String("metric", name))
		return nil, false
	}

	// Load from metrics plugin's collectors sync.Map
	collector, exists := m.plugin.metricsPlugin.collectors.Load(name)
	if !exists {
		return nil, false
	}

	// Extract the actual collector from the wrapper
	col := collector.(*metricsCollector)
	actualCollector := col.col

	// Cache it for future use
	m.cachedCollectors.Store(name, actualCollector)

	return actualCollector, true
}

// add adds value to a counter or gauge (follows metrics plugin rpc.go pattern)
func (m *MetricsBinding) add(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) < 2 {
		return otto.UndefinedValue()
	}

	name := call.Argument(0).String()
	value, err := call.Argument(1).ToFloat()
	if err != nil {
		return otto.UndefinedValue()
	}

	// Extract labels if provided
	var labelValues []string
	if len(call.ArgumentList) > 2 {
		labelValues = m.extractLabelValues(call, 2)
	}

	// Get collector from metrics plugin (same pattern as rpc.go)
	collector, exists := m.getCollector(name)
	if !exists {
		m.plugin.log.Warn("metric not found in metrics plugin",
			zap.String("name", name))
		return otto.UndefinedValue()
	}

	// Handle different collector types (exact pattern from metrics plugin rpc.go)
	switch c := collector.(type) {
	case prometheus.Counter:
		c.Add(value)

	case *prometheus.CounterVec:
		if len(labelValues) == 0 {
			m.plugin.log.Warn("required labels for collector",
				zap.String("name", name))
			return otto.UndefinedValue()
		}
		counter, err := c.GetMetricWithLabelValues(labelValues...)
		if err != nil {
			m.plugin.log.Error("failed to get metric with labels",
				zap.String("name", name),
				zap.Strings("labels", labelValues),
				zap.Error(err))
			return otto.UndefinedValue()
		}
		counter.Add(value)

	case prometheus.Gauge:
		c.Add(value)

	case *prometheus.GaugeVec:
		if len(labelValues) == 0 {
			m.plugin.log.Warn("required labels for collector",
				zap.String("name", name))
			return otto.UndefinedValue()
		}
		gauge, err := c.GetMetricWithLabelValues(labelValues...)
		if err != nil {
			m.plugin.log.Error("failed to get metric with labels",
				zap.String("name", name),
				zap.Strings("labels", labelValues),
				zap.Error(err))
			return otto.UndefinedValue()
		}
		gauge.Add(value)

	default:
		m.plugin.log.Warn("collector does not support add operation",
			zap.String("name", name))
	}

	return otto.UndefinedValue()
}

// set sets a gauge value (follows metrics plugin rpc.go pattern)
func (m *MetricsBinding) set(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) < 2 {
		return otto.UndefinedValue()
	}

	name := call.Argument(0).String()
	value, err := call.Argument(1).ToFloat()
	if err != nil {
		return otto.UndefinedValue()
	}

	// Extract labels if provided
	var labelValues []string
	if len(call.ArgumentList) > 2 {
		labelValues = m.extractLabelValues(call, 2)
	}

	// Get collector from metrics plugin
	collector, exists := m.getCollector(name)
	if !exists {
		m.plugin.log.Warn("metric not found in metrics plugin",
			zap.String("name", name))
		return otto.UndefinedValue()
	}

	// Handle different gauge types (exact pattern from metrics plugin rpc.go)
	switch c := collector.(type) {
	case prometheus.Gauge:
		c.Set(value)

	case *prometheus.GaugeVec:
		if len(labelValues) == 0 {
			m.plugin.log.Warn("required labels for collector",
				zap.String("name", name))
			return otto.UndefinedValue()
		}
		gauge, err := c.GetMetricWithLabelValues(labelValues...)
		if err != nil {
			m.plugin.log.Error("failed to get metric with labels",
				zap.String("name", name),
				zap.Strings("labels", labelValues),
				zap.Error(err))
			return otto.UndefinedValue()
		}
		gauge.Set(value)

	default:
		m.plugin.log.Warn("collector does not support set operation (only gauges)",
			zap.String("name", name))
	}

	return otto.UndefinedValue()
}

// observe records a histogram observation (follows metrics plugin rpc.go pattern)
func (m *MetricsBinding) observe(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) < 2 {
		return otto.UndefinedValue()
	}

	name := call.Argument(0).String()
	value, err := call.Argument(1).ToFloat()
	if err != nil {
		return otto.UndefinedValue()
	}

	// Extract labels if provided
	var labelValues []string
	if len(call.ArgumentList) > 2 {
		labelValues = m.extractLabelValues(call, 2)
	}

	// Get collector from metrics plugin
	collector, exists := m.getCollector(name)
	if !exists {
		m.plugin.log.Warn("metric not found in metrics plugin",
			zap.String("name", name))
		return otto.UndefinedValue()
	}

	// Handle different histogram types (exact pattern from metrics plugin rpc.go)
	switch c := collector.(type) {
	case prometheus.Histogram:
		c.Observe(value)

	case *prometheus.HistogramVec:
		if len(labelValues) == 0 {
			m.plugin.log.Warn("required labels for collector",
				zap.String("name", name))
			return otto.UndefinedValue()
		}
		observer, err := c.GetMetricWithLabelValues(labelValues...)
		if err != nil {
			m.plugin.log.Error("failed to get metric with labels",
				zap.String("name", name),
				zap.Strings("labels", labelValues),
				zap.Error(err))
			return otto.UndefinedValue()
		}
		observer.Observe(value)

	default:
		m.plugin.log.Warn("collector does not support observe operation (only histograms)",
			zap.String("name", name))
	}

	return otto.UndefinedValue()
}

// extractLabelValues extracts label values as string slice (for GetMetricWithLabelValues)
// This accepts either an array of label values or an object with label key-value pairs
func (m *MetricsBinding) extractLabelValues(call otto.FunctionCall, argIndex int) []string {
	if len(call.ArgumentList) <= argIndex {
		return nil
	}

	labelsValue := call.Argument(argIndex)

	// Handle array of label values: ["value1", "value2"]
	if labelsValue.Class() == "Array" {
		length, err := labelsValue.Object().Get("length")
		if err != nil {
			return nil
		}

		lengthInt, err := length.ToInteger()
		if err != nil {
			return nil
		}

		values := make([]string, lengthInt)
		for i := int64(0); i < lengthInt; i++ {
			item, err := labelsValue.Object().Get(fmt.Sprintf("%d", i))
			if err != nil {
				continue
			}
			values[i] = item.String()
		}
		return values
	}

	// Handle object with label key-value pairs: {method: "GET", status: "200"}
	// Convert to array of values (order matters for GetMetricWithLabelValues!)
	if labelsValue.IsObject() {
		labelsObj := labelsValue.Object()
		keys := labelsObj.Keys()

		values := make([]string, len(keys))
		for i, key := range keys {
			value, err := labelsObj.Get(key)
			if err != nil {
				continue
			}
			values[i] = value.String()
		}
		return values
	}

	return nil
}

// metricsCollector is the internal collector wrapper used by metrics plugin
// This mirrors the structure in metrics/plugin.go
type metricsCollector struct {
	col        prometheus.Collector
	registered bool
}
