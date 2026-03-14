package metrics

import (
	"sync"
	"time"
)

// Metric represents a recorded metric.
type Metric struct {
	Name      string
	Value     float64
	Timestamp time.Time
	Labels    map[string]string
}

// MetricsRecorder records metrics for instrumentation.
type MetricsRecorder struct {
	mu      sync.RWMutex
	metrics []Metric
	enabled bool
}

// NewMetricsRecorder creates a new metrics recorder.
func NewMetricsRecorder() *MetricsRecorder {
	return &MetricsRecorder{
		metrics: make([]Metric, 0),
		enabled: true,
	}
}

// Record records a metric.
func (m *MetricsRecorder) Record(name string, value float64, labels map[string]string) {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	metric := Metric{
		Name:      name,
		Value:     value,
		Timestamp: time.Now(),
		Labels:    labels,
	}

	m.metrics = append(m.metrics, metric)
}

// RecordDuration records a duration metric in milliseconds.
func (m *MetricsRecorder) RecordDuration(name string, duration time.Duration, labels map[string]string) {
	m.Record(name, float64(duration.Milliseconds()), labels)
}

// RecordCount records a count metric.
func (m *MetricsRecorder) RecordCount(name string, count int, labels map[string]string) {
	m.Record(name, float64(count), labels)
}

// GetMetrics returns all recorded metrics.
func (m *MetricsRecorder) GetMetrics() []Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	metrics := make([]Metric, len(m.metrics))
	copy(metrics, m.metrics)
	return metrics
}

// Clear clears all recorded metrics.
func (m *MetricsRecorder) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = make([]Metric, 0)
}

// Enable enables metrics recording.
func (m *MetricsRecorder) Enable() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = true
}

// Disable disables metrics recording.
func (m *MetricsRecorder) Disable() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = false
}

// IsEnabled returns true if metrics recording is enabled.
func (m *MetricsRecorder) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.enabled
}

// Timer is a utility for timing operations.
type Timer struct {
	name     string
	start    time.Time
	labels   map[string]string
	recorder *MetricsRecorder
	logger   *Logger
}

// NewTimer creates a new timer.
func NewTimer(name string, labels map[string]string, recorder *MetricsRecorder, logger *Logger) *Timer {
	return &Timer{
		name:     name,
		start:    time.Now(),
		labels:   labels,
		recorder: recorder,
		logger:   logger,
	}
}

// Stop stops the timer and records the duration.
func (t *Timer) Stop() time.Duration {
	duration := time.Since(t.start)

	if t.recorder != nil {
		t.recorder.RecordDuration(t.name, duration, t.labels)
	}

	if t.logger != nil {
		t.logger.WithFields(map[string]interface{}{
			"operation": t.name,
			"duration":  duration.Milliseconds(),
			"labels":    t.labels,
		}).Debugf("%s completed in %dms", t.name, duration.Milliseconds())
	}

	return duration
}

// Global metrics recorder
var defaultRecorder *MetricsRecorder

// init initializes the default metrics recorder.
func init() {
	defaultRecorder = NewMetricsRecorder()
}

// SetDefaultRecorder sets the global default metrics recorder.
func SetDefaultRecorder(recorder *MetricsRecorder) {
	defaultRecorder = recorder
}

// GetDefaultRecorder returns the global default metrics recorder.
func GetDefaultRecorder() *MetricsRecorder {
	return defaultRecorder
}

// Record records a metric using the default recorder.
func Record(name string, value float64, labels map[string]string) {
	defaultRecorder.Record(name, value, labels)
}

// RecordDuration records a duration metric using the default recorder.
func RecordDuration(name string, duration time.Duration, labels map[string]string) {
	defaultRecorder.RecordDuration(name, duration, labels)
}

// RecordCount records a count metric using the default recorder.
func RecordCount(name string, count int, labels map[string]string) {
	defaultRecorder.RecordCount(name, count, labels)
}

// StartTimer starts a timer using the default recorder and logger.
func StartTimer(name string, labels map[string]string) *Timer {
	return NewTimer(name, labels, defaultRecorder, defaultLogger)
}
