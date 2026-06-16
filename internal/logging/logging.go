package logging

import (
	"fmt"
	stdlog "log"
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
	level  Level
	stdlib *stdlog.Logger
}

func New(level string) (*Logger, error) {
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	return &Logger{
		level:  parsed,
		stdlib: stdlog.New(os.Stdout, "", stdlog.LstdFlags),
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

func (l *Logger) Debugf(format string, args ...any) {
	l.logf(LevelDebug, "DEBUG", format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.logf(LevelInfo, "INFO", format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.logf(LevelWarn, "WARN", format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.logf(LevelError, "ERROR", format, args...)
}

func (l *Logger) logf(level Level, label, format string, args ...any) {
	if l == nil || l.stdlib == nil {
		return
	}
	if level < l.level {
		return
	}
	l.stdlib.Printf("[%s] %s", label, fmt.Sprintf(format, args...))
}
