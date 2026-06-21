/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPreviewBarcodeLinesFitReceiptWidth(t *testing.T) {
	pos, err := MarkdownToPOS(strings.Join([]string{
		"# Lantern Market",
		"",
		"::line",
		"::barcode code39 1042",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(Preview(pos), "\n") {
		if width := utf8.RuneCountInString(line); width > previewWidth {
			t.Fatalf("preview line is %d columns, want <= %d: %q", width, previewWidth, line)
		}
	}
}
