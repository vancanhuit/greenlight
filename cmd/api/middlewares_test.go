package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"go.uber.org/goleak"
)

func TestEnableCORSSetsBothVaryHeaders(t *testing.T) {
	app := &application{}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	app.enableCORS(next).ServeHTTP(rr, req)

	vary := rr.Result().Header.Values("Vary")

	if !slices.Contains(vary, "Origin") {
		t.Errorf("expected Vary to contain %q, got %v", "Origin", vary)
	}
	if !slices.Contains(vary, "Access-Control-Request-Method") {
		t.Errorf("expected Vary to contain %q, got %v", "Access-Control-Request-Method", vary)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		trustProxy bool
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "no trust uses remote addr", trustProxy: false, remoteAddr: "203.0.113.9:5555", forwarded: "70.41.3.18", want: "203.0.113.9"},
		{name: "trust uses forwarded header", trustProxy: true, remoteAddr: "203.0.113.9:5555", forwarded: "70.41.3.18", want: "70.41.3.18"},
		{name: "no trust ignores forwarded", trustProxy: false, remoteAddr: "198.51.100.7:443", forwarded: "70.41.3.18", want: "198.51.100.7"},
		{name: "no trust malformed remote addr falls back", trustProxy: false, remoteAddr: "garbage-no-port", forwarded: "", want: "garbage-no-port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &application{}
			app.config.tls.trustProxy = tt.trustProxy

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			if got := app.clientIP(req); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRateLimitExceeded confirms the per-IP limiter denies a second request
// that exceeds the configured burst, returning 429 for the same client IP.
func TestRateLimitExceeded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	app := &application{logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), shutdownCtx: ctx}
	app.config.limiter.enabled = true
	app.config.limiter.rps = 1
	app.config.limiter.burst = 1

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := app.rateLimit(next)

	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.10:12345"
		return req
	}

	first := httptest.NewRecorder()
	h.ServeHTTP(first, newReq())
	if first.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want %d", first.Code, http.StatusOK)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, newReq())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

// TestRateLimitCleanupStopsOnShutdown verifies the per-limiter cleanup goroutine
// terminates when the app's shutdown context is cancelled, rather than leaking
// for the lifetime of the process.
func TestRateLimitCleanupStopsOnShutdown(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	app := &application{shutdownCtx: ctx}
	app.config.limiter.enabled = true
	app.config.limiter.rps = 1
	app.config.limiter.burst = 1

	// Constructing the middleware spawns the cleanup goroutine.
	_ = app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	// Cancelling the shutdown context must terminate that goroutine.
	cancel()
}
