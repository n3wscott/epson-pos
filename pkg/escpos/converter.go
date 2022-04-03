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

		parts := strings.Split(line, " ")
		inQuote := false
		for _, p := range parts {
			if len(p) == 1 && p == "\"" {
				inQuote = true
				continue
			}
			if inQuote && (p == " " || p == "") {
				_, _ = out.Write([]byte(" "))
			}

			if p == "" || p == " " {
				continue
			}
			if inQuote && !strings.HasSuffix(p, "\"") {
				if debug {
					fmt.Printf("Mid-Quote: %s\n", p)
				}
				n, _ := out.Write(StringToAsciiBytes(p))
				if debug {
					fmt.Println("wrote bytes: ", n)
				}
				_, _ = out.Write([]byte(" "))
			} else if code, ok := commands[p]; ok {
				if debug {
					fmt.Printf("Code: %s, %x\n", p, code)
				}
				_, _ = out.Write([]byte{code})
			} else if strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") {
				code := strings.TrimPrefix(p, "\"")
				code = strings.TrimSuffix(code, "\"")
				if debug {
					fmt.Printf("String: %s\n", code)
				}
				_, _ = out.Write(StringToAsciiBytes(code))
			} else if strings.HasPrefix(p, "\"") {
				inQuote = true
				code := strings.TrimPrefix(p, "\"")
				if debug {
					fmt.Printf("Start Quote: %s\n", code)
				}
				n, _ := out.Write(StringToAsciiBytes(code))
				if debug {
					fmt.Println("wrote bytes: ", n)
				}
			} else if strings.HasSuffix(p, "\"") {
				inQuote = false
				code := strings.TrimSuffix(p, "\"")
				if debug {
					fmt.Printf("End Quote: %s\n", code)
				}
				n, _ := out.Write(StringToAsciiBytes(code))
				if debug {
					fmt.Println("wrote bytes: ", n)
				}
			} else {
				// Number.
				code, err := strconv.ParseUint(p, 0, 8)
				if err != nil {
					return fmt.Errorf("failed to parse number %v: %w", p, err)
				}
				if debug {
					if strings.HasPrefix(p, "0x") {
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
