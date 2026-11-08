package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func get(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	var body map[string]any
	if rec.Header().Get("Content-Type") == "application/json" {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return rec, body
}

func ok(context.Context) error   { return nil }
func fail(context.Context) error { return errors.New("down") }

func TestLiveness(t *testing.T) {
	rec, body := get(t, New(Options{}).Handler(), "/healthz")
	if rec.Code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name   string
		checks []Check
		want   int
	}{
		{"no checks", nil, http.StatusOK},
		{"all ok", []Check{{Name: "nats", Critical: true, Fn: ok}}, http.StatusOK},
		{"critical down", []Check{{Name: "nats", Critical: true, Fn: fail}}, http.StatusServiceUnavailable},
		{"non-critical down", []Check{{Name: "nats", Critical: true, Fn: ok}, {Name: "postgres", Fn: fail}}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := get(t, New(Options{Checks: tt.checks}).Handler(), "/readyz")
			if rec.Code != tt.want {
				t.Fatalf("got %d want %d: %v", rec.Code, tt.want, body)
			}
		})
	}
}

func TestReadinessWhileShuttingDown(t *testing.T) {
	s := New(Options{Checks: []Check{{Name: "nats", Critical: true, Fn: ok}}})
	s.SetShuttingDown()
	rec, body := get(t, s.Handler(), "/readyz")
	if rec.Code != http.StatusServiceUnavailable || body["shuttingDown"] != true {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}

func TestVersionAndOpenAPI(t *testing.T) {
	s := New(Options{})
	rec, body := get(t, s.Handler(), "/api/v1/version")
	if rec.Code != http.StatusOK || body["version"] == nil {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	for _, p := range []string{"/healthz", "/readyz", "/api/v1/version"} {
		if s.OpenAPI().Paths[p] == nil {
			t.Errorf("OpenAPI document misses %s", p)
		}
	}
}

func TestMetricsAndUIRoutes(t *testing.T) {
	marker := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(name)) })
	}
	h := New(Options{Metrics: marker("metrics"), UI: marker("ui")}).Handler()
	for path, want := range map[string]string{"/metrics": "metrics", "/": "ui", "/rooms/kitchen": "ui"} {
		rec, _ := get(t, h, path)
		if rec.Body.String() != want {
			t.Errorf("%s: got %q want %q", path, rec.Body.String(), want)
		}
	}
}
