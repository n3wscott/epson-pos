/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

//go:embed assets/*.png
var nvImageAssets embed.FS

func writeRasterRow(out *strings.Builder, imageKey, qrValue string) error {
	left, err := loadEmbeddedNVImage(imageKey)
	if err != nil {
		return err
	}
	qr, err := qrcode.New(qrValue, qrcode.Highest)
	if err != nil {
		return fmt.Errorf("build QR code: %w", err)
	}

	qrImage, err := rowQRCodeImage(qr)
	if err != nil {
		return err
	}

	const gap = 36
	width := left.Bounds().Dx() + gap + qrImage.Bounds().Dx()
	height := maxInt(left.Bounds().Dy(), qrImage.Bounds().Dy())
	if width > 512 {
		return fmt.Errorf("::row composed image is too wide")
	}

	canvas := image.NewGray(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 0, left.Bounds().Dx(), left.Bounds().Dy()), left, left.Bounds().Min, draw.Over)

	qrY := (height - qrImage.Bounds().Dy()) / 2
	qrRect := image.Rect(left.Bounds().Dx()+gap, qrY, width, qrY+qrImage.Bounds().Dy())
	draw.Draw(canvas, qrRect, qrImage, qrImage.Bounds().Min, draw.Over)

	writeRasterImage(out, canvas)
	return nil
}

func rowQRCodeImage(qr *qrcode.QRCode) (image.Image, error) {
	const (
		maxQRDots      = 220
		maxModuleScale = 7
	)
	natural := qr.Image(-1)
	moduleGrid := natural.Bounds().Dx()
	if moduleGrid <= 0 {
		return nil, fmt.Errorf("::row QR code is empty")
	}
	scale := maxQRDots / moduleGrid
	if scale < 1 {
		return nil, fmt.Errorf("::row QR code is too large for row layout")
	}
	if scale > maxModuleScale {
		scale = maxModuleScale
	}
	return qr.Image(-scale), nil
}

func loadEmbeddedNVImage(key string) (image.Image, error) {
	key = printableText(key)
	f, err := nvImageAssets.Open("assets/" + key + ".png")
	if err != nil {
		return nil, fmt.Errorf("::row has no local image asset for key %s", key)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode local image asset %s: %w", key, err)
	}
	return img, nil
}

func writeRasterImage(out *strings.Builder, img image.Image) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	bytesPerRow := (width + 7) / 8

	out.WriteString(fmt.Sprintf(`    GS "v" "0" 0 %d %d %d %d`+"\n",
		lowByte(bytesPerRow), highByte(bytesPerRow),
		lowByte(height), highByte(height),
	))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		out.WriteString("    ")
		for bx := 0; bx < bytesPerRow; bx++ {
			value := 0
			for bit := 0; bit < 8; bit++ {
				x := bounds.Min.X + bx*8 + bit
				if x < bounds.Max.X && isDark(img.At(x, y)) {
					value |= 1 << (7 - bit)
				}
			}
			if bx > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(fmt.Sprintf("0x%02X", value))
		}
		out.WriteByte('\n')
	}
}

func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	y := (299*r + 587*g + 114*b) / 1000
	return y < 0x8000
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
