package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const USAGE = `
USAGE

./compareimage [--colors <colorspace> | --timeout <S> | --wait <S>] <base> <ref>

DESCRIPTION

Compare two images and quantify their difference.

OPTIONS

--colors
  defines the color space.

<colorspace> is one of "RGB" (default) or "Y'UV"
  RGB is the standard color model.
  "Y'UV" resembles the perception of the colors by the eye better.
  Hence the differences better quantify the visual differences.

--timeout with default '0s' (special meaning: infinity)
  assigns a maximum runtime for this program.

<S> matches '\d+[ismh]'
  is a duration specifier. The prefix defines the value.
  The suffix defines the unit. Examples:
    '600i'   600 milliseconds       '2s'    2 seconds
    '1m'     1 minute               '24h'   24 hours

--wait with default '0s'
  defines how long the program should wait before reading
  the image files.

<base> is a required positional argument
  is a filepath to the base image (contains no transparency)

<ref> is a required positional argument
  is a filepath to the reference image (optionally contains transparency)

REMARKS

Scoring uses a 64-bit floating point number.
So this program is subject to floating point rounding errors.

RETURN CODE

The return code is an integer with min. 0 and max. 102:
  0     no differences (every pixel has same RGB value)
  100   high difference
  101   invalid arguments OR dimensions do not correspond
  102   timeout reached
`

const WR = float64(0.299)
const WG = float64(0.587)
const WB = float64(0.114)

// Settings defines the application settings
type Settings struct {
	ColorSpace string
	Timeout    time.Duration
	Wait       time.Duration
	BaseImg    string
	RefImg     string
}

// img represents an image with explicit width and height values
type img struct {
	i image.Image
	w int
	h int
	f string
}

// difference stores a difference measure for two images
type difference struct {
	score               float64
	minValue            float64
	maxValue            float64
	roundingErrorFactor float64
}

// readDurationSpecifier takes a human-readable duration specifier
// like '12s' and returns `time.Second * 12`
func readDurationSpecifier(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	errmsg := "invalid duration specifier; expected integer and one of 'ismh'; got '%s'"

	if s == "" {
		return time.Duration(0), fmt.Errorf(errmsg, s)
	}

	end := len(s) - 1
	if '0' <= s[end] && s[end] <= '9' {
		end += 1
	}

	val, err := strconv.Atoi(s[:end])
	if err != nil {
		return time.Duration(0), fmt.Errorf(errmsg, s)
	}

	if '0' <= s[len(s)-1] && s[len(s)-1] <= '9' {
		return time.Duration(val) * time.Second, nil
	}

	switch s[len(s)-1] {
	case 'i':
		return time.Duration(val) * time.Millisecond, nil
	case 's':
		return time.Duration(val) * time.Second, nil
	case 'm':
		return time.Duration(val) * time.Minute, nil
	case 'h':
		return time.Duration(val) * time.Hour, nil
	}

	return time.Duration(0), fmt.Errorf(errmsg, s)
}

// parseArguments takes `args` and fills `Settings` with its data
func parseArguments(s *Settings, args []string) error {
	// key in '--key value'
	var key string

	for _, a := range args {
		if key != "" {
			switch key {
			case "colors":
				s.ColorSpace = a
			case "timeout":
				dur, err := readDurationSpecifier(a)
				if err != nil {
					return err
				}
				s.Timeout = dur
			case "wait":
				dur, err := readDurationSpecifier(a)
				if err != nil {
					return err
				}
				s.Wait = dur
			}
			key = ""
		} else if len(a) > 2 && a[0:2] == "--" {
			key = strings.ToLower(strings.TrimSpace(a[2:]))
			if key != "colors" && key != "wait" && key != "timeout" {
				return fmt.Errorf("unknown argument '%s'", a)
			}
		} else if s.BaseImg == "" {
			s.BaseImg = a
		} else if s.RefImg == "" {
			s.RefImg = a
		} else {
			return fmt.Errorf("unknown positional argument '%s'", a)
		}
	}

	if s.RefImg == "" {
		count := 0
		if s.BaseImg != "" {
			count += 1
		}
		return fmt.Errorf("expected 2 positional arguments; baseimage and reference image; got %d", count)
	}

	if s.ColorSpace != "Y'UV" && s.ColorSpace != "RGB" {
		return fmt.Errorf("unknown color space '%s'", s.ColorSpace)
	}

	return nil
}

// readImageMetadata reads metadata about the image like width, height and the format
func readImageMetadata(filepath string, i *img) error {
	reader, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer reader.Close()
	decoded, format, err := image.Decode(reader)
	if err != nil {
		return err
	}

	// width & height
	i.w = decoded.Bounds().Max.X
	i.h = decoded.Bounds().Max.Y
	i.i = decoded
	i.f = format

	return nil
}

// toNRGBA converts a RGBA color to un-alpha-scaled NRGBA
// based on https://golang.org/src/image/color/color.go?s=4600:4767
func toNRGBA(r, g, b, a uint32) (float64, float64, float64, float64) {
	d := float64(a)
	if d == 0.0 {
		// if transparent, return black, not NaN
		return 0.0, 0.0, 0.0, 0.0
	}
	return float64(r*0xFFFF) / d, float64(g*0xFFFF) / d, float64(b*0xFFFF) / d, d
}

