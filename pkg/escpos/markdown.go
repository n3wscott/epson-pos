/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	headingPattern   = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	unorderedPattern = regexp.MustCompile(`^\s*[-*]\s+(.+)$`)
	orderedPattern   = regexp.MustCompile(`^\s*\d+[.)]\s+(.+)$`)
	linkPattern      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
)

// MarkdownToPOS compiles the dashboard markdown dialect into the project's
// textual ESC/POS format. The resulting text can be passed to Convert.
func MarkdownToPOS(source string) (string, error) {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	source = strings.TrimRight(source, "\n")

	var out strings.Builder
	out.WriteString(`'// Generated from markdown` + "\n")
	out.WriteString(`    ESC "@"` + "\n")
	out.WriteString(`    ESC "M" 1` + "\n")
	out.WriteString(`    ESC "a" 0` + "\n")

	for _, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			out.WriteString(`    "" LF` + "\n")
			continue
		}
		if strings.HasPrefix(line, "<!--") && strings.HasSuffix(line, "-->") {
			continue
		}
		if strings.HasPrefix(line, "::") {
			if err := compileDirective(&out, strings.TrimSpace(strings.TrimPrefix(line, "::"))); err != nil {
				return "", err
			}
			continue
		}
		if isMarkdownRule(line) {
			writeTextLine(&out, strings.Repeat("-", previewWidth), false)
			continue
		}
		if heading := headingPattern.FindStringSubmatch(line); heading != nil {
			level := len(heading[1])
			writeHeading(&out, level, heading[2])
			continue
		}
		if item := unorderedPattern.FindStringSubmatch(line); item != nil {
			writeTextLine(&out, "- "+item[1], false)
			continue
		}
		if item := orderedPattern.FindStringSubmatch(line); item != nil {
			writeTextLine(&out, "- "+item[1], false)
			continue
		}
		if isTableDelimiter(line) {
			continue
		}
		if strings.Contains(line, "|") {
			writeTextLine(&out, markdownTableLine(line), false)
			continue
		}
		writeTextLine(&out, line, false)
	}

	return out.String(), nil
}

func compileDirective(out *strings.Builder, directive string) error {
	fields := strings.Fields(directive)
	if len(fields) == 0 {
		return nil
	}

	name := strings.ToLower(fields[0])
	switch name {
	case "left", "center", "right":
		writeAlign(out, name)
	case "align":
		if len(fields) != 2 {
			return fmt.Errorf("::align expects left, center, or right")
		}
		writeAlign(out, fields[1])
	case "bold":
		if len(fields) != 2 {
			return fmt.Errorf("::bold expects on or off")
		}
		switch strings.ToLower(fields[1]) {
		case "on":
			out.WriteString(`    ESC "E" 1` + "\n")
		case "off":
			out.WriteString(`    ESC "E" 0` + "\n")
		default:
			return fmt.Errorf("::bold expects on or off")
		}
	case "font":
		if len(fields) != 2 {
			return fmt.Errorf("::font expects a or b")
		}
		switch strings.ToLower(fields[1]) {
		case "a":
			out.WriteString(`    ESC "M" 0` + "\n")
		case "b":
			out.WriteString(`    ESC "M" 1` + "\n")
		default:
			return fmt.Errorf("::font expects a or b")
		}
	case "size":
		if len(fields) != 2 {
			return fmt.Errorf("::size expects values like 1x1, 2x2, or 3x1")
		}
		width, height, err := parseSize(fields[1])
		if err != nil {
			return err
		}
		out.WriteString(fmt.Sprintf(`    GS "!" 0x%d%d`+"\n", width-1, height-1))
	case "feed":
		if len(fields) != 2 {
			return fmt.Errorf("::feed expects a line count")
		}
		n, err := parseUint8(fields[1])
		if err != nil {
			return fmt.Errorf("::feed expects a line count from 0 to 255: %w", err)
		}
		out.WriteString(fmt.Sprintf(`    ESC "d" %d`+"\n", n))
	case "line", "rule":
		writeTextLine(out, strings.Repeat("-", previewWidth), false)
	case "cut":
		full := false
		feed := uint8(30)
		for _, field := range fields[1:] {
			switch strings.ToLower(field) {
			case "full":
				full = true
			case "partial":
				full = false
			default:
				n, err := parseUint8(field)
				if err != nil {
					return fmt.Errorf("::cut expects partial, full, or feed amount: %w", err)
				}
				feed = n
			}
		}
		if full {
			out.WriteString(fmt.Sprintf(`    GS "V" 65 %d`+"\n", feed))
		} else {
			out.WriteString(fmt.Sprintf(`    GS "V" 66 %d`+"\n", feed))
		}
	case "barcode":
		if len(fields) < 3 {
			return fmt.Errorf("::barcode expects a type and value")
		}
		return writeBarcode(out, fields[1], strings.Join(fields[2:], " "))
	case "qr":
		if len(fields) < 2 {
			return fmt.Errorf("::qr expects data")
		}
		return writeQRCode(out, strings.Join(fields[1:], " "))
	case "row":
		return writeRow(out, fields[1:])
	case "image", "nv-image", "nvimage":
		if len(fields) != 2 {
			return fmt.Errorf("::image expects an NV image key, for example G1")
		}
		key := printableText(fields[1])
		out.WriteString(`    ESC "a" 1` + "\n")
		out.WriteString(fmt.Sprintf(`    GS "(L" 6 0 48 69 "%s" 1 1`+"\n", key))
		out.WriteString(`    ESC "a" 0` + "\n")
	default:
		return fmt.Errorf("unknown directive ::%s", fields[0])
	}
	return nil
}

