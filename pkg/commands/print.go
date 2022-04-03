/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/n3wscott/escpos/pkg/escpos"
)

func Print() *cobra.Command {
	impl := new(printImpl)
	var file string
	cmd := &cobra.Command{
		Use:   "print HOST:PORT --file [ESC_POS_FILE | -]",
		Short: "Print a ESC/POS formatted file to a printer.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			impl.stdout = cmd.OutOrStdout()
			impl.stderr = cmd.ErrOrStderr()

			// Host
			if len(args) != 1 {
				return fmt.Errorf("expected HOST:PORT (tip: look for printer-??? using `arp -a`; PORT is normally 9100)")
			}
			host := args[0]
			if err := impl.WithHost(host); err != nil {
				return err
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
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

type printImpl struct {
	done   []func() error
	stdout io.Writer
	stderr io.Writer

	in  io.Reader
	out net.Conn
}

// withHost will attempt to resolve and connect to the provided `host`.
// `host` is expected to be `HOST:PORT`.
func (i *printImpl) WithHost(host string) error {
	_, err := net.ResolveTCPAddr("tcp", host)
	if err != nil {
		return fmt.Errorf("unable to resolve TCP address %s: %w", host, err)
	}
	ipconn, err := net.DialTimeout("tcp", host, time.Second*5)
	if err != nil {
		return fmt.Errorf("unable to dial to TCP address %s: %w", host, err)
	}
	i.out = ipconn
	i.done = append(i.done, ipconn.Close)
	return nil
}

// PrintESCPOS will convert in into ESC/POS bytes and send them to out.
func (i *printImpl) PrintESCPOS(ctx context.Context) error {
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
