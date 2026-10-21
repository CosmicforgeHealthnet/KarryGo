package verification

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func prescreenFixture(t *testing.T, kind string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 320; x++ {
			val := uint8(120)
			switch kind {
			case "dark": val = 5
			case "bright": val = 252
			case "gradient": val = uint8(55 + ((x*5 + y*11 + x*y/70) % 145))
			}
			img.Set(x, y, color.RGBA{R: val, G: val, B: val, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 88}); err != nil { t.Fatal(err) }
	return out.Bytes()
}

func TestAutomatedSelfiePrescreenRejectsObviousFailures(t *testing.T) {
	for _, kind := range []string{"flat", "dark", "bright"} {
		t.Run(kind, func(t *testing.T) {
			_, err := inspectSelfieImage(bytes.NewReader(prescreenFixture(t, kind)))
			if err == nil { t.Fatalf("%s selfie unexpectedly passed", kind) }
		})
	}
}

func TestAutomatedSelfiePrescreenAllowsDetailedPhotoWithoutIdentityDecision(t *testing.T) {
	result, err := inspectSelfieImage(bytes.NewReader(prescreenFixture(t, "gradient")))
	if err != nil { t.Fatalf("quality precheck failed: %v", err) }
	if result.Width != 320 || result.Height != 320 { t.Fatalf("dimensions=%+v", result) }
	if result.Contrast < 10 { t.Fatalf("contrast=%v", result.Contrast) }
}

func TestAutomatedSelfiePrescreenRejectsCorruptImage(t *testing.T) {
	_, err := inspectSelfieImage(bytes.NewReader(bytes.Repeat([]byte{0xff, 0xd8, 0x00}, 600)))
	if err == nil { t.Fatal("corrupt image should not pass") }
}
