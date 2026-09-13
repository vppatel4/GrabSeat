// Package observability wires up two things that let us follow a single booking
// attempt end to end:
//
//  1. Structured JSON logging (slog) where every log line carries the trace_id
//     of the request that produced it.
//  2. OpenTelemetry spans, so the lock-acquisition and confirm code paths show
//     up as real timed spans we can point at and say "this is where the time
//     went during the load test".
//
// The trace_id in the logs is the same id as the OTel trace, so a log line and
// a span can always be lined up against each other.
package observability

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type ctxKey int

const loggerKey ctxKey = iota

// Tracer is the single tracer the whole service uses.
var Tracer = otel.Tracer("grabseat")

// InitLogger returns a JSON logger at the requested level and installs it as the
// process default.
func InitLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
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
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

// InitTracer sets up the OpenTelemetry trace provider. "stdout" prints spans to
// the logs (zero external dependencies, which keeps the project signup-free);
// "none" installs a no-op so tracing can be turned off without code changes.
func InitTracer(ctx context.Context, exporter, instanceID string) (func(context.Context) error, error) {
	if exporter == "none" || exporter == "" {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := stdouttrace.New(
		stdouttrace.WithWriter(os.Stdout),
		// One line per span keeps the container logs readable during a load test.
		stdouttrace.WithoutTimestamps(),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("grabseat-backend"),
		semconv.ServiceInstanceID(instanceID),
	))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	Tracer = tp.Tracer("grabseat")
	return tp.Shutdown, nil
}

// WithLogger stores a request-scoped logger in the context.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// LoggerFromContext returns the request-scoped logger, or the default logger if
// none was attached. It never returns nil, so callers can log unconditionally.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// TraceIDFromContext pulls the current trace id out of the active span, or ""
// if there is no recording span.
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}
