package bus

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// HeaderCarrier adapts NATS message headers to OpenTelemetry propagation.
type HeaderCarrier nats.Header

var _ propagation.TextMapCarrier = HeaderCarrier{}

// Get returns the first value of a header.
func (h HeaderCarrier) Get(key string) string { return nats.Header(h).Get(key) }

// Set replaces a header value.
func (h HeaderCarrier) Set(key, value string) { nats.Header(h).Set(key, value) }

// Keys lists the header names.
func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}

// InjectTrace writes the trace context of ctx into msg's headers.
func InjectTrace(ctx context.Context, msg *nats.Msg) {
	if msg.Header == nil {
		msg.Header = nats.Header{}
	}
	otel.GetTextMapPropagator().Inject(ctx, HeaderCarrier(msg.Header))
}

// ExtractTrace returns ctx enriched with the trace context found in msg's headers.
func ExtractTrace(ctx context.Context, msg *nats.Msg) context.Context {
	if msg.Header == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, HeaderCarrier(msg.Header))
}
