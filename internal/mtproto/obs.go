package mtproto

import (
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gotd/teled/internal/obs"
)

const instrumentationName = "github.com/gotd/teled/internal/mtproto"

// observability holds the tracer and metric instruments for the MTProto layer,
// built from the providers passed via ServerOptions.
type observability struct {
	tracer trace.Tracer
	// activeConns tracks the number of currently connected clients.
	activeConns metric.Int64UpDownCounter
	// messages counts decrypted MTProto messages handled by the server.
	messages metric.Int64Counter
}

func newObservability(p obs.Providers) observability {
	m := p.Meter(instrumentationName)
	return observability{
		tracer: p.Tracer(instrumentationName),
		activeConns: obs.Must(m.Int64UpDownCounter("teled.mtproto.connections.active",
			metric.WithDescription("Number of active client connections."),
			metric.WithUnit("{connection}"),
		)),
		messages: obs.Must(m.Int64Counter("teled.mtproto.messages",
			metric.WithDescription("Number of decrypted MTProto messages handled."),
			metric.WithUnit("{message}"),
		)),
	}
}
