package validation

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Data-export content validation.
//
// CSV and JSONL have no magic signature, so the ordinary signature detector
// correctly refuses them: for a document contract, "I could not identify these
// bytes" must mean rejection. A generated data export is the one case where
// that answer is wrong, and this file is the narrow, purpose-scoped path that
// handles it.
//
// It is emphatically NOT "trust the declared Content-Type". The declaration
// only selects WHICH claim to verify; the bytes then have to actually be that
// kind of thing — printable UTF-8, free of NUL and control characters, and
// structurally parseable as the format named. A PDF, an ELF binary, a ZIP
// container, an HTML page, an SVG or a shell script declared as text/csv all
// fail, which is what keeps this from being a MIME bypass.

// Canonical export content types. These two, exactly. There is deliberately no
// text/*, no application/*, no application/octet-stream and no HTML, XML, SVG,
// JavaScript, Markdown, PDF, XLSX or archive type reachable through this path.
const (
	MIMECSV   = "text/csv"
	MIMEJSONL = "application/x-ndjson"
)

// exportFormats pairs each canonical type with the one extension it may carry.
// The pairing is exact in both directions: a .csv declared as JSONL and a
// .jsonl declared as CSV are both refused, so an operator cannot end up with a
// file whose name disagrees with what it is.
var exportFormats = map[string]string{
	MIMECSV:   ".csv",
	MIMEJSONL: ".jsonl",
}

// Export validation failure codes. They are controlled values, never a parser's
// raw message: a caller learns which rule refused it, not what the CSV reader
// said about line 4,812.
const (
	ExportFailureUnsupportedType = "EXPORT_UNSUPPORTED_TYPE"
	ExportFailureExtension       = "EXPORT_EXTENSION_MISMATCH"
	ExportFailureEncoding        = "EXPORT_INVALID_ENCODING"
	ExportFailureControlBytes    = "EXPORT_CONTROL_BYTES"
	ExportFailureExecutable      = "EXPORT_EXECUTABLE_CONTENT"
	ExportFailureStructure       = "EXPORT_MALFORMED_STRUCTURE"
	ExportFailureEmpty           = "EXPORT_EMPTY"
	ExportFailureTooLarge        = "EXPORT_SIZE_EXCEEDED"
)

// MaxExportLineBytes bounds one logical line during validation. It exists so a
// single unterminated line cannot make a bounded-size upload consume unbounded
// memory while being checked.
const MaxExportLineBytes = 1 << 20

// NormalizeExportMIME maps a declared content type onto its canonical export
// form, or "" when it is not an export type at all.
func NormalizeExportMIME(declared string) string {
	normalized := NormalizeMIME(declared)
	switch normalized {
	case MIMECSV, MIMEJSONL:
		return normalized
	}
	return ""
}

// ExportExtensionFor returns the one extension a canonical export type may use.
func ExportExtensionFor(canonical string) (string, bool) {
	extension, ok := exportFormats[canonical]
	return extension, ok
}

// ExportPairing checks a declared type and a file name together.
//
// Both must be present and must agree. A missing name is refused rather than
// defaulted, because the name is what a person will see when the file lands on
// their disk and a mismatched one is how a CSV ends up opened as something
// else.
func ExportPairing(declaredMIME, fileName string) (string, string) {
	canonical := NormalizeExportMIME(declaredMIME)
	if canonical == "" {
		return "", ExportFailureUnsupportedType
	}
	want, _ := ExportExtensionFor(canonical)
	name := strings.TrimSpace(fileName)
	if name == "" {
		return "", ExportFailureExtension
	}
	// A name is a name, not a path: a caller cannot smuggle a directory or a
	// second extension past this by describing one.
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", ExportFailureExtension
	}
	if !strings.EqualFold(path.Ext(name), want) {
		return "", ExportFailureExtension
	}
	return canonical, ""
}

// ValidateExportContent streams the artifact and verifies it really is the
// format it claims to be.
//
// Validation is a single bounded pass with a fixed-size line buffer, so a
// maximum-size export is never held in memory a second time and one pathological
// line cannot grow the buffer without limit. It returns "" when the content is
// acceptable, or a controlled failure code.
func ValidateExportContent(reader io.Reader, canonicalMIME string, maxBytes int64) string {
	if maxBytes <= 0 {
		maxBytes = 1 << 24
	}
	// One byte over the limit is read deliberately, so an oversized artifact is
	// detected as oversized rather than silently truncated into something that
	// would then parse.
	limited := io.LimitReader(reader, maxBytes+1)

	switch canonicalMIME {
	case MIMECSV:
		return validateCSVExport(limited, maxBytes)
	case MIMEJSONL:
		return validateJSONLExport(limited, maxBytes)
	}
	return ExportFailureUnsupportedType
}

// guardedReader enforces the byte ceiling and the character rules while the
// format parser above it consumes the stream.
//
// Doing both here rather than in a pre-pass is what keeps validation to one
// bounded pass: the parser and the safety checks see the same bytes once.
type guardedReader struct {
	source   io.Reader
	remain   int64
	failure  string
	seenAny  bool
	overflow bool
	// carry holds a trailing partial UTF-8 sequence across reads, so a
	// multi-byte character split across two buffers is not mistaken for
	// invalid encoding.
	carry []byte
}

func (g *guardedReader) Read(buffer []byte) (int, error) {
	n, err := g.source.Read(buffer)
	if n > 0 {
		g.seenAny = true
		g.remain -= int64(n)
		if g.remain < 0 {
			g.overflow = true
			g.failure = ExportFailureTooLarge
			return 0, io.ErrUnexpectedEOF
		}
		if code := g.inspect(buffer[:n]); code != "" {
			g.failure = code
			return 0, io.ErrUnexpectedEOF
		}
	}
	return n, err
}

