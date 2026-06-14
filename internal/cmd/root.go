// Package cmd implements commands of teled binary.
package cmd

import (
	"context"
	"net"
	"os"

	"github.com/go-faster/errors"
	"github.com/go-faster/sdk/app"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/gotd/teled/internal/key"
	"github.com/gotd/teled/internal/objstore"
	"github.com/gotd/teled/internal/obs"
	"github.com/gotd/teled/server"
)

func newRoot(a *application) *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:   "teled",
		Short: "Telegram Server in Go",
		Long: `Telegram Server in Go, including auxiliary commands.

Not affiliated with official Telegram.

Apache License 2.0, The GoTD Authors. 
Based on https://gotd.dev Telegram protocol implementation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Only the long-running server uses go-faster/sdk app.Run, which
			// sets up telemetry and graceful shutdown. CLI subcommands (e.g.
			// keys) run without it. app.Run terminates the process itself.
			setTelemetryDefaults()
			app.Run(func(ctx context.Context, lg *zap.Logger, t *app.Telemetry) error {
				a.lg = lg
				return a.serve(ctx, obs.Providers{
					TracerProvider: t.TracerProvider(),
					MeterProvider:  t.MeterProvider(),
				})
			})
			return nil
		},
	}

	{
		f := rootCmd.Flags()
		f.StringVar(&a.Host, "host", "localhost", "Hostname of the server")
		f.IntVar(&a.Port, "port", 10443, "Port of the server")
		f.StringVar(&a.PrivateKeyPath, "key", "", "Path to PEM-encoded private key")
		f.StringVar(&a.PostgresURI, "postgres-uri", os.Getenv("DATABASE_URL"),
			"PostgreSQL DSN (postgres://...); if empty, auth keys are kept in memory")
		f.StringVar(&a.ObjectStoreDir, "object-store-dir", "./objects",
			"Local filesystem directory for media object storage")

		markFlagsRequired(f, "key")
	}

	rootCmd.AddCommand(
		newKeys(a),
	)

	return rootCmd
}

// serve runs the Telegram server until ctx is canceled. providers carries the
// OpenTelemetry tracer and meter threaded into every layer.
func (a *application) serve(ctx context.Context, providers obs.Providers) error {
	privateKeyEncoded, err := os.ReadFile(a.PrivateKeyPath)
	if err != nil {
		return errors.Wrap(err, "failed to read private key")
	}
	k, err := key.ParsePrivateKey(privateKeyEncoded)
	if err != nil {
		return errors.Wrap(err, "failed to parse private key")
	}

	pool, cleanup, err := a.setupStorage(ctx, providers)
	if err != nil {
		return errors.Wrap(err, "setup storage")
	}
	defer cleanup()

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", a.Addr())
	if err != nil {
		return errors.Wrap(err, "failed to listen")
	}
	store, err := objstore.NewFS(a.ObjectStoreDir, providers)
	if err != nil {
		return errors.Wrap(err, "object store")
	}

	const dc = 1
	a.lg.Info("Listening",
		zap.String("addr", a.Addr()),
		zap.Int("dc", dc),
	)
	srv := server.New(k, pool, store, server.Options{
		DC:             dc,
		Host:           a.Host,
		Port:           a.Port,
		Logger:         a.lg,
		Obfuscated:     true,
		TracerProvider: providers.TracerProvider,
		MeterProvider:  providers.MeterProvider,
	})
	return srv.Serve(ctx, ln)
}

// setTelemetryDefaults makes telemetry opt-in by defaulting the OTEL exporters
// to no-op unless the operator explicitly configures them, so the server does
// not block trying to reach a collector that is not there.
func setTelemetryDefaults() {
	for _, env := range []string{
		"OTEL_METRICS_EXPORTER",
		"OTEL_TRACES_EXPORTER",
		"OTEL_LOGS_EXPORTER",
	} {
		if _, ok := os.LookupEnv(env); !ok {
			_ = os.Setenv(env, "none")
		}
	}
}

// Execute executes root command.
func Execute() {
	if err := newRoot(&application{}).Execute(); err != nil {
		os.Exit(1)
	}
}
