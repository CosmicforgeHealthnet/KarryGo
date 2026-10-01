package referralmodels

import (
	"crypto/rand"
	"errors"
	"io"
	"strings"
)

// Referral codes are 8 characters from the Crockford base-32 alphabet (no I,
// L, O or U, so nothing is confusable when read aloud or typed): 7 random
// characters plus 1 check character. 32^7 is about 34 billion codes.
const (
	CodeLength   = 8
	codeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// codeWeights are odd, so each is invertible mod 32: any single mistyped
// character always changes the check character.
var codeWeights = [CodeLength - 1]int{1, 3, 5, 7, 9, 11, 13}

// GenerateCode returns a fresh random code. random may be nil (crypto/rand).
func GenerateCode(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	var raw [CodeLength - 1]byte
	if _, err := io.ReadFull(random, raw[:]); err != nil {
		return "", err
	}
	var code strings.Builder
	for _, b := range raw {
		code.WriteByte(codeAlphabet[int(b)%len(codeAlphabet)])
	}
	body := code.String()
	return body + string(checkChar(body)), nil
}

// NormalizeCode turns what a user typed or a link carried into canonical
// form: upper case, separators removed, and the letters people confuse with
// digits mapped to those digits.
func NormalizeCode(raw string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		switch r {
		case ' ', '-', '_', '.':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		out.WriteRune(r)
	}
	return out.String()
}

// ErrMalformedCode means the code cannot be valid, whether or not it exists.
var ErrMalformedCode = errors.New("referral code is malformed")

// ValidateCode checks a normalized code's shape and check character. It says
// nothing about whether the code belongs to anyone.
func ValidateCode(code string) error {
	if len(code) != CodeLength {
		return ErrMalformedCode
	}
	for i := 0; i < CodeLength; i++ {
		if strings.IndexByte(codeAlphabet, code[i]) < 0 {
			return ErrMalformedCode
		}
	}
	if checkChar(code[:CodeLength-1]) != code[CodeLength-1] {
		return ErrMalformedCode
	}
	return nil
}

func checkChar(body string) byte {
	sum := 0
	for i := 0; i < len(body) && i < len(codeWeights); i++ {
		sum += codeWeights[i] * strings.IndexByte(codeAlphabet, body[i])
	}
	return codeAlphabet[sum%len(codeAlphabet)]
}
