package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMetricsRecorder(t *testing.T) {
	recorder := NewMetricsRecorder()
	assert.NotNil(t, recorder)
	assert.True(t, recorder.IsEnabled())
	assert.Empty(t, recorder.GetMetrics())
}

func TestMetricsRecorderRecord(t *testing.T) {
	recorder := NewMetricsRecorder()

	labels := map[string]string{
		"workspace": "test",
		"provider":  "claude",
	}

	recorder.Record("mount.duration", 150.5, labels)

	metrics := recorder.GetMetrics()
	assert.Len(t, metrics, 1)

	metric := metrics[0]
	assert.Equal(t, "mount.duration", metric.Name)
	assert.Equal(t, 150.5, metric.Value)
	assert.Equal(t, labels, metric.Labels)
	assert.False(t, metric.Timestamp.IsZero())
}

func TestMetricsRecorderRecordDuration(t *testing.T) {
	recorder := NewMetricsRecorder()

	duration := 500 * time.Millisecond
	labels := map[string]string{"operation": "mount"}

	recorder.RecordDuration("operation.duration", duration, labels)

	metrics := recorder.GetMetrics()
	assert.Len(t, metrics, 1)

	metric := metrics[0]
	assert.Equal(t, "operation.duration", metric.Name)
	assert.Equal(t, float64(500), metric.Value)
}

func TestMetricsRecorderRecordCount(t *testing.T) {
	recorder := NewMetricsRecorder()

	count := 5
	labels := map[string]string{"type": "sources"}

	recorder.RecordCount("sources.count", count, labels)

	metrics := recorder.GetMetrics()
	assert.Len(t, metrics, 1)

	metric := metrics[0]
	assert.Equal(t, "sources.count", metric.Name)
	assert.Equal(t, float64(5), metric.Value)
}

func TestMetricsRecorderClear(t *testing.T) {
	recorder := NewMetricsRecorder()

	recorder.Record("test.metric", 100, nil)
	assert.Len(t, recorder.GetMetrics(), 1)

	recorder.Clear()
	assert.Empty(t, recorder.GetMetrics())
}

func TestMetricsRecorderEnableDisable(t *testing.T) {
	recorder := NewMetricsRecorder()

	assert.True(t, recorder.IsEnabled())

	recorder.Disable()
	assert.False(t, recorder.IsEnabled())

	// Recording when disabled should not add metrics
	recorder.Record("test.metric", 100, nil)
	assert.Empty(t, recorder.GetMetrics())

	recorder.Enable()
	assert.True(t, recorder.IsEnabled())

	// Recording when enabled should work
	recorder.Record("test.metric", 100, nil)
	assert.Len(t, recorder.GetMetrics(), 1)
}

func TestTimer(t *testing.T) {
	recorder := NewMetricsRecorder()
	logger := NewJSONLogger(LogLevelDebug)

	labels := map[string]string{"operation": "test"}
	timer := NewTimer("test.operation", labels, recorder, logger)

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	duration := timer.Stop()

	assert.True(t, duration >= 10*time.Millisecond)

	metrics := recorder.GetMetrics()
	assert.Len(t, metrics, 1)

	metric := metrics[0]
	assert.Equal(t, "test.operation", metric.Name)
	assert.True(t, metric.Value >= 10) // At least 10ms
}

func TestTimerWithoutRecorderOrLogger(t *testing.T) {
	timer := NewTimer("test.operation", nil, nil, nil)

	// Should not panic
	duration := timer.Stop()
	assert.True(t, duration >= 0)
}

func TestDefaultRecorder(t *testing.T) {
	recorder := GetDefaultRecorder()
	assert.NotNil(t, recorder)

	newRecorder := NewMetricsRecorder()
	SetDefaultRecorder(newRecorder)

	assert.Equal(t, newRecorder, GetDefaultRecorder())
}

func TestGlobalMetricsFunctions(t *testing.T) {
	// Set up a test recorder
	testRecorder := NewMetricsRecorder()
	SetDefaultRecorder(testRecorder)

	t.Run("record", func(t *testing.T) {
		testRecorder.Clear()
		Record("test.metric", 100, map[string]string{"label": "value"})

		metrics := testRecorder.GetMetrics()
		assert.Len(t, metrics, 1)
		assert.Equal(t, "test.metric", metrics[0].Name)
	})

	t.Run("record duration", func(t *testing.T) {
		testRecorder.Clear()
		duration := 250 * time.Millisecond
		RecordDuration("test.duration", duration, map[string]string{"op": "test"})

		metrics := testRecorder.GetMetrics()
		assert.Len(t, metrics, 1)
		assert.Equal(t, "test.duration", metrics[0].Name)
		assert.Equal(t, float64(250), metrics[0].Value)
	})

	t.Run("record count", func(t *testing.T) {
		testRecorder.Clear()
		RecordCount("test.count", 10, map[string]string{"type": "items"})

		metrics := testRecorder.GetMetrics()
		assert.Len(t, metrics, 1)
		assert.Equal(t, "test.count", metrics[0].Name)
		assert.Equal(t, float64(10), metrics[0].Value)
	})

	t.Run("start timer", func(t *testing.T) {
		testRecorder.Clear()
		timer := StartTimer("test.timer", map[string]string{"op": "test"})

		time.Sleep(5 * time.Millisecond)
		duration := timer.Stop()

		assert.True(t, duration >= 5*time.Millisecond)

		metrics := testRecorder.GetMetrics()
		assert.Len(t, metrics, 1)
		assert.Equal(t, "test.timer", metrics[0].Name)
	})
}
