// Package obs holds small helpers shared by the OpenTelemetry instrumentation
// across teled's internal packages.
package obs

import (
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Providers carries the OpenTelemetry providers threaded explicitly through the
// application (from go-faster/sdk app.Run) instead of relying on the global
// otel providers. The zero value is valid and yields no-op instrumentation.
type Providers struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

// Tracer returns a named tracer, falling back to a no-op provider when unset.
func (p Providers) Tracer(name string) trace.Tracer {
	tp := p.TracerProvider
	if tp == nil {
		tp = tracenoop.NewTracerProvider()
	}
	return tp.Tracer(name)
}

// Meter returns a named meter, falling back to a no-op provider when unset.
func (p Providers) Meter(name string) metric.Meter {
	mp := p.MeterProvider
	if mp == nil {
		mp = metricnoop.NewMeterProvider()
	}
	return mp.Meter(name)
}

// Must panics if err is non-nil, otherwise returns v. It is meant for
// constructing OpenTelemetry instruments, where an error means a programming
// mistake (e.g. an invalid instrument name).
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
