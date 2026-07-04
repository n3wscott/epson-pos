/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMarkdownPreviewAPIContract(t *testing.T) {
	app := newTestDashboard(t)

	body := bytes.NewBufferString(`{"source":"# Lantern Market\n\n::barcode code39 1042"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/preview", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp previewResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Bytes == 0 {
		t.Fatal("expected compiled byte count")
	}
	if !strings.Contains(resp.Preview, "Lantern Market") {
		t.Fatalf("expected preview text, got %q", resp.Preview)
	}
	if !strings.Contains(resp.POS, `GS "k" 4 "*1042*" 0`) {
		t.Fatalf("expected CODE39 POS command, got:\n%s", resp.POS)
	}
}

func TestMarkdownPrintAPIRejectsInvalidMarkdownBeforeDialingPrinter(t *testing.T) {
	app := newTestDashboard(t)

	body := bytes.NewBufferString(`{"source":"::nope","printer":"127.0.0.1:1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/print", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp printResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Fatal("expected failed print response")
	}
	if !strings.Contains(resp.Error, "unknown directive") {
		t.Fatalf("expected markdown error, got %q", resp.Error)
	}
}

func TestMarkdownPrintAPIDefaultsToServerPrinter(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	received := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			received <- nil
			return
		}
		defer conn.Close()
		data, _ := io.ReadAll(conn)
		received <- data
	}()

	app, err := newDashboard("127.0.0.1:0", listener.Addr().String(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"source":"# Lantern Market\n\nOrder | 1042"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/print", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp printResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Bytes == 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}

	got := <-received
	if len(got) == 0 {
		t.Fatal("expected bytes written to default printer connection")
	}
	if !bytes.Contains(got, []byte("Lantern Market")) {
		t.Fatalf("expected receipt bytes, got %q", got)
	}
}

func TestMarkdownPrintAPIRediscoversAfterDialFailure(t *testing.T) {
	app := newTestDashboard(t)
	badTarget := "10.77.0.85:9100"
	goodTarget := "192.168.86.56:9100"
	app.printers.configuredTarget = badTarget
	app.printers.activeTarget = badTarget

	var got bytes.Buffer
	var attempts []string
	app.printers.dial = func(_ context.Context, target string) (io.WriteCloser, error) {
		attempts = append(attempts, target)
		if target == badTarget {
			return nil, errors.New("no route to host")
		}
		return nopWriteCloser{Writer: &got}, nil
	}
	app.printers.check = func(_ context.Context, target, _ string) bool {
		return target == goodTarget
	}
	app.printers.scan = func(_ context.Context, preferred []string, _ string) ([]string, error) {
		if len(preferred) == 0 || preferred[0] != badTarget {
			t.Fatalf("expected bad configured target first, got %#v", preferred)
		}
		return []string{goodTarget}, nil
	}

	body := bytes.NewBufferString(`{"source":"# Lantern Market\n\nOrder | 1042"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/print", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(attempts) != 2 || attempts[0] != badTarget || attempts[1] != goodTarget {
		t.Fatalf("expected bad target then rediscovered target, got %#v", attempts)
	}
	if !bytes.Contains(got.Bytes(), []byte("Lantern Market")) {
		t.Fatalf("expected receipt bytes, got %q", got.Bytes())
	}

	status := app.printers.Status(req.Context())
	if status.ActiveTarget != goodTarget || status.LastSuccessfulTarget != goodTarget {
		t.Fatalf("expected successful target %s in status, got %#v", goodTarget, status)
	}
}

func TestMarkdownPrintAPIDoesNotRediscoverAfterWriteFailure(t *testing.T) {
	app := newTestDashboard(t)
	target := "192.168.86.56:9100"
	app.printers.configuredTarget = target
	app.printers.activeTarget = target

	scanned := false
	app.printers.dial = func(_ context.Context, target string) (io.WriteCloser, error) {
		return failingWriteCloser{}, nil
	}
	app.printers.scan = func(_ context.Context, preferred []string, _ string) ([]string, error) {
		scanned = true
		return nil, nil
	}

	body := bytes.NewBufferString(`{"source":"# Lantern Market\n\nOrder | 1042"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/print", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if scanned {
		t.Fatal("did not expect rediscovery after a write failure")
	}
}

func TestMarkdownPrintAPIRejectsTargetWhenPrinterMACDoesNotMatch(t *testing.T) {
	app := newTestDashboard(t)
	target := "192.168.86.246:9100"
	app.printers.configuredTarget = target
	app.printers.activeTarget = target
	app.printers.printerMAC = "b0:e8:92:fc:dd:26"

	dialed := false
	app.printers.check = func(_ context.Context, target, printerMAC string) bool {
		return false
	}
	app.printers.dial = func(_ context.Context, target string) (io.WriteCloser, error) {
		dialed = true
		return nopWriteCloser{Writer: io.Discard}, nil
	}
	app.printers.scan = func(_ context.Context, preferred []string, printerMAC string) ([]string, error) {
		return nil, nil
	}

	body := bytes.NewBufferString(`{"source":"# Lantern Market\n\nOrder | 1042"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/print", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if dialed {
		t.Fatal("did not expect receipt bytes to be sent before MAC validation passed")
	}
}

func newTestDashboard(t *testing.T) *dashboard {
	t.Helper()
	app, err := newDashboard("127.0.0.1:0", "127.0.0.1:1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return app
}

type nopWriteCloser struct {
	io.Writer
}

func (w nopWriteCloser) Close() error {
	return nil
}

type failingWriteCloser struct{}

func (f failingWriteCloser) Write(_ []byte) (int, error) {
	return 0, errors.New("paper path failed")
}

func (f failingWriteCloser) Close() error {
	return nil
}
