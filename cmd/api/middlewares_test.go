package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
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
