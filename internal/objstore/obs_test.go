package objstore_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/gotd/teled"
	"github.com/gotd/teled/internal/objstore"
	"github.com/gotd/teled/internal/obs"
)

// TestFSTracing verifies that object store operations emit spans through the
// tracer provider passed in via NewFS.
func TestFSTracing(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))

	ctx := context.Background()
	fs, err := objstore.NewFS(t.TempDir(), obs.Providers{TracerProvider: tp})
	require.NoError(t, err)

	require.NoError(t, fs.Put(ctx, "abcd-key", bytes.NewBufferString("payload"), 7, teled.PutOptions{}))
	rc, err := fs.Get(ctx, "abcd-key")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, rc)
	require.NoError(t, rc.Close())
	require.NoError(t, fs.Delete(ctx, "abcd-key"))

	require.NoError(t, tp.ForceFlush(ctx))
	var names []string
	for _, s := range sr.Ended() {
		names = append(names, s.Name())
	}
	require.Subset(t, names, []string{"objstore.put", "objstore.get", "objstore.delete"})
}
