package evidenceimage

import (
	"bytes"
	"cosmicforge/logistics/shared/go/apperrors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
)

const MaxBytes = 5 << 20

func Read(reader io.Reader, size int64) ([]byte, string, error) {
	if size <= 0 || size > MaxBytes {
		return nil, "", apperrors.BadRequest("Choose a JPG or PNG photo smaller than 5 MB.", nil)
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxBytes+1))
	if err != nil || int64(len(data)) != size {
		return nil, "", apperrors.BadRequest("The photo could not be read completely.", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 40000000 {
		return nil, "", apperrors.BadRequest("Choose a valid JPG or PNG photo.", err)
	}
	return data, "image/" + format, nil
}
