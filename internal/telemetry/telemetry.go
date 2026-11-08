// Package telemetry sets up structured logs, metrics and traces.
//
// Logs go to stdout through slog. Metrics are exposed in Prometheus format
// through an OpenTelemetry meter provider. Traces are exported over OTLP/gRPC
// when an endpoint is configured, and the W3C trace context is the propagator
// used for HTTP and NATS headers.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/xonoko/robust-home-automation/internal/config"
	"github.com/xonoko/robust-home-automation/internal/version"
)

// ServiceName identifies the hub in logs, metrics and traces.
const ServiceName = "hub"

// Telemetry owns the logger and the OpenTelemetry providers.
type Telemetry struct {
	Logger         *slog.Logger
	MetricsHandler http.Handler

	shutdowns []func(context.Context) error
}

// Setup configures logging, metrics and tracing, and installs the providers globally.
func Setup(ctx context.Context, cfg config.Telemetry, nodeID string, out io.Writer) (*Telemetry, error) {
	logger, err := newLogger(cfg, nodeID, out)
	if err != nil {
		return nil, err
	}
	t := &Telemetry{Logger: logger}

	res := resource.NewSchemaless(
		attribute.String("service.name", ServiceName),
		attribute.String("service.version", version.Version),
		attribute.String("service.instance.id", nodeID),
	)

	// Metrics: a dedicated registry, so only our metrics and the Go runtime are exposed.
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	exporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter), sdkmetric.WithResource(res))
	otel.SetMeterProvider(meterProvider)
	t.shutdowns = append(t.shutdowns, meterProvider.Shutdown)
	t.MetricsHandler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry})

	// Traces: spans are always created (so trace IDs appear in logs and headers),
	// and only exported when an OTLP endpoint is set.
	traceOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}
	if cfg.OTLPEndpoint != "" {
		traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(cfg.OTLPEndpoint))
		if err != nil {
			return nil, fmt.Errorf("otlp trace exporter: %w", err)
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(traceExporter))
	}
	tracerProvider := sdktrace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tracerProvider)
	t.shutdowns = append(t.shutdowns, tracerProvider.Shutdown)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("opentelemetry error", "error", err)
	}))

	return t, nil
}

// Shutdown flushes pending spans and metrics.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for i := len(t.shutdowns) - 1; i >= 0; i-- {
		errs = append(errs, t.shutdowns[i](ctx))
	}
	return errors.Join(errs...)
}

func newLogger(cfg config.Telemetry, nodeID string, out io.Writer) (*slog.Logger, error) {
	level, err := config.ParseLogLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if cfg.LogFormat == "text" {
		h = slog.NewTextHandler(out, opts)
	} else {
		h = slog.NewJSONHandler(out, opts)
	}
	return slog.New(h).With("service", ServiceName, "node", nodeID, "version", version.Version), nil
}
