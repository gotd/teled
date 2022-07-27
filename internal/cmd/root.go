// Package cmd implements commands of teled binary.
package cmd

import (
	"github.com/spf13/cobra"
)

func newRoot(a *application) *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:   "teled",
		Short: "Telegram Server in Go",
		Long: `Telegram Server in Go, including auxiliary commands.

Not affiliated with official Telegram.

Apache License 2.0, The GoTD Authors. 
Based on https://gotd.dev Telegram protocol implementation.`,
	}

	rootCmd.AddCommand(
		newKeys(a),
	)

	return rootCmd
}

// Execute executes root command.
func Execute() {
	a := &application{}
	if err := newRoot(a).Execute(); err != nil {
		panic(err)
	}
}
