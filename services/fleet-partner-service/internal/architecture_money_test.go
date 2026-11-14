package internal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// floatMoney matches a float64 conversion on a line that handles a money
// amount (whole minor units). Money is integer kobo end to end (FLEET-6d);
// a float64 round trip silently loses exactness above 2^53.
var floatMoney = regexp.MustCompile(`float64\(`)
var moneyName = regexp.MustCompile(`(?i)(minor|kobo|share|amount|fee|price)`)

func TestMoneyNeverPassesThroughFloat64(t *testing.T) {
	for _, path := range collectGoFiles(t, filepath.Join(serviceRoot, "internal", "features"), false) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if floatMoney.MatchString(line) && moneyName.MatchString(line) {
				t.Errorf("%s:%d: money passes through float64: %s", path, n+1, trimmed)
			}
		}
	}
}
