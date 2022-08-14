// Package cmd implements commands of teled binary.
package cmd

import (
	"net"
	"os"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/gotd/td/tg"

	"github.com/gotd/td/tgtest"
	"github.com/gotd/td/transport"

	"github.com/gotd/teled/internal/key"
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
			ctx := cmd.Context()
			privateKeyEncoded, err := os.ReadFile(a.PrivateKeyPath)
			if err != nil {
				return errors.Wrap(err, "failed to read private key")
			}
			k, err := key.ParsePrivateKey(privateKeyEncoded)
			if err != nil {
				return errors.Wrap(err, "failed to parse private key")
			}

			var lc net.ListenConfig
			ln, err := lc.Listen(ctx, "tcp", a.Addr())
			if err != nil {
				return errors.Wrap(err, "failed to listen")
			}
			opt := tgtest.ServerOptions{
				DC:     1,
				Logger: a.lg,
			}
			a.lg.Info("Listening",
				zap.String("addr", a.Addr()),
				zap.Int("dc", opt.DC),
			)
			srv := tgtest.NewServer(tgtest.NewPrivateKey(k), tgtest.UnpackInvoke(a), opt)
			return srv.Serve(ctx, transport.Listen(transport.ObfuscatedListener(ln)))
		},
	}

	{
		f := rootCmd.Flags()
		f.StringVar(&a.Host, "host", "localhost", "Hostname of the server")
		f.IntVar(&a.Port, "port", 10443, "Port of the server")
		f.StringVar(&a.PrivateKeyPath, "key", "", "Path to PEM-encoded private key")

		markFlagsRequired(f, "key")
	}

	rootCmd.AddCommand(
		newKeys(a),
	)

	return rootCmd
}

// Execute executes root command.
func Execute() {
	lg, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	a := &application{
		lg: lg,
	}
	a.setDispatcher(tg.NewServerDispatcher(a.Fallback))
	if err := newRoot(a).Execute(); err != nil {
		panic(err)
	}
}
