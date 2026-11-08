// Package api serves the hub's HTTP API, health endpoints, metrics and web UI.
//
// Endpoints are declared with Huma, which generates the OpenAPI document the
// web UI's TypeScript client is built from (`hub openapi`).
package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/xonoko/robust-home-automation/internal/version"
)

// Title of the OpenAPI document.
const Title = "Robust Home Automation API"

// checkTimeout bounds each readiness check.
const checkTimeout = 2 * time.Second

func init() {
	// Handlers never return nil slices, so arrays are not nullable in the
	// OpenAPI document and the generated TypeScript types stay exact.
	huma.DefaultArrayNullable = false
}

// Check is a dependency checked by the readiness endpoint. A failing critical
// check makes the node not ready; a failing non-critical one is only reported.
type Check struct {
	Name     string
	Critical bool
	Fn       func(context.Context) error
}

// Options configures the server. Every field is optional.
type Options struct {
	Checks  []Check
	Metrics http.Handler
	UI      http.Handler
}

// Server holds the HTTP routes.
type Server struct {
	mux          *http.ServeMux
	api          huma.API
	checks       []Check
	shuttingDown atomic.Bool
}

// New builds the server and registers every route.
func New(opts Options) *Server {
	mux := http.NewServeMux()
	cfg := huma.DefaultConfig(Title, version.Version)
	cfg.Info.Description = "HTTP API of the Robust Home Automation hub."

	s := &Server{mux: mux, api: humago.New(mux, cfg), checks: opts.Checks}
	s.registerSystem()

	if opts.Metrics != nil {
		mux.Handle("GET /metrics", opts.Metrics)
	}
	if opts.UI != nil {
		mux.Handle("GET /", opts.UI)
	}
	return s
}

// Handler returns the root handler, instrumented with OpenTelemetry.
// Health and metrics requests are not traced.
func (s *Server) Handler() http.Handler {
	return otelhttp.NewHandler(s.mux, "hub.http",
		otelhttp.WithFilter(func(r *http.Request) bool {
			switch r.URL.Path {
			case "/healthz", "/readyz", "/metrics":
				return false
			}
			return true
		}),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
}

// OpenAPI returns the generated OpenAPI document.
func (s *Server) OpenAPI() *huma.OpenAPI { return s.api.OpenAPI() }

// SetShuttingDown makes the readiness endpoint fail, so the node stops
// receiving new work while it drains.
func (s *Server) SetShuttingDown() { s.shuttingDown.Store(true) }

// CheckResult is the outcome of one readiness check.
type CheckResult struct {
	Name     string `json:"name"`
	Critical bool   `json:"critical" doc:"A failing critical check makes the node not ready"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}

// Readiness is the body of the readiness endpoint.
type Readiness struct {
	Ready        bool          `json:"ready"`
	ShuttingDown bool          `json:"shuttingDown"`
	Checks       []CheckResult `json:"checks"`
}

type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

type readinessOutput struct {
	Status int
	Body   Readiness
}

type versionOutput struct {
	Body version.Info
}

func (s *Server) registerSystem() {
	huma.Register(s.api, huma.Operation{
		OperationID: "get-liveness",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Liveness",
		Description: "Returns 200 while the process is running.",
		Tags:        []string{"System"},
	}, func(context.Context, *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-readiness",
		Method:      http.MethodGet,
		Path:        "/readyz",
		Summary:     "Readiness",
		Description: "Returns 200 when every critical dependency is available, 503 otherwise or while the node is shutting down.",
		Tags:        []string{"System"},
		Responses: map[string]*huma.Response{
			"503": {Description: "Not ready"},
		},
	}, func(ctx context.Context, _ *struct{}) (*readinessOutput, error) {
		body := s.readiness(ctx)
		status := http.StatusOK
		if !body.Ready {
			status = http.StatusServiceUnavailable
		}
		return &readinessOutput{Status: status, Body: body}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-version",
		Method:      http.MethodGet,
		Path:        "/api/v1/version",
		Summary:     "Build information",
		Tags:        []string{"System"},
	}, func(context.Context, *struct{}) (*versionOutput, error) {
		return &versionOutput{Body: version.Get()}, nil
	})
}

// readiness runs every check concurrently.
func (s *Server) readiness(ctx context.Context) Readiness {
	results := make([]CheckResult, len(s.checks))
	var wg sync.WaitGroup
	for i, c := range s.checks {
		wg.Go(func() {
			cctx, cancel := context.WithTimeout(ctx, checkTimeout)
			defer cancel()
			r := CheckResult{Name: c.Name, Critical: c.Critical, OK: true}
			if err := c.Fn(cctx); err != nil {
				r.OK = false
				r.Error = err.Error()
			}
			results[i] = r
		})
	}
	wg.Wait()

	shuttingDown := s.shuttingDown.Load()
	ready := !shuttingDown
	for _, r := range results {
		if r.Critical && !r.OK {
			ready = false
		}
	}
	return Readiness{Ready: ready, ShuttingDown: shuttingDown, Checks: results}
}

// IsAPIPath reports whether a path belongs to the JSON API rather than the UI.
func IsAPIPath(p string) bool { return strings.HasPrefix(p, "/api/") }
