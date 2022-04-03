/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"github.com/spf13/cobra"
	"sigs.k8s.io/release-utils/version"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "escpos",
		Short:         "Epson ESC/POS printer interface",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Add sub-commands.
	cmd.AddCommand(Print(), version.Version())

	return cmd
}
