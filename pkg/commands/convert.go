/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"golang.org/x/image/draw"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Convert() *cobra.Command {
	var invert bool
	var scaleY float32
	impl := new(imageImpl)
	cmd := &cobra.Command{
		Use:   "convert IMAGE_FILE",
		Short: "Convert an image to ESC/POS formatted output.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Only log to stderr.
			impl.stdout = cmd.ErrOrStderr()
			impl.stderr = cmd.ErrOrStderr()

			// File
			if len(args) != 1 {
				return fmt.Errorf("expected IMAGE_FILE")
			}
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			impl.in = f
			impl.done = append(impl.done, f.Close)
			impl.kind = filepath.Ext(args[0])
			impl.scaleY = scaleY
			if impl.scaleY == 0 {
				impl.scaleY = 1
			}
			impl.out = cmd.OutOrStdout()

			impl.invert = invert

			impl.max = 56 // only works on Font B. TODO: support Font A.
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			defer impl.Done()
			return impl.Convert(cmd.Context())
		},
	}
	cmd.Flags().BoolVar(&invert, "invert", false, "Invert the image.")
	cmd.Flags().Float32Var(&scaleY, "y-scale", 1.0, "Additional scale for Y axis. (Is multiplied to X axis scalar.)")
	return cmd
}

type imageImpl struct {
	done   []func() error
	stdout io.Writer
	stderr io.Writer

	in     io.Reader
	out    io.Writer
	max    int
	scaleY float32
	kind   string
	invert bool
}

func (i *imageImpl) Convert(ctx context.Context) error {
	var err error
	var src image.Image

	switch strings.ToLower(i.kind) {
	case "png":
		src, err = png.Decode(i.in)
		if err != nil {
			return err
		}
	case "jpeg", "jpg":
		src, err = jpeg.Decode(i.in)
		if err != nil {
			return err
		}
	default:
		src, _, err = image.Decode(i.in)
	}

	scale := src.Bounds().Max.X / i.max

	scaleY := int(float32(scale) * i.scaleY)
	// Set the expected size that you want:
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Max.X/scale, src.Bounds().Max.Y/(scaleY)))

	// Resize:
	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	levels := []string{" ", "░", "▒", "▓", "█"}

	img := dst

	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		fmt.Printf(`    "`)
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			level := c.Y / 51 // 51 * 5 = 255
			if level == 5 {
				level--
			}
			if !i.invert {
				level = 4 - level
			}
			fmt.Printf("%s", levels[level])
		}
		fmt.Printf(`" LF`)
		fmt.Print("\n")
	}
	return nil
}

// Done calls all the done functions.
func (i *imageImpl) Done() {
	for _, done := range i.done {
		if err := done(); err != nil {
			_, _ = fmt.Fprint(i.stderr, err.Error())
		}
	}
}
