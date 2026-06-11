package objstore

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gotd/teled/internal/obs"
)

const instrumentationName = "github.com/gotd/teled/internal/objstore"

var (
	tracer = otel.Tracer(instrumentationName)
	meter  = otel.Meter(instrumentationName)

	// opDuration records object store operation latency in seconds, labeled by
	// operation (put, get, ...).
	opDuration = obs.Must(meter.Float64Histogram("teled.objstore.duration",
		metric.WithDescription("Duration of object store operations."),
		metric.WithUnit("s"),
	))
)

// observe starts a span for an object store operation and returns a function
// that ends it and records the operation duration. Pass the operation's error
// to the returned function so failures are recorded on the span.
func observe(ctx context.Context, op string) func(error) {
	ctx, span := tracer.Start(ctx, "objstore."+op, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(attribute.String("objstore.operation", op))
	start := time.Now()
	return func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
		opDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("objstore.operation", op)))
	}
}
