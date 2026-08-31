package validation

import (
	"bytes"
	"strings"
	"testing"
)

// Data-export validation tests.
//
// The property under test throughout is that the declared content type selects
// WHICH claim gets verified and never substitutes for the verification. Every
// refusal case below is something that declared itself a legitimate export.

func validateExport(content, declaredMIME, fileName string, maxBytes int64) string {
	canonical, failure := ExportPairing(declaredMIME, fileName)
	if failure != "" {
		return failure
	}
	head := Head([]byte(content))
	if ExportExecutableSignature(head) {
		return ExportFailureExecutable
	}
	if ExportBinarySignature(head) || ExportMarkupSignature(head) {
		return ExportFailureUnsupportedType
	}
	return ValidateExportContent(bytes.NewReader([]byte(content)), canonical, maxBytes)
}

func TestExport_AcceptsGenuineCSVAndJSONL(t *testing.T) {
	cases := map[string]struct{ content, mime, name string }{
		"csv with header and rows": {
			"profile_ref,status\ncrmp_1,ACTIVE\ncrmp_2,ARCHIVED\n", "text/csv", "customer_summary.csv"},
		"csv header only": {"profile_ref,status\n", "text/csv", "empty_summary.csv"},
		"csv with quoted commas": {
			"name,note\n\"Acme, Ltd\",\"line one\"\n", "text/csv", "corporate.csv"},
		"csv with charset parameter": {
			"a,b\n1,2\n", "text/csv; charset=utf-8", "scores.csv"},
		"csv with crlf":      {"a,b\r\n1,2\r\n", "text/csv", "scores.csv"},
		"csv with non-ascii": {"name\nCafé Ada\n", "text/csv", "names.csv"},
		"jsonl": {
			"{\"profile_ref\":\"crmp_1\"}\n{\"profile_ref\":\"crmp_2\"}\n",
			"application/x-ndjson", "scores.jsonl"},
		"jsonl with blank lines": {
			"{\"a\":1}\n\n{\"b\":2}\n", "application/x-ndjson", "scores.jsonl"},
	}
	for name, testCase := range cases {
		if failure := validateExport(testCase.content, testCase.mime, testCase.name, 1<<20); failure != "" {
			t.Errorf("%s: refused with %s", name, failure)
		}
	}
	// No repository-established alias exists, so near-miss spellings are
	// refused rather than widening the exact two-type contract.
	for _, alias := range []string{"application/jsonl", "application/x-jsonlines", "application/x-ndjson+json"} {
		if canonical := NormalizeExportMIME(alias); canonical != "" {
			t.Errorf("unsupported alias %q normalised to %q", alias, canonical)
		}
	}
}

