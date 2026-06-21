/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var debug = false

type posToken struct {
	value  string
	quoted bool
}

// Convert takes an ascii ESC/POS file from `reader` and converts it to raw
// bytes written to `out` intended for the POS printer.
func Convert(reader io.Reader, out io.Writer) error {
	in := bufio.NewReader(reader)
	for {
		line, err := in.ReadString(commands["LF"])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if len(line) <= 1 || strings.HasPrefix(line, "'//") {
			// Empty or comment. Skip.
			continue
		}

		parts := tokenizePOSLine(line)
		for _, p := range parts {
			if p.value == "" && !p.quoted {
				continue
			}
			if p.quoted {
				n, _ := out.Write(StringToAsciiBytes(p.value))
				if debug {
					fmt.Println("wrote string bytes: ", n)
				}
			} else if code, ok := commands[p.value]; ok {
				if debug {
					fmt.Printf("Code: %s, %x\n", p.value, code)
				}
				_, _ = out.Write([]byte{code})
			} else {
				// Number.
				code, err := strconv.ParseUint(p.value, 0, 8)
				if err != nil {
					return fmt.Errorf("failed to parse number %v: %w", p.value, err)
				}
				if debug {
					if strings.HasPrefix(p.value, "0x") {
						fmt.Printf("%08b", code)
					} else {
						fmt.Printf("Number %d\n", code)
					}
				}
				_, _ = out.Write([]byte{byte(code)})
			}
		}
	}
}

func tokenizePOSLine(line string) []posToken {
	tokens := []posToken(nil)
	var b strings.Builder
	inQuote := false
	quoted := false

	flush := func() {
		tokens = append(tokens, posToken{value: b.String(), quoted: quoted})
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
