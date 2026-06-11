// Package obs holds small helpers shared by the OpenTelemetry instrumentation
// across teled's internal packages.
package obs

// Must panics if err is non-nil, otherwise returns v. It is meant for
// constructing OpenTelemetry instruments at package init, where an error means
// a programming mistake (e.g. an invalid instrument name).
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
