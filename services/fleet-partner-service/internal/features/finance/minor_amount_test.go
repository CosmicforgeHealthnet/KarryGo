package finance

import (
	"bytes"
	"encoding/json"
	"testing"
)

func decodeNumbers(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

// Amounts above 2^53 minor units used to be rounded through float64.
func TestMinorAmountIsExact(t *testing.T) {
	got, err := minorAmount(decodeNumbers(t, `{"amountMinor": 9007199254740993}`), "amountMinor")
	if err != nil || got == nil || *got != 9007199254740993 {
		t.Fatalf("got %v %v", got, err)
	}
	got, err = minorAmount(decodeNumbers(t, `{"amountPaidMinor": 1500}`), "amountMinor", "amountPaidMinor")
	if err != nil || got == nil || *got != 1500 {
		t.Fatalf("fallback field: %v %v", got, err)
	}
	if got, err := minorAmount(decodeNumbers(t, `{"status":"PAID"}`), "amountMinor"); err != nil || got != nil {
		t.Fatalf("absent amount: %v %v", got, err)
	}
	for _, bad := range []string{`{"amountMinor": 12.5}`, `{"amountMinor": -1}`, `{"amountMinor": "100"}`} {
		if _, err := minorAmount(decodeNumbers(t, bad), "amountMinor"); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
}
