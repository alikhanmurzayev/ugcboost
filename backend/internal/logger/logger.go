// Package logger wraps log/slog so every layer can depend on an interface
// and be unit-tested with a mock.
package logger

import (
	"context"
	"log/slog"
)

// Logger is the logging interface injected into every backend layer.
type Logger interface {
	Debug(ctx context.Context, msg string, args ...any)
	Info(ctx context.Context, msg string, args ...any)
	Warn(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
	With(args ...any) Logger
}

// SlogLogger is the production Logger backed by *slog.Logger.
type SlogLogger struct {
	inner *slog.Logger
}

// New wraps inner as a Logger.
func New(inner *slog.Logger) *SlogLogger {
	return &SlogLogger{inner: inner}
}

func (l *SlogLogger) Debug(_ context.Context, msg string, args ...any) {
	l.inner.Debug(msg, args...)
}

func (l *SlogLogger) Info(_ context.Context, msg string, args ...any) {
	l.inner.Info(msg, args...)
}

func (l *SlogLogger) Warn(_ context.Context, msg string, args ...any) {
	l.inner.Warn(msg, args...)
}

func (l *SlogLogger) Error(_ context.Context, msg string, args ...any) {
	l.inner.Error(msg, args...)
}

// With returns a derived Logger with the given attrs prepended.
// It does not mutate the receiver.
func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{inner: l.inner.With(args...)}
}
