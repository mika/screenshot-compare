package main

import (
	"path/filepath"
	"testing"
	"time"
)

var FILES map[string]string

func init() {
	FILES = make(map[string]string)
	folder := "tests/"

	FILES["black"] = "black.png"
	FILES["white"] = "white.png"
	FILES["g"] = "google_query_screenshot.png"
	FILES["g_transparent"] = "google_query_transparent.png"
	FILES["grmlforensic_website"] = "grmlforensic_contact_website.png"
	FILES["grml_kB"] = "grml_booting_totalmemory_kB.png"
	FILES["grml_MB"] = "grml_booting_totalmemory_MB.png"
	FILES["grmlf_bo_back"] = "grmlforensic_bootoptions_backtomainmenu.png"
	FILES["grmlf_bo_debug"] = "grmlforensic_bootoptions_debugmode.png"
	FILES["grmlf_bs_23"] = "grmlforensic_bootsplash_23sec.png"
	FILES["grmlf_bs_30"] = "grmlforensic_bootsplash_30sec.png"
	FILES["grmlf_bs_graphical"] = "grmlforensic_bootsplash_graphicalmode.png"
	FILES["grmlf_bs_transparent"] = "grmlforensic_bootsplash_selection_transparent.png"

	if folder != "" {
		for k, v := range FILES {
			FILES[k] = filepath.Join(folder, v)
		}
	}
}

func defaultSettings() Settings {
	return Settings{ColorSpace: "RGB", Timeout: time.Duration(0), Wait: time.Hour * 24}
}

func TestDurationSpecifier(t *testing.T) {
	test := func(teststr string, expected time.Duration) {
		dur, err := readDurationSpecifier(teststr)
		if err != nil {
			t.Log(err)
		}
		if dur != expected {
			t.Fatalf("'%s' was not recognized by duration specifier parser", teststr)
		}
	}

	// must not throw exception
	readDurationSpecifier("")

	test("1s", time.Second*1)
	test("30s", time.Second*30)
	test("500i", time.Millisecond*500)
	test("1000i", time.Second)
	test("30m", time.Minute*30)
	test("2h", time.Hour*2)
	test("5", time.Second*5)
}

func TestEqualImages(t *testing.T) {
	s := defaultSettings()
	s.BaseImg = FILES["g"]
	s.RefImg = FILES["g"]
	diff, err := CompareImages(s)
	if err != nil {
		t.Log(err)
	}
	if diff > 0.01 {
		t.Fatalf("Same image must return difference %f; got %f", 0.0, diff)
	}
}

func TestDifferentImages(t *testing.T) {
	s := defaultSettings()
	s.BaseImg = FILES["g"]
	s.RefImg = FILES["grmlforensic_website"]
	diff, err := CompareImages(s)
	if err != nil {
		t.Log(err)
	}
	if diff <= 0.1 {
		t.Fatalf("Different images must return high difference; got %f", diff)
	}
}

func TestTotallyDifferentImages(t *testing.T) {
	s := defaultSettings()
	s.BaseImg = FILES["black"]
	s.RefImg = FILES["white"]
	diff, err := CompareImages(s)
	if err != nil {
		t.Log(err)
	}
	if diff <= 0.9 {
		t.Fatalf("Totally different images must return very high difference; got %f", diff)
	}
}

func TestRGBAndYUV(t *testing.T) {
	s := defaultSettings()
	s.ColorSpace = "RGB"
	s.BaseImg = FILES["g"]
	s.RefImg = FILES["grmlforensic_website"]
	diffRGB, err := CompareImages(s)
	if err != nil {
		t.Log(err)
	}

	s.ColorSpace = "Y'UV"
	diffYUV, err := CompareImages(s)
	if err != nil {
		t.Log(err)
	}

	// I only expect them to be different
	if diffRGB == diffYUV {
		t.Fatalf("Expecting a difference between RGB and Y'UV; got %f and %f", diffRGB, diffYUV)
	}
}

func TestTransparency(t *testing.T) {
	s := defaultSettings()
	s.BaseImg = FILES["g"]
	s.RefImg = FILES["g_transparent"]
	diff, err := CompareImages(s)
	if err != nil {
		t.Log(err)
	}
	if diff > 0.01 {
		t.Fatalf("Base image must match given transparent reference image; got difference of %f", diff)
	}
}
