package verification

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
)

// PrescreenResult is a collection of mechanical image checks, not biometric
// face comparison, presentation-attack detection, or proof of identity.
type PrescreenResult struct {
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
}

var ErrLowQualitySelfie = errors.New("selfie needs more light, focus or resolution")

// inspectSelfieImage bounds decompression and samples image brightness and
// contrast. A submitted photo is never automatically considered a face match.
func inspectSelfieImage(body io.Reader) (PrescreenResult, error) {
	const maxImageBytes = maxSelfieFileSize + 1
	limited := io.LimitReader(body, maxImageBytes)
	data, err := io.ReadAll(limited)
	if err != nil || len(data) < 1024 || int64(len(data)) > maxSelfieFileSize {
		return PrescreenResult{}, ErrLowQualitySelfie
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return PrescreenResult{}, ErrLowQualitySelfie
	}
	if config.Width < 240 || config.Height < 240 || config.Width > 4096 || config.Height > 4096 ||
		int64(config.Width)*int64(config.Height) > 12_000_000 {
		return PrescreenResult{}, ErrLowQualitySelfie
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return PrescreenResult{}, ErrLowQualitySelfie
	}

	bounds := picture.Bounds()
	stepX := int(math.Max(1, float64(bounds.Dx()/80)))
	stepY := int(math.Max(1, float64(bounds.Dy()/80)))
	var sum, squared float64
	var count float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, _ := picture.At(x, y).RGBA()
			lum := 0.2126*float64(r)/257.0 +
				0.7152*float64(g)/257.0 +
				0.0722*float64(b)/257.0
			sum += lum
			squared += lum * lum
			count++
		}
	}
	if count < 1 {
		return PrescreenResult{}, ErrLowQualitySelfie
	}
	mean := sum / count
	variance := math.Max(0, squared/count-mean*mean)
	std := math.Sqrt(variance)
	if mean < 30 || mean > 245 || std < 10 {
		return PrescreenResult{Width: config.Width, Height: config.Height, Brightness: mean, Contrast: std}, ErrLowQualitySelfie
	}
	return PrescreenResult{Width: config.Width, Height: config.Height, Brightness: mean, Contrast: std}, nil
}
