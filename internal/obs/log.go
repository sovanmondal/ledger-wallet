// Package obs holds observability primitives: structured logging and Prometheus metrics.
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
)

type ctxKey int

const (
	loggerKey ctxKey = iota
	corrKey
)

// NewLogger returns a JSON slog logger writing to stdout at the given level.
func NewLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

// NewCorrelationID returns a random 128-bit hex id.
func NewCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// WithLogger stores a request-scoped logger in the context.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// WithCorrelationID stores the correlation id in the context.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, corrKey, id)
}

// CorrelationID returns the correlation id from the context, or "" if absent.
func CorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(corrKey).(string); ok {
		return v
	}
	return ""
}

// L returns the request-scoped logger, falling back to the default logger.
func L(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
