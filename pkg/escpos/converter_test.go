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

func TestConvertPreservesSpacesInsideQuotedStrings(t *testing.T) {
	var out bytes.Buffer
	if err := Convert(strings.NewReader(`"Lantern Market" LF`+"\n"), &out); err != nil {
		t.Fatal(err)
	}

	want := []byte("Lantern Market\n")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("got %q, want %q", out.Bytes(), want)
	}
}

func TestConvertPreservesRepeatedSpacesInsideQuotedStrings(t *testing.T) {
	var out bytes.Buffer
	if err := Convert(strings.NewReader(`"a   b" LF`+"\n"), &out); err != nil {
		t.Fatal(err)
	}

	want := []byte("a   b\n")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("got %q, want %q", out.Bytes(), want)
	}
}
