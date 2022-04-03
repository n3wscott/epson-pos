/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

// ref: https://reference.epson-biz.com/modules/ref_escpos/index.php?content_id=72
var commands = map[string]byte{
	"EOT": 0x04, // End of Transmission
	"ENQ": 0x05, // Enquiry
	"HT":  0x09, // Horizontal Tab
	"DLE": 0x10, // Data Line Escape
	"LF":  0x0a, // Print and Line Feed
	"FF":  0x0C, // Print and return to Standard mode from Page mode.
	"CR":  0x0D, // Carriage Return
	"DC4": 0x14, // Device Control 4
	"CAN": 0x18, // Cancel print data
	"ESC": 0x1B, // Escape
	"FS":  0x1C, // File Separator
	"GS":  0x1D, // Group Separator
	"SP":  0x20, // Space ` `

	// TODO: NUL
}
