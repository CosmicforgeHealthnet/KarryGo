package geo

import (
	"errors"
	"math"
	"strings"
)

// PolylinePrecision is the number of decimal places Google's encoded polyline
// algorithm preserves. Routes API responses use 5; anything else would decode
// to the wrong scale.
const PolylinePrecision = 5

// ErrInvalidPolyline is returned when an encoded polyline ends mid-value or
// contains a character outside the encoding's range.
var ErrInvalidPolyline = errors.New("geo: malformed encoded polyline")

// DecodePolyline expands a Google encoded polyline into its points. An empty
// string decodes to nil with no error, so an unset route column is not an
// error condition.
func DecodePolyline(encoded string) ([]Point, error) {
	if encoded == "" {
		return nil, nil
	}

	factor := math.Pow10(PolylinePrecision)
	points := make([]Point, 0, len(encoded)/4)

	var lat, lng int
	for i := 0; i < len(encoded); {
		dLat, next, err := decodeValue(encoded, i)
		if err != nil {
			return nil, err
		}
		i = next

		dLng, next, err := decodeValue(encoded, i)
		if err != nil {
			return nil, err
		}
		i = next

		lat += dLat
		lng += dLng
		points = append(points, Point{
			Lat: float64(lat) / factor,
			Lng: float64(lng) / factor,
		})
	}
	return points, nil
}

// EncodePolyline compresses points into a Google encoded polyline. It is the
// exact inverse of DecodePolyline at PolylinePrecision.
func EncodePolyline(points []Point) string {
	if len(points) == 0 {
		return ""
	}

	factor := math.Pow10(PolylinePrecision)
	var buf strings.Builder
	buf.Grow(len(points) * 6)

	var prevLat, prevLng int
	for _, p := range points {
		lat := int(math.Round(p.Lat * factor))
		lng := int(math.Round(p.Lng * factor))
		encodeValue(&buf, lat-prevLat)
		encodeValue(&buf, lng-prevLng)
		prevLat, prevLng = lat, lng
	}
	return buf.String()
}

// decodeValue reads one varint-style value starting at index i and returns it
// along with the index just past it.
func decodeValue(encoded string, i int) (value int, next int, err error) {
	var shift uint
	var result int
	for {
		if i >= len(encoded) {
			return 0, 0, ErrInvalidPolyline
		}
		b := int(encoded[i]) - 63
		if b < 0 || b > 63 {
			return 0, 0, ErrInvalidPolyline
		}
		i++
		result |= (b & 0x1f) << shift
		shift += 5
		if b < 0x20 {
			break
		}
		if shift > 32 {
			return 0, 0, ErrInvalidPolyline
		}
	}
	if result&1 != 0 {
		result = ^(result >> 1)
	} else {
		result >>= 1
	}
	return result, i, nil
}

func encodeValue(buf *strings.Builder, value int) {
	v := value << 1
	if value < 0 {
		v = ^v
	}
	for v >= 0x20 {
		buf.WriteByte(byte((0x20 | (v & 0x1f)) + 63))
		v >>= 5
	}
	buf.WriteByte(byte(v + 63))
}
