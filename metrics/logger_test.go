package metrics

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelInfo, &buf)
	assert.NotNil(t, logger)

	logger.Info("test message")
	assert.Contains(t, buf.String(), "test message")
}

func TestNewConsoleLogger(t *testing.T) {
	logger := NewConsoleLogger(LogLevelDebug)
	assert.NotNil(t, logger)
}

func TestNewJSONLogger(t *testing.T) {
	logger := NewJSONLogger(LogLevelInfo)
	assert.NotNil(t, logger)
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level LogLevel
	}{
		{"debug", LogLevelDebug},
		{"info", LogLevelInfo},
		{"warn", LogLevelWarn},
		{"error", LogLevelError},
		{"fatal", LogLevelFatal},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic
			_ = parseLogLevel(tt.level)
		})
	}
}

func TestLoggerMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)

	t.Run("debug", func(t *testing.T) {
		buf.Reset()
		logger.Debug("debug message")
		assert.Contains(t, buf.String(), "debug message")
	})

	t.Run("debugf", func(t *testing.T) {
		buf.Reset()
		logger.Debugf("debug %s", "formatted")
		assert.Contains(t, buf.String(), "debug formatted")
	})

	t.Run("info", func(t *testing.T) {
		buf.Reset()
		logger.Info("info message")
		assert.Contains(t, buf.String(), "info message")
	})

	t.Run("infof", func(t *testing.T) {
		buf.Reset()
		logger.Infof("info %s", "formatted")
		assert.Contains(t, buf.String(), "info formatted")
	})

	t.Run("warn", func(t *testing.T) {
		buf.Reset()
		logger.Warn("warn message")
		assert.Contains(t, buf.String(), "warn message")
	})

	t.Run("warnf", func(t *testing.T) {
		buf.Reset()
		logger.Warnf("warn %s", "formatted")
		assert.Contains(t, buf.String(), "warn formatted")
	})

	t.Run("error", func(t *testing.T) {
		buf.Reset()
		logger.Error("error message")
		assert.Contains(t, buf.String(), "error message")
	})

	t.Run("errorf", func(t *testing.T) {
		buf.Reset()
		logger.Errorf("error %s", "formatted")
		assert.Contains(t, buf.String(), "error formatted")
	})
}

func TestLoggerWithField(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelInfo, &buf)

	contextLogger := logger.WithField("key", "value")
	contextLogger.Info("message")

	output := buf.String()
	assert.Contains(t, output, "message")
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
}

func TestLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelInfo, &buf)

	fields := map[string]interface{}{
		"field1": "value1",
		"field2": 42,
	}

	contextLogger := logger.WithFields(fields)
	contextLogger.Info("message")

	output := buf.String()
	assert.Contains(t, output, "message")
	assert.Contains(t, output, "field1")
	assert.Contains(t, output, "field2")
}

func TestLoggerWithError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelInfo, &buf)

	err := assert.AnError
	contextLogger := logger.WithError(err)
	contextLogger.Error("error occurred")

	output := buf.String()
	assert.Contains(t, output, "error occurred")
	assert.Contains(t, output, "error")
}

func TestDefaultLogger(t *testing.T) {
	logger := GetDefaultLogger()
	assert.NotNil(t, logger)

	newLogger := NewConsoleLogger(LogLevelDebug)
	SetDefaultLogger(newLogger)

	assert.Equal(t, newLogger, GetDefaultLogger())
}

func TestGlobalLogFunctions(t *testing.T) {
	// Set up a test logger
	var buf bytes.Buffer
	testLogger := NewLogger(LogLevelDebug, &buf)
	SetDefaultLogger(testLogger)

	t.Run("debug", func(t *testing.T) {
		buf.Reset()
		Debug("debug message")
		assert.Contains(t, buf.String(), "debug message")
	})

	t.Run("debugf", func(t *testing.T) {
		buf.Reset()
		Debugf("debug %s", "formatted")
		assert.Contains(t, buf.String(), "debug formatted")
	})

	t.Run("info", func(t *testing.T) {
		buf.Reset()
		Info("info message")
		assert.Contains(t, buf.String(), "info message")
	})

	t.Run("infof", func(t *testing.T) {
		buf.Reset()
		Infof("info %s", "formatted")
		assert.Contains(t, buf.String(), "info formatted")
	})

	t.Run("warn", func(t *testing.T) {
		buf.Reset()
		Warn("warn message")
		assert.Contains(t, buf.String(), "warn message")
	})

	t.Run("warnf", func(t *testing.T) {
		buf.Reset()
		Warnf("warn %s", "formatted")
		assert.Contains(t, buf.String(), "warn formatted")
	})

	t.Run("error", func(t *testing.T) {
		buf.Reset()
		Error("error message")
		assert.Contains(t, buf.String(), "error message")
	})

	t.Run("errorf", func(t *testing.T) {
		buf.Reset()
		Errorf("error %s", "formatted")
		assert.Contains(t, buf.String(), "error formatted")
	})
}
