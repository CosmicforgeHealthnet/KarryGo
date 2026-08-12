// Package validation inspects actual stored bytes.
//
// Nothing here consults a filename, an extension or the client's declared
// content type to decide what a file IS. Those are inputs to a comparison, not
// sources of truth: an attacker controls all three, and a check that trusts
// them is a check that can be told the answer.
//
// What the package does trust is the leading bytes of the file, which is the
// only part of the input that has to be correct for the file to be usable as
// the type it claims to be.
package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"mime"
	"strings"
)

// Detection is the outcome of inspecting a file's bytes.
type Detection struct {
	// DetectedMIME is the type the bytes actually are, or "" when the leading
	// bytes match no signature this service recognises.
	DetectedMIME string
	// Executable reports whether the bytes carry a recognised executable or
	// script-container signature.
	Executable bool
}

// signature is one magic-byte rule.
type signature struct {
	mimeType string
	offset   int
	prefix   []byte
	// verify runs after the prefix match for container formats where the
	// prefix alone is ambiguous (RIFF is WEBP, WAVE or AVI).
	verify func(head []byte) bool
}

var signatures = []signature{
	{mimeType: "application/pdf", prefix: []byte("%PDF-")},
	{mimeType: "image/png", prefix: []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}},
	{mimeType: "image/jpeg", prefix: []byte{0xFF, 0xD8, 0xFF}},
	{mimeType: "image/gif", prefix: []byte("GIF87a")},
	{mimeType: "image/gif", prefix: []byte("GIF89a")},
	{mimeType: "image/webp", prefix: []byte("RIFF"), verify: func(head []byte) bool {
		return len(head) >= 12 && bytes.Equal(head[8:12], []byte("WEBP"))
	}},
	{mimeType: "image/tiff", prefix: []byte{0x49, 0x49, 0x2A, 0x00}},
	{mimeType: "image/tiff", prefix: []byte{0x4D, 0x4D, 0x00, 0x2A}},
}

// executableSignatures are refused wherever executable content is prohibited.
//
// The ZIP signature is here rather than in signatures because a ZIP container
// is also what a modern Office document is. This service does not accept
// either on the private document contract, so treating the container as
// unsupported is both correct and simpler than unpacking it to find out.
var executableSignatures = []signature{
	{mimeType: "application/x-msdownload", prefix: []byte("MZ")},
	{mimeType: "application/x-elf", prefix: []byte{0x7F, 'E', 'L', 'F'}},
	{mimeType: "application/x-mach-binary", prefix: []byte{0xCF, 0xFA, 0xED, 0xFE}},
	{mimeType: "application/x-mach-binary", prefix: []byte{0xFE, 0xED, 0xFA, 0xCF}},
	{mimeType: "application/java-vm", prefix: []byte{0xCA, 0xFE, 0xBA, 0xBE}},
	{mimeType: "application/x-sh", prefix: []byte("#!")},
	{mimeType: "application/zip", prefix: []byte("PK\x03\x04")},
}

// Detect classifies the leading bytes of a file.
//
// head should be at least the first 512 bytes; a shorter slice still works and
// simply matches fewer signatures.
func Detect(head []byte) Detection {
	for _, candidate := range executableSignatures {
		if matches(head, candidate) {
			return Detection{DetectedMIME: candidate.mimeType, Executable: true}
		}
	}
	for _, candidate := range signatures {
		if matches(head, candidate) {
			return Detection{DetectedMIME: candidate.mimeType}
		}
	}
	return Detection{}
}

func matches(head []byte, candidate signature) bool {
	end := candidate.offset + len(candidate.prefix)
	if len(head) < end {
		return false
	}
	if !bytes.Equal(head[candidate.offset:end], candidate.prefix) {
		return false
	}
	if candidate.verify != nil {
		return candidate.verify(head)
	}
	return true
}

// NormalizeMIME lowercases a content type and strips its parameters, so
// "Application/PDF; charset=binary" and "application/pdf" compare equal.
func NormalizeMIME(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(value)
	}
	return strings.ToLower(parsed)
}

// Checksum returns the lowercase hex SHA-256 of the supplied bytes.
func Checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// HeadSize is how many leading bytes signature detection inspects.
const HeadSize = 512

// Head returns the leading bytes used for detection.
func Head(content []byte) []byte {
	if len(content) > HeadSize {
		return content[:HeadSize]
	}
	return content
}
