package db

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gotd/teled/internal/obs"
)

const instrumentationName = "github.com/gotd/teled/internal/db"

var (
	tracer = otel.Tracer(instrumentationName)
	meter  = otel.Meter(instrumentationName)

	// queryDuration records database query latency in seconds, labeled by the
	// SQL operation (SELECT, INSERT, ...). A histogram also carries the count.
	queryDuration = obs.Must(meter.Float64Histogram("teled.db.query.duration",
		metric.WithDescription("Duration of database queries."),
		metric.WithUnit("s"),
	))
)

// queryTracer implements pgx.QueryTracer to open a span and record metrics for
// every query executed through the pool, covering all DB methods at once.
type queryTracer struct{}

var _ pgx.QueryTracer = queryTracer{}

type queryTraceKey struct{}

type queryTraceData struct {
	span  trace.Span
	start time.Time
	op    string
}

// TraceQueryStart starts a client span for the query.
func (queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	op := sqlOperation(data.SQL)
	ctx, span := tracer.Start(ctx, "db.query "+op, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", op),
		attribute.String("db.statement", data.SQL),
	)
	return context.WithValue(ctx, queryTraceKey{}, &queryTraceData{span: span, start: time.Now(), op: op})
}

// TraceQueryEnd ends the span and records the query duration metric.
func (queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	d, ok := ctx.Value(queryTraceKey{}).(*queryTraceData)
	if !ok {
		return
	}
	if data.Err != nil {
		d.span.RecordError(data.Err)
		d.span.SetStatus(codes.Error, data.Err.Error())
	}
	d.span.End()
	queryDuration.Record(ctx, time.Since(d.start).Seconds(),
		metric.WithAttributes(attribute.String("db.operation", d.op)))
}

// sqlOperation returns the leading SQL keyword (e.g. SELECT) for low-cardinality
// labeling, or "UNKNOWN" if the statement is empty.
func sqlOperation(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "UNKNOWN"
	}
	if i := strings.IndexAny(sql, " \t\r\n"); i > 0 {
		sql = sql[:i]
	}
	return strings.ToUpper(sql)
}
