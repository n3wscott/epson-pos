/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"fmt"
	"io"
	"strings"
)

func NewBuilder(out io.Writer) ESCPOS {
	return &builder{out: out}
}

type builder struct {
	out io.Writer
}

type ESCPOS interface {
	// InitializePrinter - Clears the data in the print buffer and resets the
	// printer modes to the modes that were in effect when the power was
	// turned on.
	// - Any macro definitions are not cleared.
	// - Offline response selection is not cleared.
	// - Contents of user NV memory are not cleared.
	// - NV graphics (NV bit image) and NV user memory are not cleared.
	// - The maintenance counter value is not affected by this command.
	// - Software setting values are not cleared.
	InitializePrinter()

	// CharacterFont selects the font to be used.
	// TM-T88V:
	// n          | size (width × height) | baseline (from the top of a character)
	// 0 (Font A) | 12 × 24 dots          | 21 dots
	// 1 (Font B) | 9 × 17 dots           | 16 dots
	CharacterFont(n uint8)

	// Justification sets the text justification.
	// One of: ["right", "center", "left"]
	Justification(justification string)

	// Home returns the printer beginning of the print line.
	// `print`:
	// - true: Prints the data in the print buffer then moves the print position.
	// - false: Erases the data in the print buffer then moves the print position.
	Home(print bool)

	// Cut feeds paper to [cutting position + (n × vertical motion unit)] and executes paper cut.
	Cut(n uint, full bool)

	// Strong turns emphasized mode on/off.
	Strong(enabled bool)

	// CharacterSize selects character size.
	// `width` and `height` are allowed to be [0-7].
	CharacterSize(width, height uint8)

	// DefaultLineSpacing selects the default line spacing.
	DefaultLineSpacing()

	// LineSpacing sets the line spacing to n × (vertical or horizontal motion unit).
	// See also: SetMotion()
	LineSpacing(n uint8)

	// Print prints text. Fixing newlines and tabs.
	Print(line string)

	//PrintFeed prints the data in the print buffer and feeds the paper
	// [n × (vertical or horizontal motion unit)], n = [0-255].
	PrintFeed(lines uint8)

	//PrintFeedLines prints and feeds n lines. n = [0-255]
	PrintFeedLines(lines uint8)
}

func (b *builder) InitializePrinter() {
	_, _ = fmt.Fprintln(b.out, `'// Initialize printer`)
	_, _ = fmt.Fprintln(b.out, `    ESC "@"`)
}

func (b *builder) CharacterFont(n uint8) {
	switch n {
	case 0, 48:
		_, _ = fmt.Fprintln(b.out, `'// Select Font A`)
	case 1, 49:
		_, _ = fmt.Fprintln(b.out, `'// Select Font B`)
	case 2, 50:
		_, _ = fmt.Fprintln(b.out, `'// Select Font C`)
	case 3, 51:
		_, _ = fmt.Fprintln(b.out, `'// Select Font D`)
	case 4, 52:
		_, _ = fmt.Fprintln(b.out, `'// Select Font D`)
	case 97:
		_, _ = fmt.Fprintln(b.out, `'// Select Special Font A`)
	case 98:
		_, _ = fmt.Fprintln(b.out, `'// Select Special Font B`)
	default:
		_, _ = fmt.Fprintln(b.out, `'// WARNING: Select Unknown Font`)
	}
	_, _ = fmt.Fprintf(b.out, `    ESC "M" %d`+"\n", n)
}

func (b *builder) Strong(enabled bool) {
	if enabled {
		_, _ = fmt.Fprintln(b.out, `'// Emphasized mode on`)
		_, _ = fmt.Fprintln(b.out, `    ESC "E" 1`)
	} else {
		_, _ = fmt.Fprintln(b.out, `'// Emphasized mode off`)
		_, _ = fmt.Fprintln(b.out, `    ESC "E" 0`)
	}
}

