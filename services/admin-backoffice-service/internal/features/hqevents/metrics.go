package hqevents

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync/atomic"
)

// Metrics are low-cardinality counters exported only on the private probe
// listener (Section 27). Labels never carry staff, customer or case ids.
type Metrics struct {
	IngressAccepted, IngressQuarantined, IngressBadSignature, IngressReplay atomic.Uint64
}

func (m *Metrics) inc(c *atomic.Uint64) {
	if m != nil {
		c.Add(1)
	}
}

func decodeSecret(encoded string) ([]byte, error) {
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(secret) < 32 {
		return nil, errors.New("invalid event source secret")
	}
	return secret, nil
}