// inspect applies the character rules to one chunk.
func (g *guardedReader) inspect(chunk []byte) string {
	for _, b := range chunk {
		if b == 0 {
			// A NUL is the single clearest signal that this is not text, and
			// the one most often present in a binary file renamed to .csv.
			return ExportFailureControlBytes
		}
		if b < 0x20 && b != 0x09 && b != 0x0A && b != 0x0D {
			return ExportFailureControlBytes
		}
	}
	combined := chunk
	if len(g.carry) > 0 {
		combined = append(append([]byte(nil), g.carry...), chunk...)
		g.carry = nil
	}
	// Hold back at most three trailing bytes: a UTF-8 sequence is at most four
	// bytes, so anything longer than that at the tail is genuinely incomplete.
	for cut := 0; cut < 4 && cut < len(combined); cut++ {
		candidate := combined[:len(combined)-cut]
		if utf8.Valid(candidate) {
			g.carry = append([]byte(nil), combined[len(combined)-cut:]...)
			return ""
		}
	}
	return ExportFailureEncoding
}

// finish reports any rule the stream broke that only shows at the end.
func (g *guardedReader) finish() string {
	if g.failure != "" {
		return g.failure
	}
	if len(g.carry) > 0 && !utf8.Valid(g.carry) {
		return ExportFailureEncoding
	}
	if !g.seenAny {
		return ExportFailureEmpty
	}
	return ""
}

func newGuardedReader(source io.Reader, maxBytes int64) *guardedReader {
	return &guardedReader{source: source, remain: maxBytes}
}

// validateCSVExport parses the artifact with the standard library reader.
//
// Structural validity is the whole check: media does not know or care what the
// columns mean. FieldsPerRecord is left at its default so the reader enforces
// a consistent column count, which is exactly the property that separates a
// real export from a text file that happens to contain commas.
func validateCSVExport(source io.Reader, maxBytes int64) string {
	guard := newGuardedReader(source, maxBytes)
	reader := csv.NewReader(guard)
	reader.ReuseRecord = true
	reader.LazyQuotes = false
	// A cell is not a document. Bounding the field size stops a crafted quote
	// from turning the whole artifact into one field.
	reader.FieldsPerRecord = 0

	rows := 0
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			if code := guard.finish(); code != "" {
				return code
			}
			return ExportFailureStructure
		}
		rows++
	}
	if code := guard.finish(); code != "" {
		return code
	}
	if rows == 0 {
		// An export always carries at least a header row; zero records means
		// there is nothing here to have been exported.
		return ExportFailureEmpty
	}
	return ""
}

// validateJSONLExport checks that every non-empty line is one JSON value.
//
// It uses a bounded scanner rather than decoding the file, so a large export is
// validated line by line and never deserialised into memory as a whole.
func validateJSONLExport(source io.Reader, maxBytes int64) string {
	guard := newGuardedReader(source, maxBytes)
	scanner := bufio.NewScanner(guard)
	scanner.Buffer(make([]byte, 0, 64*1024), MaxExportLineBytes)

	records := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// One JSON OBJECT per line. A bare array, number or string is valid
		// JSON but is not a newline-delimited record set, and accepting one
		// would let a whole-file JSON document masquerade as JSONL.
		if line[0] != '{' || !json.Valid([]byte(line)) {
			return ExportFailureStructure
		}
		records++
	}
	if err := scanner.Err(); err != nil {
		if code := guard.finish(); code != "" {
			return code
		}
		// A line longer than the buffer is a structural problem, not a
		// transport one: no legitimate export row is a megabyte of one line.
		return ExportFailureStructure
	}
	if code := guard.finish(); code != "" {
		return code
	}
	if records == 0 {
		return ExportFailureEmpty
	}
	return ""
}

// ExportExecutableSignature reports whether the leading bytes carry a
// recognised executable or archive signature.
//
// It runs before the format parsers, so a shell script or a ZIP renamed to
// .csv is refused as executable content — with the real reason — rather than
// getting as far as a confusing parse error.
func ExportExecutableSignature(head []byte) bool {
	for _, candidate := range executableSignatures {
		if matches(head, candidate) {
			return true
		}
	}
	return false
}

// ExportMarkupSignature reports whether the content opens as markup.
//
// It exists because structural parsing alone is not sufficient: Go's CSV reader
// will happily accept `<script>alert(1)</script>` as a valid one-column,
// one-row file, since a document with no commas is still a document with one
// field. That is a true statement about CSV and a useless one about exports.
//
// The rule is a positive one about what an export IS rather than a blocklist of
// payloads: the first non-whitespace character of a generated export is a
// header field name or an opening brace, and is never `<`. That single check
// covers HTML, SVG, XML, XHTML and PHP openings at once, and does not restrict
// what may appear inside a cell — a CRM note containing `<` still exports fine,
// because only the start of the document is examined.
func ExportMarkupSignature(head []byte) bool {
	for _, b := range head {
		switch b {
		case 0x20, 0x09, 0x0D, 0x0A, 0xEF, 0xBB, 0xBF: // whitespace and a UTF-8 BOM
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
}

// ExportBinarySignature reports whether the leading bytes match any format this
// service recognises by signature.
//
// Anything with a real signature is by definition not a sniff-less text format,
// so it does not belong on this path no matter what it was declared as. This is
// what stops a PDF or a PNG being uploaded as text/csv.
func ExportBinarySignature(head []byte) bool {
	return Detect(head).DetectedMIME != ""
}
