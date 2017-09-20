package main

import (
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	_ "image/png"
)

const USAGE = `
Usage: ./subimage <baseimage.png> <refimage.png> <timeout>
  Compare baseimage with refimage within timeout.

  Returns an exit code in [0,101]
     101 ... timeout reached
     100 ... no equivalence (too different, different dimensions)
     0   ... no differences (every pixel has same RGB value)

  Timeout needs to be an integer followed by a flag; one of 'ism'.
  e.g. '2s' or '2' means 2 seconds, i for milliseconds and m for minute

  Pixels with a positive alpha value are compared in relation
  to the alpha value. A transparent image of same dimension
  is equivalent to every other image. Alpha values in the base image are ignored.
  The implementation uses floating point number; subject to rounding errors
`

// Img represents an image with explicit
// width and height values
type Img struct {
	i *image.NRGBA
	w int
	h int
}

// Match represents a subimage match rated by a score
type Match struct {
	s float32
	r image.Rectangle
}

// parseDuration takes a user-specified string and tries
// to interpret it as duration
func parseDuration(spec string) time.Duration {
	if sec, err := strconv.Atoi(spec); err == nil {
		return time.Duration(sec) * time.Second
	}

	val, err := strconv.Atoi(spec[:len(spec)-1])
	if err != nil {
		log.Fatal("invalid interval specified - '" + spec + "', I need a string \\d+[ismh]")
		return time.Duration(0)
	}

	switch spec[len(spec)-1] {
	case 'i':
		return time.Duration(val) * time.Millisecond
	case 's':
		return time.Duration(val) * time.Second
	case 'm':
		return time.Duration(val) * time.Minute
	case 'h':
		return time.Duration(val) * time.Hour
	}

	log.Panic("invalid interval duration specifier - '" + spec[len(spec)-1:] + "', must be one of 'ismh'")
	return time.Duration(0)
}

// readImage takes a filepath and reads
// the image into the memory. It returns
// the corresponding Img struct.
func readImage(filepath string) Img {
	// read file
	reader, err := os.Open(filepath)
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()
	img, format, err := image.Decode(reader)
	if err != nil {
		log.Fatal(err)
	}

	// width & height
	width := img.Bounds().Max.X
	height := img.Bounds().Max.Y

	// now convert to NRGBA discarding alpha pre-multiplication
	nrgbaImage := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			nrgbaImage.Set(x, y, img.At(x, y))
		}
	}

	// return instance
	log.Printf("%s image (%d×%d) read\n", format, width, height)
	return Img{nrgbaImage, width, height}
}

// compareRow compares row pixels of base and sub images.
// It returns an equivalence score between 0 (eq) and 100 (diff).
func compareRow(base, sub Img, row int, wait *sync.WaitGroup, result *float32) {
	defer wait.Done()

	var scale float64 = 42949672.96
	var r, g, b uint32
	var refR, refG, refB, refA uint32
	var diff float64
	var sum float64

	for x := 0; x < sub.w; x++ {
		r, g, b, _ = base.i.NRGBAAt(x, row).RGBA()
		refR, refG, refB, refA = sub.i.NRGBAAt(x, row).RGBA()
		diff = (math.Abs(float64(refR-r)) + math.Abs(float64(refG-g)) + math.Abs(float64(refB-b))) / 3
		val := (diff / scale) * (float64(refA) / 4294967296.0)
		if val > 100.0 {
			val = 100.0
		}
		sum += val
	}

	*result = float32(sum / float64(sub.w))
}

func main() {
	var w sync.WaitGroup
	d := make(chan bool)

	start := time.Now()

	// CLI options
	if len(os.Args) != 4 {
		fmt.Println(USAGE)
		os.Exit(1)
	}

	// preparing
	base := readImage(os.Args[1])
	sub := readImage(os.Args[2])
	timeout := parseDuration(os.Args[3])

	// check equivalence in every row
	if base.w != sub.w || base.h != sub.h {
		fmt.Printf("  Base image: %d×%d\n", base.w, base.h)
		fmt.Printf("  Sub image:  %d×%d\n", sub.w, sub.h)
		fmt.Println("Dimensions do not correspond")
		os.Exit(100)
	}
	rowEqs := make([]float32, base.h)
	w.Add(base.h)
	go func() {
		for row := 0; row < base.h; row++ {
			compareRow(base, sub, row, &w, &rowEqs[row])
		}
	}()

	// one goroutine waits for the result
	go func() {
		w.Wait()
		d <- true
	}()

	// either interrupt or wait for result
	timedout := false
wait:
	for {
		select {
		case <-d:
			break wait
		case <-time.After(timeout):
			timedout = true
			break wait
		}
	}

	// determine equivalence score
	var eq float64
	var score int = 128
	for row := 0; row < base.h; row++ {
		eq += float64(rowEqs[row])
	}

	// <screenshot-specific code>
	// makes matching more sensitive
	CORRECTION := float64(base.h * 10000)
	eq *= CORRECTION
	// </screenshot-specific code>

	eq /= float64(base.h)
	for i := 0; i < 100; i++ {
		if float64(i) <= eq && eq < float64(i+1) {
			score = i
		}
	}
	if score == 128 {
		score = 100
	}

	// output
	end := time.Now()
	fmt.Printf("runtime: %v\n", end.Sub(start))
	fmt.Printf("score:   %f\n", eq)

	if timedout {
		fmt.Println("timeout")
		os.Exit(101)
	} else {
		fmt.Println("finished")
		os.Exit(score)
	}
}
