package jsmachine

import (
	"fmt"

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
type MetricsBinding struct {
	plugin *Plugin

	// User-defined metrics registry
	userCounters   map[string]*prometheus.CounterVec
	userGauges     map[string]*prometheus.GaugeVec
	userHistograms map[string]*prometheus.HistogramVec
}

// newMetricsBinding creates a new metrics binding
func newMetricsBinding(plugin *Plugin) *MetricsBinding {
	return &MetricsBinding{
		plugin:         plugin,
		userCounters:   make(map[string]*prometheus.CounterVec),
		userGauges:     make(map[string]*prometheus.GaugeVec),
		userHistograms: make(map[string]*prometheus.HistogramVec),
	}
}

// inject injects the metrics object into the VM
func (m *MetricsBinding) inject(vm *otto.Otto) error {
	metricsObj, err := vm.Object(`({})`)
	if err != nil {
		return err
	}

	// metrics.increment(name, labels)
	if err := metricsObj.Set("increment", m.increment); err != nil {
		return err
	}

	// metrics.gauge(name, value, labels)
	if err := metricsObj.Set("gauge", m.gauge); err != nil {
		return err
	}

	// metrics.histogram(name, value, labels)
	if err := metricsObj.Set("histogram", m.histogram); err != nil {
		return err
	}

	return vm.Set("metrics", metricsObj)
}

// increment increments a counter metric
func (m *MetricsBinding) increment(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) == 0 {
		return otto.UndefinedValue()
	}

	name := call.Argument(0).String()
	labels := m.extractLabels(call, 1)

	// Get or create counter
	counter := m.getOrCreateCounter(name, labels)
	if counter == nil {
		return otto.UndefinedValue()
	}

	counter.With(labels).Inc()
	return otto.UndefinedValue()
}

// gauge sets a gauge metric
func (m *MetricsBinding) gauge(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) < 2 {
		return otto.UndefinedValue()
	}

	name := call.Argument(0).String()
	value, err := call.Argument(1).ToFloat()
	if err != nil {
		return otto.UndefinedValue()
	}

	labels := m.extractLabels(call, 2)

	// Get or create gauge
	gauge := m.getOrCreateGauge(name, labels)
	if gauge == nil {
		return otto.UndefinedValue()
	}

	gauge.With(labels).Set(value)
	return otto.UndefinedValue()
}

// histogram observes a value in a histogram metric
func (m *MetricsBinding) histogram(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) < 2 {
		return otto.UndefinedValue()
	}

	name := call.Argument(0).String()
	value, err := call.Argument(1).ToFloat()
	if err != nil {
		return otto.UndefinedValue()
	}

	labels := m.extractLabels(call, 2)

	// Get or create histogram
	histogram := m.getOrCreateHistogram(name, labels)
	if histogram == nil {
		return otto.UndefinedValue()
	}

	histogram.With(labels).Observe(value)
	return otto.UndefinedValue()
}

// extractLabels extracts labels from the function call
func (m *MetricsBinding) extractLabels(call otto.FunctionCall, argIndex int) prometheus.Labels {
	if len(call.ArgumentList) <= argIndex {
		return prometheus.Labels{}
	}

	labelsValue := call.Argument(argIndex)
	if !labelsValue.IsObject() {
		return prometheus.Labels{}
	}

	labelsObj := labelsValue.Object()
	keys := labelsObj.Keys()

	labels := make(prometheus.Labels, len(keys))
	for _, key := range keys {
		value, err := labelsObj.Get(key)
		if err != nil {
			continue
		}
		labels[key] = value.String()
	}

	return labels
}

// getOrCreateCounter gets or creates a counter metric
func (m *MetricsBinding) getOrCreateCounter(name string, labels prometheus.Labels) *prometheus.CounterVec {
	// Check if counter already exists
	if counter, exists := m.userCounters[name]; exists {
		return counter
	}

	// Create label names slice
	labelNames := make([]string, 0, len(labels))
	for key := range labels {
		labelNames = append(labelNames, key)
	}

	// Create new counter
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "js_user",
			Name:      name,
			Help:      fmt.Sprintf("User-defined counter: %s", name),
		},
		labelNames,
	)

	// Register with Prometheus
	if err := prometheus.Register(counter); err != nil {
		// If already registered by another VM, try to get it
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
				m.userCounters[name] = existing
				return existing
			}
		}
		m.plugin.log.Error("failed to register counter", zap.String("name", name), zap.Error(err))
		return nil
	}

	m.userCounters[name] = counter
	return counter
}

// getOrCreateGauge gets or creates a gauge metric
func (m *MetricsBinding) getOrCreateGauge(name string, labels prometheus.Labels) *prometheus.GaugeVec {
	// Check if gauge already exists
	if gauge, exists := m.userGauges[name]; exists {
		return gauge
	}

	// Create label names slice
	labelNames := make([]string, 0, len(labels))
	for key := range labels {
		labelNames = append(labelNames, key)
	}

	// Create new gauge
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "js_user",
			Name:      name,
			Help:      fmt.Sprintf("User-defined gauge: %s", name),
		},
		labelNames,
	)

	// Register with Prometheus
	if err := prometheus.Register(gauge); err != nil {
		// If already registered by another VM, try to get it
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.GaugeVec); ok {
				m.userGauges[name] = existing
				return existing
			}
		}
		m.plugin.log.Error("failed to register gauge", zap.String("name", name), zap.Error(err))
		return nil
	}

	m.userGauges[name] = gauge
	return gauge
}

// getOrCreateHistogram gets or creates a histogram metric
func (m *MetricsBinding) getOrCreateHistogram(name string, labels prometheus.Labels) *prometheus.HistogramVec {
	// Check if histogram already exists
	if histogram, exists := m.userHistograms[name]; exists {
		return histogram
	}

	// Create label names slice
	labelNames := make([]string, 0, len(labels))
	for key := range labels {
		labelNames = append(labelNames, key)
	}

	// Create new histogram with sensible default buckets
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "js_user",
			Name:      name,
			Help:      fmt.Sprintf("User-defined histogram: %s", name),
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		labelNames,
	)

	// Register with Prometheus
	if err := prometheus.Register(histogram); err != nil {
		// If already registered by another VM, try to get it
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.HistogramVec); ok {
				m.userHistograms[name] = existing
				return existing
			}
		}
		m.plugin.log.Error("failed to register histogram", zap.String("name", name), zap.Error(err))
		return nil
	}

	m.userHistograms[name] = histogram
	return histogram
}
