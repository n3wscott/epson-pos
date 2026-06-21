/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarkdownToPOSBarcode(t *testing.T) {
	pos, err := MarkdownToPOS("# Receipt\n\n::barcode code39 1042\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pos, `GS "k" 4 "*1042*" 0`) {
		t.Fatalf("expected CODE39 command, got:\n%s", pos)
	}
	if !strings.Contains(pos, `GS "!" 0x22`) {
		t.Fatalf("expected h1 size command, got:\n%s", pos)
	}

	var out bytes.Buffer
	if err := Convert(strings.NewReader(pos), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("expected compiled bytes")
	}
}

func TestMarkdownToPOSQRCode(t *testing.T) {
	pos, err := MarkdownToPOS("::qr https://example.test/order/1042\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pos, `GS "(k" 4 0 49 65 50 0`) {
		t.Fatalf("expected QR model command, got:\n%s", pos)
	}
	if !strings.Contains(pos, `49 80 48 "https://example.test/order/1042"`) {
		t.Fatalf("expected QR store command, got:\n%s", pos)
	}
}

func TestMarkdownHeadingSizes(t *testing.T) {
	pos, err := MarkdownToPOS(strings.Join([]string{
		"# Level 1",
		"## Level 2",
		"### Level 3",
		"#### Level 4",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`GS "!" 0x22`,
		`GS "!" 0x11`,
		`GS "!" 0x10`,
		`GS "!" 0x00`,
	} {
		if !strings.Contains(pos, want) {
			t.Fatalf("expected heading size %s, got:\n%s", want, pos)
		}
	}
}
