package rpc

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/gotd/teled/internal/obs"
)

const instrumentationName = "github.com/gotd/teled/internal/rpc"

var (
	tracer = otel.Tracer(instrumentationName)
	meter  = otel.Meter(instrumentationName)

	// rpcRequests counts handled RPC requests, labeled by method and status.
	rpcRequests = obs.Must(meter.Int64Counter("teled.rpc.requests",
		metric.WithDescription("Total number of handled RPC requests."),
		metric.WithUnit("{request}"),
	))
	// rpcDuration records RPC handler latency in seconds, labeled by method.
	rpcDuration = obs.Must(meter.Float64Histogram("teled.rpc.duration",
		metric.WithDescription("Duration of RPC request handling."),
		metric.WithUnit("s"),
	))
)