func (b *builder) Justification(justification string) {
	// TODO: this command only works if print position is 0.
	switch strings.ToLower(justification) {
	case "left":
		_, _ = fmt.Fprintln(b.out, `'// Left justification`)
		_, _ = fmt.Fprintln(b.out, `    ESC "a" 0`)
	case "center", "centered":
		_, _ = fmt.Fprintln(b.out, `'// Centered justification`)
		_, _ = fmt.Fprintln(b.out, `    ESC "a" 1`)
	case "right":
		_, _ = fmt.Fprintln(b.out, `'// Right justification`)
		_, _ = fmt.Fprintln(b.out, `    ESC "a" 2`)
	default:
		_, _ = fmt.Fprintf(b.out, "'// ERROR: unknown justification: %s\n", justification)
	}
}

func (b *builder) Home(print bool) {
	if print {
		_, _ = fmt.Fprintln(b.out, `'// Home, print first`)
		_, _ = fmt.Fprintln(b.out, `    GS "T" 1`)
	} else {
		_, _ = fmt.Fprintln(b.out, `'// Home, reset print buffer`)
		_, _ = fmt.Fprintln(b.out, `    GS "T" 0`)
	}
}

func (b *builder) Cut(n uint, full bool) {
	// https://reference.epson-biz.com/modules/ref_escpos/index.php?content_id=87
	if n == 0 {
		// This is implementing Function A
		if full {
			_, _ = fmt.Fprintln(b.out, `'// Cut Paper (full cut)`)
			_, _ = fmt.Fprintln(b.out, `    GS "V" 0`)
		} else {
			_, _ = fmt.Fprintln(b.out, `'// Cut Paper (partial cut)`)
			_, _ = fmt.Fprintln(b.out, `    GS "V" 1`)
		}
	} else {
		// This is implementing Function B
		if full {
			_, _ = fmt.Fprintln(b.out, `'// Feed and Cut Paper (full cut)`)
			_, _ = fmt.Fprintf(b.out, `    GS "V" 65 %d`+"\n", n)
		} else {
			_, _ = fmt.Fprintln(b.out, `'// Feed and Cut Paper (partial cut)`)
			_, _ = fmt.Fprintf(b.out, `    GS "V" 66 %d`+"\n", n)
		}
	}
}

func (b *builder) CharacterSize(width, height uint8) {
	if width > 7 {
		width = 7
	}
	if height > 7 {
		height = 7
	}
	_, _ = fmt.Fprintf(b.out, "'// Character magnification Wx%d Hx%d\n", width+1, height+1)
	_, _ = fmt.Fprintf(b.out, `    GS "!" 0x%d%d`+"\n", width, height)
}

func (b *builder) DefaultLineSpacing() {
	_, _ = fmt.Fprintln(b.out, `'// Default Line Spacing`)
	_, _ = fmt.Fprintln(b.out, `    ESC 2`)
}

func (b *builder) LineSpacing(n uint8) {
	_, _ = fmt.Fprintln(b.out, `'// Set Line Spacing`)
	_, _ = fmt.Fprintf(b.out, `    ESC 3 %d`+"\n", n)
}

func (b *builder) Print(line string) {
	// Outside of strings:
	// - Tabs are converted to HT
	// - Newlines are converted to LF

	nl := strings.Split(line, "\n")

	for i, l := range nl {
		if i > 0 {
			_, _ = fmt.Fprintln(b.out, ` LF`)
		}
		_, _ = fmt.Fprint(b.out, `    `)
		tl := strings.Split(l, "\t")
		for i, t := range tl {
			if i > 0 {
				_, _ = fmt.Fprint(b.out, ` HT `)
			}
			if len(t) > 0 {
				_, _ = fmt.Fprintf(b.out, `"%s"`, t)
			}
		}
	}
	_, _ = fmt.Fprintln(b.out, ``)
}

func (b *builder) PrintFeed(n uint8) {
	if n > 255 {
		n = 255
	}
	_, _ = fmt.Fprintln(b.out, `'// Print and feed`)
	_, _ = fmt.Fprintf(b.out, `    ESC "J" %d`+"\n", n)
}

func (b *builder) PrintFeedLines(n uint8) {
	if n > 255 {
		n = 255
	}
	_, _ = fmt.Fprintln(b.out, `'// Print and feed lines`)
	_, _ = fmt.Fprintf(b.out, `    ESC "d" %d`+"\n", n)
}