func writeRow(out *strings.Builder, fields []string) error {
	values := map[string]string{}
	for _, field := range fields {
		key, value, ok := splitDirectiveOption(field)
		if !ok {
			return fmt.Errorf("::row expects options like image:G1 and qr:https://example.test")
		}
		values[key] = value
	}

	imageKey := printableText(values["image"])
	qrValue := values["qr"]
	if imageKey == "" || qrValue == "" {
		return fmt.Errorf("::row expects image:<key> and qr:<data>")
	}
	if len([]byte(imageKey)) != 2 {
		return fmt.Errorf("::row image key must be two ASCII characters, for example G1")
	}

	qrValue = printableText(strings.Trim(qrValue, `"`))
	out.WriteString(fmt.Sprintf(`'// PREVIEW %s`+"\n", previewRowText(imageKey)))
	return writeRasterRow(out, imageKey, qrValue)
}

func splitDirectiveOption(field string) (key, value string, ok bool) {
	if idx := strings.Index(field, ":"); idx >= 0 {
		return strings.ToLower(strings.TrimSpace(field[:idx])), strings.TrimSpace(field[idx+1:]), true
	}
	if idx := strings.Index(field, "="); idx >= 0 {
		return strings.ToLower(strings.TrimSpace(field[:idx])), strings.TrimSpace(field[idx+1:]), true
	}
	return "", "", false
}

func writeHeading(out *strings.Builder, level int, text string) {
	width, height := headingSize(level)
	out.WriteString(`    ESC "a" 1` + "\n")
	out.WriteString(`    ESC "E" 1` + "\n")
	out.WriteString(fmt.Sprintf(`    GS "!" 0x%d%d`+"\n", width-1, height-1))
	writeTextLine(out, text, true)
	out.WriteString(`    GS "!" 0x00` + "\n")
	out.WriteString(`    ESC "E" 0` + "\n")
	out.WriteString(`    ESC "a" 0` + "\n")
}

func headingSize(level int) (int, int) {
	switch level {
	case 1:
		return 3, 3
	case 2:
		return 2, 2
	case 3:
		return 2, 1
	default:
		return 1, 1
	}
}

func writeTextLine(out *strings.Builder, text string, alreadyStyled bool) {
	if !alreadyStyled {
		text = strings.TrimSpace(text)
	}
	out.WriteString("    ")
	writeInline(out, text)
	out.WriteString(" LF\n")
}

func writeInline(out *strings.Builder, text string) {
	text = cleanMarkdownInline(text)
	for {
		start := strings.Index(text, "**")
		if start < 0 {
			writeQuotedParts(out, text)
			return
		}
		end := strings.Index(text[start+2:], "**")
		if end < 0 {
			writeQuotedParts(out, text)
			return
		}
		writeQuotedParts(out, text[:start])
		bold := text[start+2 : start+2+end]
		out.WriteString(` ESC "E" 1 `)
		writeQuotedParts(out, bold)
		out.WriteString(` ESC "E" 0 `)
		text = text[start+2+end+2:]
	}
}

func writeQuotedParts(out *strings.Builder, text string) {
	if text == "" {
		return
	}
	parts := strings.Split(text, "\t")
	for i, part := range parts {
		if i > 0 {
			out.WriteString(" HT ")
		}
		if part != "" {
			out.WriteString(fmt.Sprintf(`"%s"`, printableText(part)))
		}
	}
}

func writeAlign(out *strings.Builder, value string) {
	switch strings.ToLower(value) {
	case "center", "centered":
		out.WriteString(`    ESC "a" 1` + "\n")
	case "right":
		out.WriteString(`    ESC "a" 2` + "\n")
	default:
		out.WriteString(`    ESC "a" 0` + "\n")
	}
}

