/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const previewWidth = 56

type previewToken struct {
	value  string
	quoted bool
}

// Preview renders a best-effort text preview for the project's textual ESC/POS
// format. It intentionally ignores commands that do not affect visual preview.
func Preview(source string) string {
	lines := []previewLine(nil)
	var current strings.Builder
	align := "left"
	inPageMode := false

	for _, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "'// PREVIEW ") {
			if current.Len() > 0 {
				lines = append(lines, previewLine{text: current.String(), align: align})
				current.Reset()
			}
			lines = append(lines, previewLine{override: strings.TrimPrefix(line, "'// PREVIEW ")})
			continue
		}
		if len(line) <= 1 || strings.HasPrefix(line, "'//") {
			continue
		}

		tokens := tokenizePreviewLine(line)
		for i := 0; i < len(tokens); i++ {
			t := tokens[i]

			if inPageMode {
				if !t.quoted && t.value == "FF" {
					inPageMode = false
				}
				continue
			}

			if !t.quoted {
				switch t.value {
				case "LF":
					lines = append(lines, previewLine{text: current.String(), align: align})
					current.Reset()
					continue
				case "HT":
					current.WriteString("    ")
					continue
				case "ESC":
					if i+1 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "L" {
						inPageMode = true
						i++
						continue
					}
					if i+1 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "@" {
						i++
						continue
					}
					if i+2 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "a" {
						align = previewAlignment(tokens[i+2].value)
						i += 2
						continue
					}
					if i+2 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "E" {
						i += 2
						continue
					}
					if i+2 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "M" {
						i += 2
						continue
					}
				case "GS":
					if i+1 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "(k" {
						if i+6 < len(tokens) && tokens[i+4].value == "49" && tokens[i+5].value == "81" {
							if current.Len() > 0 {
								lines = append(lines, previewLine{text: current.String(), align: align})
								current.Reset()
							}
							lines = append(lines, previewLine{align: "center", override: centerPreviewText("[QR CODE]", previewWidth)})
						}
						i = len(tokens)
						continue
					}
					if i+1 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "(L" {
						if i+6 < len(tokens) && tokens[i+5].value == "69" {
							if current.Len() > 0 {
								lines = append(lines, previewLine{text: current.String(), align: align})
								current.Reset()
							}
							lines = append(lines, previewLine{align: "center", override: centerPreviewText("[NV IMAGE "+tokens[i+6].value+"]", previewWidth)})
						}
						i = len(tokens)
						continue
					}
					if i+3 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "k" {
						if barcode, ok := previewBarcode(tokens[i+2], tokens[i+3]); ok {
							if current.Len() > 0 {
								lines = append(lines, previewLine{text: current.String(), align: align})
								current.Reset()
							}
							lines = append(lines, previewLine{align: "center", override: barcode})
							i += 3
							continue
						}
					}
					if i+2 < len(tokens) && tokens[i+1].quoted && tokens[i+1].value == "v" {
						i = len(tokens)
						continue
					}
					if i+2 < len(tokens) && tokens[i+1].quoted && (tokens[i+1].value == "!" || tokens[i+1].value == "V") {
						i += 2
						continue
					}
					if i+2 < len(tokens) && tokens[i+1].quoted && (tokens[i+1].value == "h" || tokens[i+1].value == "H" || tokens[i+1].value == "f") {
						i += 2
						continue
					}
				}
			}

			if t.quoted {
				current.WriteString(t.value)
			}
		}
	}

	if current.Len() > 0 {
		lines = append(lines, previewLine{text: current.String(), align: align})
	}

	var out strings.Builder
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		if line.override != "" {
			out.WriteString(line.override)
			continue
		}
		out.WriteString(line.render(previewWidth))
	}
	return out.String()
}

type previewLine struct {
	text     string
	align    string
	override string
}

func (l previewLine) render(width int) string {
	text := l.text
	if text == "" {
		return ""
	}
	size := utf8.RuneCountInString(text)
	if size >= width {
		return text
	}

	pad := width - size
	switch l.align {
	case "right":
		return strings.Repeat(" ", pad) + text
	case "center":
		left := pad / 2
		return strings.Repeat(" ", left) + text
	default:
		return text
	}
}

func tokenizePreviewLine(line string) []previewToken {
	tokens := []previewToken(nil)
	var b strings.Builder
	inQuote := false
	quoted := false

	flush := func() {
		tokens = append(tokens, previewToken{value: b.String(), quoted: quoted})
		b.Reset()
		quoted = false
	}

	for _, r := range line {
		switch {
		case r == '"':
			if inQuote {
				flush()
				inQuote = false
			} else {
				if b.Len() > 0 {
					flush()
				}
				inQuote = true
				quoted = true
			}
		case r == ' ' || r == '\t':
			if inQuote {
				b.WriteRune(r)
			} else if b.Len() > 0 {
				flush()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 || inQuote {
		flush()
	}
	return tokens
}

func previewAlignment(value string) string {
	switch value {
	case "1":
		return "center"
	case "2":
		return "right"
	default:
		return "left"
	}
}

func previewBarcode(kind, value previewToken) (string, bool) {
	if value.value == "" {
		return "", false
	}
	name := barcodeName(kind.value)
	bars := makeBarcodeBars(value.value)
	return fmt.Sprintf("%s\n%s\n%s", bars, centerPreviewText(name, previewWidth), centerPreviewText(value.value, previewWidth)), true
}

func barcodeName(value string) string {
	code, err := strconv.ParseUint(value, 0, 16)
	if err != nil {
		return "BARCODE"
	}
	switch code {
	case 0, 65:
		return "UPC-A"
	case 1, 66:
		return "UPC-E"
	case 2, 67:
		return "EAN13"
	case 3, 68:
		return "EAN8"
	case 4, 69:
		return "CODE39"
	case 5, 70:
		return "ITF"
	case 6, 71:
		return "CODABAR"
	case 72:
		return "CODE93"
	case 73:
		return "CODE128"
	default:
		return "BARCODE"
	}
}

func makeBarcodeBars(value string) string {
	var b strings.Builder
	b.WriteString("||")
	for _, r := range value {
		n := int(r)
		for bit := 0; bit < 7; bit++ {
			if n&(1<<bit) != 0 {
				b.WriteString("||")
			} else {
				b.WriteByte('|')
			}
			b.WriteByte(' ')
		}
	}
	b.WriteString("||")
	line := b.String()
	if utf8.RuneCountInString(line) > previewWidth {
		return string([]rune(line)[:previewWidth])
	}
	return centerPreviewText(line, previewWidth)
}

func previewRowText(imageKey string) string {
	left := "[NV IMAGE " + imageKey + "]"
	right := "[QR CODE]"
	gap := previewWidth - utf8.RuneCountInString(left) - utf8.RuneCountInString(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func centerPreviewText(text string, width int) string {
	size := utf8.RuneCountInString(text)
	if size >= width {
		return text
	}
	return strings.Repeat(" ", (width-size)/2) + text
}
