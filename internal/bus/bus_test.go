package bus

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceRoundTrip(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(context.Background(), "publish")
	defer span.End()

	msg := nats.NewMsg("hub.v1.entity.x.state")
	InjectTrace(ctx, msg)
	if msg.Header.Get("traceparent") == "" {
		t.Fatal("traceparent header not set")
	}

	got := trace.SpanContextFromContext(ExtractTrace(context.Background(), msg))
	if got.TraceID() != span.SpanContext().TraceID() {
		t.Fatalf("trace ID lost: got %s want %s", got.TraceID(), span.SpanContext().TraceID())
	}
}

func TestExtractWithoutHeaders(t *testing.T) {
	ctx := ExtractTrace(context.Background(), nats.NewMsg("s"))
	if trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("expected no span context")
	}
}

func TestSubjects(t *testing.T) {
	got, err := EntityState("light-1")
	if err != nil || got != "hub.v1.entity.light-1.state" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "a.b", "a*", ">", "a b"} {
		if _, err := DeviceCommand(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