func writeBarcode(out *strings.Builder, kind, value string) error {
	kind = strings.ToLower(kind)
	value = strings.Trim(strings.TrimSpace(value), `"`)

	out.WriteString(`    ESC "a" 1` + "\n")
	out.WriteString(`    GS "h" 60` + "\n")
	out.WriteString(`    GS "H" 2` + "\n")
	out.WriteString(`    GS "f" 1` + "\n")

	switch kind {
	case "upca", "upc-a":
		out.WriteString(fmt.Sprintf(`    GS "k" 0 "%s" 0`+"\n", printableText(value)))
	case "upce", "upc-e":
		out.WriteString(fmt.Sprintf(`    GS "k" 1 "%s" 0`+"\n", printableText(value)))
	case "ean13", "ean-13", "jan13", "jan-13":
		out.WriteString(fmt.Sprintf(`    GS "k" 2 "%s" 0`+"\n", printableText(value)))
	case "ean8", "ean-8", "jan8", "jan-8":
		out.WriteString(fmt.Sprintf(`    GS "k" 3 "%s" 0`+"\n", printableText(value)))
	case "code39", "code-39":
		if !strings.HasPrefix(value, "*") {
			value = "*" + value
		}
		if !strings.HasSuffix(value, "*") {
			value += "*"
		}
		out.WriteString(fmt.Sprintf(`    GS "k" 4 "%s" 0`+"\n", printableText(value)))
	case "itf":
		out.WriteString(fmt.Sprintf(`    GS "k" 5 "%s" 0`+"\n", printableText(value)))
	case "codabar", "nw7", "nw-7":
		out.WriteString(fmt.Sprintf(`    GS "k" 6 "%s" 0`+"\n", printableText(value)))
	case "code93", "code-93":
		text := printableText(value)
		out.WriteString(fmt.Sprintf(`    GS "k" 72 %d "%s"`+"\n", len(StringToAsciiBytes(text)), text))
	case "code128", "code-128":
		text := printableText(value)
		if !strings.HasPrefix(text, "{") {
			text = "{B" + text
		}
		out.WriteString(fmt.Sprintf(`    GS "k" 73 %d "%s"`+"\n", len(StringToAsciiBytes(text)), text))
	default:
		return fmt.Errorf("unsupported barcode type %q", kind)
	}

	out.WriteString(`    ESC "a" 0` + "\n")
	return nil
}

func writeQRCode(out *strings.Builder, value string) error {
	value = printableText(strings.Trim(strings.TrimSpace(value), `"`))
	data := StringToAsciiBytes(value)
	if len(data) > 7092 {
		return fmt.Errorf("::qr data is too long")
	}

	out.WriteString(`    ESC "a" 1` + "\n")
	writeQRCodeRaw(out, value, data, 6)
	out.WriteString(`    ESC "a" 0` + "\n")
	return nil
}

func writeQRCodeRaw(out *strings.Builder, value string, data []byte, moduleSize int) {
	pL := (len(data) + 3) % 256
	pH := (len(data) + 3) / 256

	out.WriteString(`    GS "(k" 4 0 49 65 50 0` + "\n")
	out.WriteString(fmt.Sprintf(`    GS "(k" 3 0 49 67 %d`+"\n", moduleSize))
	out.WriteString(`    GS "(k" 3 0 49 69 48` + "\n")
	out.WriteString(fmt.Sprintf(`    GS "(k" %d %d 49 80 48 "%s"`+"\n", pL, pH, value))
	out.WriteString(`    GS "(k" 3 0 49 81 48` + "\n")
}

func lowByte(value int) int {
	return value % 256
}

func highByte(value int) int {
	return value / 256
}

func cleanMarkdownInline(text string) string {
	text = linkPattern.ReplaceAllString(text, "$1")
	text = strings.ReplaceAll(text, "`", "")
	text = strings.ReplaceAll(text, "__", "**")
	return text
}

func printableText(text string) string {
	text = strings.ReplaceAll(text, `"`, `'`)
	return strings.TrimSpace(text)
}

func parseSize(value string) (int, int, error) {
	parts := strings.Split(strings.ToLower(value), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("::size expects values like 1x1, 2x2, or 3x1")
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil || width < 1 || width > 8 {
		return 0, 0, fmt.Errorf("::size width must be 1 through 8")
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil || height < 1 || height > 8 {
		return 0, 0, fmt.Errorf("::size height must be 1 through 8")
	}
	return width, height, nil
}

func parseUint8(value string) (uint8, error) {
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, err
	}
	return uint8(n), nil
}

func isMarkdownRule(line string) bool {
	if len(line) < 3 {
		return false
	}
	first := line[0]
	if first != '-' && first != '*' && first != '_' {
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != first {
			return false
		}
	}
	return true
}

func isTableDelimiter(line string) bool {
	line = strings.Trim(line, "| ")
	if line == "" {
		return false
	}
	for _, r := range line {
		if r != '-' && r != ':' && r != '|' && r != ' ' {
			return false
		}
	}
	return strings.Contains(line, "-")
}

func markdownTableLine(line string) string {
	line = strings.Trim(line, "| ")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, "\t")
}
