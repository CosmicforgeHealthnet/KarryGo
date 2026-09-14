package apperrors

import "testing"

func TestRetryabilitySemantics(t *testing.T) {
	retryable := Unavailable("dependency unavailable", nil)
	if !retryable.Retryable {
		t.Fatal("unavailable error must be retryable")
	}
	terminal := Forbidden("membership suspended", nil)
	if terminal.Retryable {
		t.Fatal("business-rule forbidden error must be terminal")
	}
}