// toYUV converts a RGB color to the Y'UV color space
func toYUV(r, g, b float64) (float64, float64, float64) {
	// https://en.wikipedia.org/wiki/YUV#SDTV_with_BT.601
	y_ := WR*r + WG*g + WB*b
	return y_, 0.492 * (b - y_), 0.877 * (r - y_)
}

func euclideanDistance(a, x, b, y, c, z float64) float64 {
	return math.Sqrt(math.Pow(a-x, 2) + math.Pow(b-y, 2) + math.Pow(c-z, 2))
}

// compareImages determines the difference score for two images
// `baseImg` and `refImg` beginning at y-coordinate `yOffset`
// for `yCount` y-coordinates.
func compareImages(s *Settings, baseImg, refImg *img, yOffset, yCount int) (difference, error) {
	var diff difference
	diff.minValue = 0.0
	diff.maxValue = 1.0
	diff.roundingErrorFactor = 1.25

	cul := 0.0
	for y := yOffset; y < yOffset+yCount; y++ {
		for x := 0; x < baseImg.w; x++ {
			var d float64
			r1, g1, b1, _ := toNRGBA(baseImg.i.At(x, y).RGBA())
			r2, g2, b2, a2 := toNRGBA(refImg.i.At(x, y).RGBA())
			//log.Println(y, x, ":", "(1)", r1, g1, b1, a1, "(2)", r2, g2, b2, a2)

			switch s.ColorSpace {
			case "RGB":
				d = euclideanDistance(r1, r2, g1, g2, b1, b2) / 113510.0
			case "Y'UV":
				y_1, u1, v1 := toYUV(r1, g1, b1)
				y_2, u2, v2 := toYUV(r2, g2, b2)
				d = euclideanDistance(y_1, y_2, u1, u2, v1, v2) / 113510.0
			}

			// NOTE only alpha channel of refImg is considered
			alpha := a2 / 65535
			if alpha < 0.0 || alpha > 1.0 {
				panic(alpha) // should not occur
			}
			//log.Println(y, x, ":", d, alpha)
			cul += d * alpha
		}
	}

	diff.score = cul / float64(yCount*baseImg.w) * diff.roundingErrorFactor
	if diff.score > 1.0 {
		diff.score = 1.0
	}
	return diff, nil
}

func CompareImages(s Settings) (float64, error) {
	var baseImg, refImg img
	if err := readImageMetadata(s.BaseImg, &baseImg); err != nil {
		return 1.0, err
	}
	if err := readImageMetadata(s.RefImg, &refImg); err != nil {
		return 1.0, err
	}
	if baseImg.w != refImg.w || baseImg.h != refImg.h {
		msg := "image dimensions do not correspond; got %d×%d (base) and %d×%d (ref)\n"
		return 1.0, fmt.Errorf(msg, baseImg.w, baseImg.h, refImg.w, refImg.h)
	}
	diff, err := compareImages(&s, &baseImg, &refImg, 0, baseImg.h)
	return diff.score, err
}

func main() {
	var s Settings
	s.ColorSpace = "RGB"
	var diff difference

	start := time.Now()

	// CLI
	if err := parseArguments(&s, os.Args[1:]); err != nil {
		fmt.Printf("invalid arguments: %s\n", err.Error())
		fmt.Println(USAGE)
		os.Exit(101)
	}

	// wait option
	if s.Wait > time.Duration(0) {
		time.Sleep(s.Wait)
	}

	// timeout setup
	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(s.Timeout)
		if s.Timeout > time.Duration(0) {
			timeout <- false
		}
	}()

	go func() {
		// image metadata
		var err error
		var baseImg img
		if err := readImageMetadata(s.BaseImg, &baseImg); err != nil {
			log.Fatal(err)
		}
		var refImg img
		if err := readImageMetadata(s.RefImg, &refImg); err != nil {
			log.Fatal(err)
		}
		if baseImg.w != refImg.w || baseImg.h != refImg.h {
			msg := "image dimensions do not correspond; got %d×%d (base) and %d×%d (ref)\n"
			log.Printf(msg, baseImg.w, baseImg.h, refImg.w, refImg.h)
			os.Exit(101)
		}

		// processing
		diff, err = compareImages(&s, &baseImg, &refImg, 0, baseImg.h)
		if err != nil {
			log.Fatal(err)
			os.Exit(101)
		}

		timeout <- true
	}()

	// print result
	if <-timeout {
		percent := float64(100*diff.score-diff.minValue) / (diff.maxValue - diff.minValue)
		fmt.Printf("difference percentage:  %.3f %%\n", percent)
		fmt.Printf("runtime:                %s\n", time.Now().Sub(start))

		os.Exit(int(percent))
	} else {
		fmt.Printf("program timed out within %s\n", s.Timeout)
		os.Exit(102)
	}
}
