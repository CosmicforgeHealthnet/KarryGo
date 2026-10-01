package referralmodels

import (
	"bytes"
	"strings"
	"testing"
)

func TestGeneratedCodesValidate(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		code, err := GenerateCode(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateCode(code); err != nil {
			t.Fatalf("generated code %q does not validate", code)
		}
		if strings.ContainsAny(code, "ILOU") {
			t.Fatalf("code %q contains a confusable letter", code)
		}
		seen[code] = true
	}
	if len(seen) < 1990 {
		t.Fatalf("only %d distinct codes in 2000 draws", len(seen))
	}
}

func TestEverySingleTypoIsCaught(t *testing.T) {
	code, err := GenerateCode(bytes.NewReader([]byte{1, 2, 3, 4, 5, 6, 7}))
	if err != nil {
		t.Fatal(err)
	}
	for position := 0; position < CodeLength; position++ {
		for _, replacement := range codeAlphabet {
			if byte(replacement) == code[position] {
				continue
			}
			typo := code[:position] + string(replacement) + code[position+1:]
			if ValidateCode(typo) == nil {
				t.Fatalf("typo %q of %q was accepted", typo, code)
			}
		}
	}
}

func TestNormalizeCodeForgivesHowPeopleTypeIt(t *testing.T) {
	if got := NormalizeCode(" ab-cd o1l i "); got != "ABCD0111" {
		t.Fatalf("NormalizeCode = %q", got)
	}
	if ValidateCode("TOOSHORT"[:5]) == nil || ValidateCode("ABCDEFGU") == nil {
		t.Fatal("malformed codes must be rejected")
	}
}
