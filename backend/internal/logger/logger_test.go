package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func newBufLogger(buf *bytes.Buffer, level slog.Level) *SlogLogger {
	return New(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})))
}

func decodeOne(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	require.NotEmpty(t, buf.Bytes(), "logger wrote nothing")
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	return m
}

func TestSlogLogger_Info(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := newBufLogger(buf, slog.LevelInfo)

	l.Info(context.Background(), "hello", "k1", "v1", "k2", 42)

	entry := decodeOne(t, buf)
	require.Equal(t, "INFO", entry["level"])
	require.Equal(t, "hello", entry["msg"])
	require.Equal(t, "v1", entry["k1"])
	require.Equal(t, float64(42), entry["k2"])
}

func TestSlogLogger_Debug(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := newBufLogger(buf, slog.LevelDebug)

	l.Debug(context.Background(), "debug-msg", "attr", "value")

	entry := decodeOne(t, buf)
	require.Equal(t, "DEBUG", entry["level"])
	require.Equal(t, "debug-msg", entry["msg"])
	require.Equal(t, "value", entry["attr"])
}

func TestSlogLogger_Warn(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := newBufLogger(buf, slog.LevelInfo)

	l.Warn(context.Background(), "careful", "count", 3)

	entry := decodeOne(t, buf)
	require.Equal(t, "WARN", entry["level"])
	require.Equal(t, "careful", entry["msg"])
	require.Equal(t, float64(3), entry["count"])
}

func TestSlogLogger_Error(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := newBufLogger(buf, slog.LevelInfo)

	l.Error(context.Background(), "boom", "error", "bang")

	entry := decodeOne(t, buf)
	require.Equal(t, "ERROR", entry["level"])
	require.Equal(t, "boom", entry["msg"])
	require.Equal(t, "bang", entry["error"])
}

func TestSlogLogger_AttrsFormat(t *testing.T) {
	t.Parallel()

	bufDirect := &bytes.Buffer{}
	bufWrapped := &bytes.Buffer{}

	direct := slog.New(slog.NewJSONHandler(bufDirect, &slog.HandlerOptions{Level: slog.LevelInfo}))
	wrapped := New(slog.New(slog.NewJSONHandler(bufWrapped, &slog.HandlerOptions{Level: slog.LevelInfo})))

	direct.Info("msg", "k1", "v1", "k2", 42)
	wrapped.Info(context.Background(), "msg", "k1", "v1", "k2", 42)

	stripTime := func(b []byte) map[string]any {
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		delete(m, "time")
		return m
	}
	require.Equal(t, stripTime(bufDirect.Bytes()), stripTime(bufWrapped.Bytes()))
}

func TestSlogLogger_With(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := newBufLogger(buf, slog.LevelInfo)

	derived := l.With("component", "auth")
	derived.Info(context.Background(), "login")

	entry := decodeOne(t, buf)
	require.Equal(t, "login", entry["msg"])
	require.Equal(t, "auth", entry["component"])
}

func TestSlogLogger_WithDoesNotMutate(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := newBufLogger(buf, slog.LevelInfo)

	_ = l.With("component", "auth")
	l.Info(context.Background(), "plain")

	entry := decodeOne(t, buf)
	require.Equal(t, "plain", entry["msg"])
	_, hasComponent := entry["component"]
	require.False(t, hasComponent, "original logger must not carry derived attrs")
}

func TestSlogLogger_IgnoresContext(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := newBufLogger(buf, slog.LevelInfo)

	require.NotPanics(t, func() {
		l.Info(context.Background(), "bg")
	})

	var nilCtx context.Context //nolint:staticcheck // deliberately nil for contract test
	require.NotPanics(t, func() {
		l.Info(nilCtx, "nil-ctx")
	})
}
