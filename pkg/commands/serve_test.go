/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"bytes"
	"encoding/json"
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

func newTestDashboard(t *testing.T) *dashboard {
	t.Helper()
	app, err := newDashboard("127.0.0.1:0", "127.0.0.1:1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return app
}
