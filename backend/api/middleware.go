package api

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"log/slog"

	"grabseat/observability"
)

// statusRecorder captures the status code while preserving the ability to
// hijack the connection (needed for the WebSocket upgrade).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// traceMiddleware opens a span for every request, attaches a logger carrying
// that request's trace_id to the context, echoes the id back in X-Trace-Id, and
// logs one structured line per request. This is what lets a single booking
// attempt be followed from HTTP entry all the way to the Postgres write.
func (s *Server) traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := observability.Tracer.Start(r.Context(), r.Method+" "+r.URL.Path)
		defer span.End()

		traceID := observability.TraceIDFromContext(ctx)
		reqLog := slog.Default().With(
			"trace_id", traceID,
			"method", r.Method,
			"path", r.URL.Path,
			"session", r.Header.Get("X-Session-Id"),
		)
		ctx = observability.WithLogger(ctx, reqLog)

		if traceID != "" {
			w.Header().Set("X-Trace-Id", traceID)
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(ctx))

		// WebSocket upgrades are long-lived; don't log a misleading duration.
		if r.URL.Path != "/ws" {
			reqLog.Info("request",
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.AllowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "X-Trace-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
