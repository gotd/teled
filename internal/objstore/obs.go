package objstore

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gotd/teled/internal/obs"
)

const instrumentationName = "github.com/gotd/teled/internal/objstore"

// observability holds the tracer and metric instruments for the object store,
// built from the providers passed to NewFS.
type observability struct {
	tracer trace.Tracer
	// opDuration records object store operation latency in seconds, labeled by
	// operation (put, get, ...).
	opDuration metric.Float64Histogram
}

func newObservability(p obs.Providers) observability {
	return observability{
		tracer: p.Tracer(instrumentationName),
		opDuration: obs.Must(p.Meter(instrumentationName).Float64Histogram(
			"teled.objstore.duration",
			metric.WithDescription("Duration of object store operations."),
			metric.WithUnit("s"),
		)),
	}
}

// observe starts a span for an object store operation and returns a function
// that ends it and records the operation duration. Pass the operation's error
// to the returned function so failures are recorded on the span.
func (s *FS) observe(ctx context.Context, op string) func(error) {
	ctx, span := s.obs.tracer.Start(ctx, "objstore."+op, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(attribute.String("objstore.operation", op))
	start := time.Now()
	return func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
		s.obs.opDuration.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("objstore.operation", op)))
	}
}
