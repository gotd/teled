package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func markFlagsRequired(flags *pflag.FlagSet, names ...string) {
	for _, name := range names {
		if err := cobra.MarkFlagRequired(flags, name); err != nil {
			panic(err)
		}
	}
}
