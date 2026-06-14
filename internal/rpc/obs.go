package rpc

import (
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gotd/teled/internal/obs"
)

const instrumentationName = "github.com/gotd/teled/internal/rpc"

// observability holds the tracer and metric instruments for the RPC layer,
// built from the providers threaded in via New.
type observability struct {
	tracer trace.Tracer
	// requests counts handled RPC requests, labeled by method and status.
	requests metric.Int64Counter
	// duration records RPC handler latency in seconds, labeled by method.
	duration metric.Float64Histogram
}

func newObservability(p obs.Providers) observability {
	m := p.Meter(instrumentationName)

	return observability{
		tracer: p.Tracer(instrumentationName),
		requests: obs.Must(m.Int64Counter("teled.rpc.requests",
			metric.WithDescription("Total number of handled RPC requests."),
			metric.WithUnit("{request}"),
		)),
		duration: obs.Must(m.Float64Histogram("teled.rpc.duration",
			metric.WithDescription("Duration of RPC request handling."),
			metric.WithUnit("s"),
		)),
	}
}
