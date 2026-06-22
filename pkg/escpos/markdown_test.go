/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"bytes"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
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

func TestMarkdownRowImageQR(t *testing.T) {
	pos, err := MarkdownToPOS("::row image:A1 qr:https://example.test/order/1042\n")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`'// PREVIEW [NV IMAGE A1]`,
		`GS "v" "0" 0`,
	} {
		if !strings.Contains(pos, want) {
			t.Fatalf("expected %s in row POS, got:\n%s", want, pos)
		}
	}
	for _, notWant := range []string{`ESC "L"`, `GS "(L" 6 0 48 69 "A1" 1 1`} {
		if strings.Contains(pos, notWant) {
			t.Fatalf("did not expect page-mode/native image command %s in row POS, got:\n%s", notWant, pos)
		}
	}

	preview := Preview(pos)
	if !strings.Contains(preview, "[NV IMAGE A1]") || !strings.Contains(preview, "[QR CODE]") {
		t.Fatalf("expected row preview, got:\n%s", preview)
	}
	if strings.Count(preview, "[QR CODE]") != 1 {
		t.Fatalf("expected one QR preview, got:\n%s", preview)
	}

	var out bytes.Buffer
	if err := Convert(strings.NewReader(pos), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("expected compiled bytes")
	}
}

func TestRowQRCodeImageUsesIntegerModuleScale(t *testing.T) {
	qr, err := qrcode.New("BEER", qrcode.Highest)
	if err != nil {
		t.Fatal(err)
	}

	natural := qr.Image(-1).Bounds().Dx()
	img, err := rowQRCodeImage(qr)
	if err != nil {
		t.Fatal(err)
	}

	size := img.Bounds().Dx()
	if size%natural != 0 {
		t.Fatalf("QR image size %d is not an integer multiple of natural grid %d", size, natural)
	}
	if size > 220 {
		t.Fatalf("QR image size %d exceeds row limit", size)
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
