/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/n3wscott/escpos/pkg/escpos"
	"github.com/n3wscott/escpos/pkg/transport"
)

func Print() *cobra.Command {
	impl := new(printImpl)
	var file string
	var transportMode string
	var usbBackend string
	cmd := &cobra.Command{
		Use:   "print TARGET --file [ESC_POS_FILE | -]",
		Short: "Print a ESC/POS formatted file to a printer.",
		Long: `Print a ESC/POS formatted file to a printer.

TARGET is normally HOST:PORT for Ethernet printers, for example 192.168.1.40:9100.
For USB, pass a usb://... DEVICE_URI from "escpos devices" and set --transport usb.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			impl.stdout = cmd.OutOrStdout()
			impl.stderr = cmd.ErrOrStderr()

			// Target
			if len(args) != 1 {
				return fmt.Errorf("expected TARGET")
			}
			target := args[0]
			switch resolveTransport(transportMode, target) {
			case "tcp":
				if err := impl.WithTCP(cmd.Context(), target); err != nil {
					return err
				}
			case "usb":
				if err := impl.WithUSBBackend(cmd.Context(), usbBackend, target); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown transport %q, expected auto, tcp, or usb", transportMode)
			}

			// File
			if file == "-" {
				impl.in = cmd.InOrStdin()
			} else {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				impl.in = f
				impl.done = append(impl.done, f.Close)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			defer impl.Done()
			return impl.PrintESCPOS(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "ESC/POS file path, or - for stdin.")
	cmd.Flags().StringVar(&transportMode, "transport", "auto", "Transport to use: auto, tcp, or usb.")
	cmd.Flags().StringVar(&usbBackend, "usb-backend", transport.DefaultCUPSUSBBackend, "Path to the CUPS USB backend.")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

type printImpl struct {
	done   []func() error
	stdout io.Writer
	stderr io.Writer

	in  io.Reader
	out io.WriteCloser
}

// WithTCP will attempt to resolve and connect to the provided `host`.
// `host` is expected to be `HOST:PORT`.
func (i *printImpl) WithTCP(ctx context.Context, host string) error {
	conn, err := transport.DialTCP(ctx, host)
	if err != nil {
		return err
	}
	i.out = conn
	i.done = append(i.done, conn.Close)
	return nil
}

func (i *printImpl) WithUSBBackend(ctx context.Context, backendPath, deviceURI string) error {
	conn, err := transport.OpenCUPSUSBBackend(ctx, backendPath, deviceURI, "escpos", i.stderr)
	if err != nil {
		return err
	}
	i.out = conn
	i.done = append(i.done, conn.Close)
	return nil
}

// PrintESCPOS will convert in into ESC/POS bytes and send them to out.
func (i *printImpl) PrintESCPOS(ctx context.Context) error {
	if i.out == nil {
		return fmt.Errorf("printer transport is not open")
	}
	return escpos.Convert(i.in, i.out)
}

// Done calls all the done functions.
func (i *printImpl) Done() {
	for _, done := range i.done {
		if err := done(); err != nil {
			_, _ = fmt.Fprint(i.stderr, err.Error())
		}
	}
}

func resolveTransport(mode, target string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "auto" {
		if strings.HasPrefix(strings.ToLower(target), "usb://") {
			return "usb"
		}
		return "tcp"
	}
	return mode
}
