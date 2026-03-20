package metrics

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

var initZerolog sync.Once

// LogLevel represents logging levels.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// Logger wraps zerolog for structured logging.
type Logger struct {
	logger zerolog.Logger
}

// NewLogger creates a new logger with the specified log level and output.
func NewLogger(level LogLevel, output io.Writer) *Logger {
	if output == nil {
		output = os.Stderr
	}

	// Configure zerolog globals exactly once (safe for concurrent test goroutines)
	initZerolog.Do(func() {
		zerolog.TimeFieldFormat = time.RFC3339
	})

	// Set log level
	zlevel := parseLogLevel(level)
	zerolog.SetGlobalLevel(zlevel)

	// Create logger with pretty output for console
	logger := zerolog.New(output).
		With().
		Timestamp().
		Caller().
		Logger()

	return &Logger{logger: logger}
}

// NewConsoleLogger creates a logger with pretty console output.
func NewConsoleLogger(level LogLevel) *Logger {
	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}
	return NewLogger(level, &output)
}

// NewJSONLogger creates a logger with JSON output.
func NewJSONLogger(level LogLevel) *Logger {
	return NewLogger(level, os.Stderr)
}

// parseLogLevel converts a LogLevel string to zerolog.Level.
func parseLogLevel(level LogLevel) zerolog.Level {
	switch level {
	case LogLevelDebug:
		return zerolog.DebugLevel
	case LogLevelInfo:
		return zerolog.InfoLevel
	case LogLevelWarn:
		return zerolog.WarnLevel
	case LogLevelError:
		return zerolog.ErrorLevel
	case LogLevelFatal:
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logger.Debug().Msgf(format, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logger.Info().Msgf(format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.logger.Warn().Msgf(format, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string) {
	l.logger.Error().Msg(msg)
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logger.Error().Msgf(format, args...)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string) {
	l.logger.Fatal().Msg(msg)
}

// Fatalf logs a formatted fatal message and exits.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logger.Fatal().Msgf(format, args...)
}

// WithField adds a field to the logger context.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		logger: l.logger.With().Interface(key, value).Logger(),
	}
}

// WithFields adds multiple fields to the logger context.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	context := l.logger.With()
	for key, value := range fields {
		context = context.Interface(key, value)
	}
	return &Logger{logger: context.Logger()}
}

// WithError adds an error to the logger context.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		logger: l.logger.With().Err(err).Logger(),
	}
}

// GetZerologLogger returns the underlying zerolog logger.
func (l *Logger) GetZerologLogger() zerolog.Logger {
	return l.logger
}

// Global logger instance
var defaultLogger *Logger

// init initializes the default logger.
func init() {
	defaultLogger = NewConsoleLogger(LogLevelInfo)
}

// SetDefaultLogger sets the global default logger.
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

// GetDefaultLogger returns the global default logger.
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// Debug logs a debug message using the default logger.
func Debug(msg string) {
	defaultLogger.Debug(msg)
}

// Debugf logs a formatted debug message using the default logger.
func Debugf(format string, args ...interface{}) {
	defaultLogger.Debugf(format, args...)
}

// Info logs an info message using the default logger.
func Info(msg string) {
	defaultLogger.Info(msg)
}

// Infof logs a formatted info message using the default logger.
func Infof(format string, args ...interface{}) {
	defaultLogger.Infof(format, args...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string) {
	defaultLogger.Warn(msg)
}

// Warnf logs a formatted warning message using the default logger.
func Warnf(format string, args ...interface{}) {
	defaultLogger.Warnf(format, args...)
}

// Error logs an error message using the default logger.
func Error(msg string) {
	defaultLogger.Error(msg)
}

// Errorf logs a formatted error message using the default logger.
func Errorf(format string, args ...interface{}) {
	defaultLogger.Errorf(format, args...)
}
