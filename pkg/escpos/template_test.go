/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParseTemplateFields(t *testing.T) {
	fields := ParseTemplateFields(`<!-- field:order_id hint="Order number" default="1042" -->
# Order {{order_id}}
Total: {{total}}
`)
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d: %#v", len(fields), fields)
	}
	if fields[0].Name != "order_id" || fields[0].Hint != "Order number" || fields[0].Default != "1042" {
		t.Fatalf("unexpected declared field: %#v", fields[0])
	}
	if fields[1].Name != "total" || fields[1].Hint == "" {
		t.Fatalf("unexpected inferred field: %#v", fields[1])
	}
}

func TestRenderTemplate(t *testing.T) {
	got := RenderTemplate("Order {{order_id}} total {{total}}", map[string]string{
		"order_id": "1042",
		"total":    "$7.75",
	})
	want := "Order 1042 total $7.75"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrinterFeatureShowcaseTemplateCompiles(t *testing.T) {
	source, err := os.ReadFile("../../templates/Printer Feature Showcase.md")
	if err != nil {
		t.Fatal(err)
	}

	values := map[string]string{}
	for _, field := range ParseTemplateFields(string(source)) {
		values[field.Name] = field.Default
	}

	pos, err := MarkdownToPOS(RenderTemplate(string(source), values))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`GS "k" 0`,
		`GS "k" 1`,
		`GS "k" 2`,
		`GS "k" 3`,
		`GS "k" 4`,
		`GS "k" 5`,
		`GS "k" 6`,
		`GS "k" 72`,
		`GS "k" 73`,
		`GS "(k"`,
		`GS "(L"`,
	} {
		if !strings.Contains(pos, want) {
			t.Fatalf("expected %s in generated POS:\n%s", want, pos)
		}
	}

	var out bytes.Buffer
	if err := Convert(strings.NewReader(pos), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("expected compiled bytes")
	}
}
