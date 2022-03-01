package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/draw"
)

func main() {
	catFile, err := os.Open("n3wscott.png")
	if err != nil {
		log.Fatal(err)
	}
	defer catFile.Close()

	// Consider using the general image.Decode as it can sniff and decode any registered image format.
	src, err := png.Decode(catFile)
	if err != nil {
		log.Fatal(err)
	}

	scale := src.Bounds().Max.X / 56
	fmt.Println("org:", src.Bounds().Max.X, "scale:", scale)

	// Set the expected size that you want:
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Max.X/scale, src.Bounds().Max.Y/(scale)))

	// Resize:
	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	//	fmt.Println(img)

	levels := []string{" ", "░", "▒", "▓", "█"}

	img := dst

	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			level := c.Y / 51 // 51 * 5 = 255
			if level == 5 {
				level--
			}
			level = 4 - level
			fmt.Printf("%s", levels[level])
		}
		fmt.Print("\n")
	}
}
