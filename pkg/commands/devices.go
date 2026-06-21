/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/n3wscott/escpos/pkg/transport"
)

func Devices() *cobra.Command {
	var usbBackend string

	cmd := &cobra.Command{
		Use:   "devices",
		Short: "Discover raw printer transports.",
		RunE: func(cmd *cobra.Command, args []string) error {
			lines, err := transport.ListCUPSUSBDevices(cmd.Context(), usbBackend)
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No USB printers reported by the CUPS USB backend.")
				return nil
			}
			for _, line := range lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&usbBackend, "usb-backend", transport.DefaultCUPSUSBBackend, "Path to the CUPS USB backend.")
	return cmd
}