// TestExport_RefusesEverythingThatIsNotWhatItClaims is the core guard.
func TestExport_RefusesEverythingThatIsNotWhatItClaims(t *testing.T) {
	cases := map[string]struct {
		content, mime, name string
		want                string
	}{
		// Binary declared as text. Each has a real signature, which is exactly
		// why it cannot be a sniff-less text format.
		"pdf as csv":    {"%PDF-1.7\n%\xE2\xE3\xCF\xD3\n", "text/csv", "x.csv", ExportFailureUnsupportedType},
		"png as csv":    {"\x89PNG\r\n\x1a\n\x00\x00", "text/csv", "x.csv", ExportFailureUnsupportedType},
		"jpeg as jsonl": {"\xFF\xD8\xFF\xE0zzz", "application/x-ndjson", "x.jsonl", ExportFailureUnsupportedType},
		// Executables and archives are refused with their real reason.
		"elf as csv":   {"\x7fELF\x02\x01\x01\x00", "text/csv", "x.csv", ExportFailureExecutable},
		"shell as csv": {"#!/bin/sh\necho hi\n", "text/csv", "x.csv", ExportFailureExecutable},
		"zip as jsonl": {"PK\x03\x04\x00\x00", "application/x-ndjson", "x.jsonl", ExportFailureExecutable},
		"mz as csv":    {"MZ\x90\x00\x03", "text/csv", "x.csv", ExportFailureExecutable},

		// Content-safety rules.
		"nul byte":     {"a,b\n1,\x00\n", "text/csv", "x.csv", ExportFailureControlBytes},
		"control byte": {"a,b\n1,\x07\n", "text/csv", "x.csv", ExportFailureControlBytes},
		"invalid utf8": {"a,b\n1,\xff\xfe\n", "text/csv", "x.csv", ExportFailureEncoding},
		"empty csv":    {"", "text/csv", "x.csv", ExportFailureEmpty},
		"empty jsonl":  {"", "application/x-ndjson", "x.jsonl", ExportFailureEmpty},

		// Structure.
		"malformed jsonl":        {"{\"a\":1}\nnot json\n", "application/x-ndjson", "x.jsonl", ExportFailureStructure},
		"jsonl fragment":         {"{\"a\":1\n", "application/x-ndjson", "x.jsonl", ExportFailureStructure},
		"csv ragged columns":     {"a,b\n1,2,3\n", "text/csv", "x.csv", ExportFailureStructure},
		"csv unterminated quote": {"a,b\n\"unclosed,2\n", "text/csv", "x.csv", ExportFailureStructure},
		// A JSON array is not newline-delimited JSON, and a CSV is not JSONL.
		"json array as jsonl":  {"[{\"a\":1},{\"b\":2}]\n", "application/x-ndjson", "x.jsonl", ExportFailureStructure},
		"csv content as jsonl": {"a,b\n1,2\n", "application/x-ndjson", "x.jsonl", ExportFailureStructure},

		// Type and name pairing.
		"csv content wrong extension":   {"a,b\n1,2\n", "text/csv", "x.jsonl", ExportFailureExtension},
		"jsonl content wrong extension": {"{\"a\":1}\n", "application/x-ndjson", "x.csv", ExportFailureExtension},
		"no extension":                  {"a,b\n1,2\n", "text/csv", "export", ExportFailureExtension},
		"no file name":                  {"a,b\n1,2\n", "text/csv", "", ExportFailureExtension},
		"path in name":                  {"a,b\n1,2\n", "text/csv", "../../etc/passwd.csv", ExportFailureExtension},
		"double extension":              {"a,b\n1,2\n", "text/csv", "x.csv.html", ExportFailureExtension},

		// Types this exception must never admit, each declaring itself openly.
		"html":       {"<html><body>hi</body></html>", "text/html", "x.html", ExportFailureUnsupportedType},
		"svg":        {"<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "image/svg+xml", "x.svg", ExportFailureUnsupportedType},
		"javascript": {"alert(1)", "application/javascript", "x.js", ExportFailureUnsupportedType},
		"markdown":   {"# title", "text/markdown", "x.md", ExportFailureUnsupportedType},
		"xml":        {"<a/>", "application/xml", "x.xml", ExportFailureUnsupportedType},
		"plain text": {"just text", "text/plain", "x.txt", ExportFailureUnsupportedType},
		"octet":      {"anything", "application/octet-stream", "x.csv", ExportFailureUnsupportedType},
		"xlsx":       {"anything", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "x.xlsx", ExportFailureUnsupportedType},
		"wildcard":   {"a,b\n1,2\n", "text/*", "x.csv", ExportFailureUnsupportedType},
	}
	for name, testCase := range cases {
		got := validateExport(testCase.content, testCase.mime, testCase.name, 1<<20)
		if got == "" {
			t.Errorf("%s: ACCEPTED; it must be refused", name)
			continue
		}
		if got != testCase.want {
			t.Errorf("%s: refused with %s; want %s", name, got, testCase.want)
		}
	}
}

// TestExport_HTMLDisguisedAsCSVCannotBecomeExecutableContent is the specific
// attack this exception must not open: a page that would run in a browser,
// wearing a name and a type that would get it stored.
func TestExport_HTMLDisguisedAsCSVCannotBecomeExecutableContent(t *testing.T) {
	payloads := map[string]string{
		"script tag":    "<script>alert(document.cookie)</script>",
		"svg onload":    "<svg onload=alert(1)>",
		"html document": "<!DOCTYPE html><html><head><title>x</title></head></html>",
		"iframe":        "<iframe src=\"javascript:alert(1)\"></iframe>",
		"php open tag":  "<?php system($_GET['c']); ?>",
	}
	for name, payload := range payloads {
		// Declared honestly: refused because the type is not an export type.
		if failure := validateExport(payload, "text/html", "x.html", 1<<20); failure != ExportFailureUnsupportedType {
			t.Errorf("%s declared as html: %s; want %s", name, failure, ExportFailureUnsupportedType)
		}
		// Declared dishonestly as CSV with a .csv name: it must still be
		// refused, because it does not parse as CSV with consistent columns.
		if failure := validateExport(payload, "text/csv", "x.csv", 1<<20); failure != ExportFailureUnsupportedType {
			t.Errorf("%s disguised as csv: %s; want %s", name, failure, ExportFailureUnsupportedType)
		}
		// And as JSONL, where the same markup rule fires first.
		if failure := validateExport(payload, "application/x-ndjson", "x.jsonl", 1<<20); failure != ExportFailureUnsupportedType {
			t.Errorf("%s disguised as jsonl: %s; want %s", name, failure, ExportFailureUnsupportedType)
		}
	}
}

// TestExport_SizeCeilingIsEnforcedDuringValidation.
func TestExport_SizeCeilingIsEnforcedDuringValidation(t *testing.T) {
	big := "a,b\n" + strings.Repeat("1,2\n", 5000)
	if failure := validateExport(big, "text/csv", "x.csv", int64(len(big)+16)); failure != "" {
		t.Fatalf("a file inside the ceiling was refused: %s", failure)
	}
	if failure := validateExport(big, "text/csv", "x.csv", 128); failure != ExportFailureTooLarge {
		t.Fatalf("an oversized file was refused with %s; want %s", failure, ExportFailureTooLarge)
	}
	// The same for JSONL, and it must be the size rule that fires rather than a
	// structural one caused by a truncated read.
	lines := strings.Repeat("{\"a\":1}\n", 5000)
	if failure := validateExport(lines, "application/x-ndjson", "x.jsonl", 128); failure != ExportFailureTooLarge {
		t.Fatalf("an oversized jsonl was refused with %s; want %s", failure, ExportFailureTooLarge)
	}
	// A single absurd line is a structural refusal, not an unbounded buffer.
	huge := "{\"a\":\"" + strings.Repeat("x", MaxExportLineBytes+10) + "\"}"
	if failure := validateExport(huge, "application/x-ndjson", "x.jsonl", int64(len(huge)+16)); failure == "" {
		t.Fatal("an unbounded single line was accepted")
	}
}

// TestExport_MultiByteCharactersSplitAcrossReadsAreNotMistakenForCorruption
// guards the streaming validator's one subtle case.
func TestExport_MultiByteCharactersSplitAcrossReadsAreNotMistakenForCorruption(t *testing.T) {
	// Long enough that the reader's internal buffer boundary lands inside a
	// multi-byte sequence at least once.
	var builder strings.Builder
	builder.WriteString("name\n")
	for i := 0; i < 4000; i++ {
		builder.WriteString("Café Ada Ọlá 中文\n")
	}
	if failure := validateExport(builder.String(), "text/csv", "x.csv", 1<<22); failure != "" {
		t.Fatalf("valid multi-byte content was refused with %s", failure)
	}
}

// TestDetect_StillRefusesTextForDocumentPurposes pins the boundary: the
// ordinary signature detector is unchanged, so nothing about the document
// contract got looser when the export path was added.
func TestDetect_StillRefusesTextForDocumentPurposes(t *testing.T) {
	if detection := Detect(Head([]byte("profile_ref,status\ncrmp_1,ACTIVE\n"))); detection.DetectedMIME != "" {
		t.Fatalf("signature detection classified text as %q; document uploads must still require a signature",
			detection.DetectedMIME)
	}
	if detection := Detect(Head([]byte("{\"a\":1}\n"))); detection.DetectedMIME != "" {
		t.Fatalf("signature detection classified jsonl as %q", detection.DetectedMIME)
	}
	for content, want := range map[string]string{
		"%PDF-1.7\n":        "application/pdf",
		"\x89PNG\r\n\x1a\n": "image/png",
		"\xFF\xD8\xFF\xE0":  "image/jpeg",
	} {
		if detection := Detect(Head([]byte(content))); detection.DetectedMIME != want {
			t.Errorf("signature detection broke for %s: got %q", want, detection.DetectedMIME)
		}
	}
	// Executables are still recognised as executables.
	if detection := Detect(Head([]byte("#!/bin/sh\n"))); !detection.Executable {
		t.Fatal("signature detection stopped recognising a shell script")
	}
}
