package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

type Logger struct {
	level Level
	slog  *slog.Logger
}

func New(level string) (*Logger, error) {
	return NewWithFormat(level, "text")
}

// NewWithFormat builds a logger with the given level and output format
// ("text" or "json").
func NewWithFormat(level, format string) (*Logger, error) {
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: slogLevel(parsed)}
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("invalid log format %q", format)
	}
	return &Logger{
		level: parsed,
		slog:  slog.New(handler),
	}, nil
}

func Must(level string) *Logger {
	logger, err := New(level)
	if err != nil {
		panic(err)
	}
	return logger
}

func ParseLevel(value string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return LevelInfo, nil
	case "debug":
		return LevelDebug, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("invalid log level %q", value)
	}
}

func NormalizeLevel(value string) (string, error) {
	level, err := ParseLevel(value)
	if err != nil {
		return "", err
	}
	switch level {
	case LevelDebug:
		return "debug", nil
	case LevelInfo:
		return "info", nil
	case LevelWarn:
		return "warn", nil
	case LevelError:
		return "error", nil
	default:
		return "info", nil
	}
}

func slogLevel(level Level) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (l *Logger) Debugf(format string, args ...any) {
	l.logf(LevelDebug, format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.logf(LevelInfo, format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.logf(LevelWarn, format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.logf(LevelError, format, args...)
}

func (l *Logger) logf(level Level, format string, args ...any) {
	if l == nil || l.slog == nil {
		return
	}
	if level < l.level {
		return
	}
	l.slog.Log(context.Background(), slogLevel(level), fmt.Sprintf(format, args...))
}
