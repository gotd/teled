package mtproto

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/gotd/teled/internal/obs"
)

const instrumentationName = "github.com/gotd/teled/internal/mtproto"

var (
	tracer = otel.Tracer(instrumentationName)
	meter  = otel.Meter(instrumentationName)

	// activeConns tracks the number of currently connected clients.
	activeConns = obs.Must(meter.Int64UpDownCounter("teled.mtproto.connections.active",
		metric.WithDescription("Number of active client connections."),
		metric.WithUnit("{connection}"),
	))
	// messages counts decrypted MTProto messages handled by the server.
	messages = obs.Must(meter.Int64Counter("teled.mtproto.messages",
		metric.WithDescription("Number of decrypted MTProto messages handled."),
		metric.WithUnit("{message}"),
	))
)
