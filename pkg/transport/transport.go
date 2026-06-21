/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const DefaultCUPSUSBBackend = "/usr/libexec/cups/backend/usb"

// DialTCP opens a raw ESC/POS network connection. Most Ethernet receipt
// printers listen for raw jobs on port 9100.
func DialTCP(ctx context.Context, host string) (io.WriteCloser, error) {
	_, err := net.ResolveTCPAddr("tcp", host)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve TCP address %s: %w", host, err)
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, fmt.Errorf("unable to dial TCP address %s: %w", host, err)
	}
	return conn, nil
}

// OpenCUPSUSBBackend opens a raw USB printer connection through the system CUPS
// USB backend. This does not require a CUPS printer queue; DEVICE_URI selects
// the USB device and the ESC/POS bytes are streamed to the backend stdin.
func OpenCUPSUSBBackend(ctx context.Context, backendPath, deviceURI, title string, stderr io.Writer) (io.WriteCloser, error) {
	if backendPath == "" {
		backendPath = DefaultCUPSUSBBackend
	}
	if strings.TrimSpace(deviceURI) == "" {
		return nil, fmt.Errorf("expected USB device URI, for example usb://EPSON/...")
	}
	if title == "" {
		title = "escpos"
	}

	user := os.Getenv("USER")
	if user == "" {
		user = "escpos"
	}

	cmd := exec.CommandContext(ctx, backendPath, "1", user, title, "1", "raw")
	cmd.Env = append(os.Environ(), "DEVICE_URI="+deviceURI)
	cmd.Stdout = stderr
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("unable to start CUPS USB backend %s: %w", backendPath, err)
	}

	return &waitWriteCloser{
		WriteCloser: stdin,
		wait:        cmd.Wait,
	}, nil
}

// ListCUPSUSBDevices returns the raw discovery lines emitted by the CUPS USB
// backend. Lines normally begin with "direct usb://...".
func ListCUPSUSBDevices(ctx context.Context, backendPath string) ([]string, error) {
	if backendPath == "" {
		backendPath = DefaultCUPSUSBBackend
	}

	out, err := exec.CommandContext(ctx, backendPath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("unable to list USB devices with %s: %w\n%s", backendPath, err, strings.TrimSpace(string(out)))
	}

	lines := []string(nil)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

type waitWriteCloser struct {
	io.WriteCloser
	wait func() error
}

func (w *waitWriteCloser) Close() error {
	closeErr := w.WriteCloser.Close()
	waitErr := w.wait()
	if closeErr != nil {
		return closeErr
	}
	return waitErr
}
