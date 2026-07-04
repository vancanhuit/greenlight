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
