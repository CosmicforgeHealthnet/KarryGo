package cart

import (
	"context"
	"errors"
	"testing"
)

// A disabled public domain refuses new carts server-side, before any state is
// touched, whatever the client sends.
func TestDomainGateRefusesDisabledDomain(t *testing.T) {
	s := NewService(nil, nil, nil, nil).WithDomainGate(func(d Domain) bool { return d == DomainShop })
	if _, err := s.CreateOrGetActiveCart(context.Background(), "11111111-1111-1111-1111-111111111111", DomainFood); !errors.Is(err, ErrDomainDisabled) {
		t.Fatalf("disabled Food: err=%v", err)
	}
	if err := cartHTTPError(ErrDomainDisabled); err == nil || err.Error() == ErrDomainDisabled.Error() {
		t.Fatalf("disabled domain is not mapped to a typed error: %v", err)
	}
}
