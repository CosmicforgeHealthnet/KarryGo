// Package canonical produces the byte-stable form of a pricing record and its
// SHA-256 seal.
//
// A quote must be provable months later: an auditor re-runs the stored inputs
// against the stored policy version and must get not merely the same amount but
// the same bytes. That requires one and only one encoding of any given value, so
// this package sorts object keys, forbids floating-point numbers outright, and
// requires timestamps to be pre-formatted in UTC.
//
// The rule is precisely "no ambiguous number": any value whose JSON form carries
// a fraction or an exponent is refused, because an IEEE double cannot represent
// most decimal fractions exactly and the same 1.35 surcharge can serialize
// differently on different paths. A float that is exactly integral (5.0) encodes
// as "5" and is accepted, sealing identically to the integer 5 -- which is the
// property that matters. Coordinates are the one unavoidable fractional input,
// and Micro converts them to fixed-point integers at the boundary.
package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Marshal returns the canonical JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical: encode: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber() // keeps digits as written instead of routing through float64
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("canonical: decode: %w", err)
	}

	var out bytes.Buffer
	if err := write(&out, value, ""); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Hash returns the hex SHA-256 of the canonical encoding of v. This is the
// value stored as a quote's input, output or record hash.
func Hash(v any) (string, error) {
	encoded, err := Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Combine seals a record that is built from hashes already computed, such as a
// quote hash over its input and output hashes. Parts are joined with a
// separator that cannot occur in a hex digest, so no two different part lists
// can produce the same input.
func Combine(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// Time formats t for inclusion in a canonical record: RFC 3339 with nanosecond
// precision, always in UTC. Callers must use this rather than embedding a
// time.Time, because the same instant in two zones would otherwise seal
// differently.
func Time(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Micro converts degrees to fixed-point microdegrees (about 11cm at the
// equator), rounding half away from zero.
//
// This is the only place a float64 is allowed to enter a canonical record.
// Coordinates arrive from callers as floats, and two callers can hold
// imperceptibly different doubles for the same point; rounding to a fixed grid
// here means the same requested place always seals identically.
func Micro(degrees float64) int64 {
	if math.IsNaN(degrees) || math.IsInf(degrees, 0) {
		return 0
	}
	return int64(math.Round(degrees * 1e6))
}

func write(out *bytes.Buffer, value any, path string) error {
	switch typed := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if typed {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Errorf("canonical: string at %s: %w", displayPath(path), err)
		}
		out.Write(encoded)
	case json.Number:
		text := typed.String()
		if strings.ContainsAny(text, ".eE") {
			return fmt.Errorf(
				"canonical: %s is the non-integer number %s; money must be whole kobo, "+
					"rates integers over an integer unit, and coordinates microdegrees",
				displayPath(path), text)
		}
		out.WriteString(text)
	case float64, float32:
		// Unreachable while UseNumber is set, but stated explicitly so that a
		// future change to the decoder cannot quietly admit floats.
		return fmt.Errorf("canonical: %s is a floating-point number, which is never canonical", displayPath(path))
	case []any:
		out.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := write(out, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			encodedKey, err := json.Marshal(key)
			if err != nil {
				return fmt.Errorf("canonical: key at %s: %w", displayPath(path), err)
			}
			out.Write(encodedKey)
			out.WriteByte(':')
			if err := write(out, typed[key], path+"."+key); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("canonical: %s has unsupported type %T", displayPath(path), value)
	}
	return nil
}

func displayPath(path string) string {
	if path == "" {
		return "the value"
	}
	return strings.TrimPrefix(path, ".")
}
