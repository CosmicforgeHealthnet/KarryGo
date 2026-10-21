// Package customerreview defines the Customer-owned review-intake contract.
// A recorded request is not account restriction, verification or fulfillment.
package customerreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
)

const Contract = "customer.admin-review-request.v1"
const Owner = "customer-service"
const StateContract = "customer.admin-review-state.v1"
const RequestCapability = "customer.review.request"
const ReadCapability = "customer.review.read"
const Recorded = "REVIEW_REQUEST_RECORDED"
const Path = "/internal/customer/v1/admin/account-review-requests"

var ErrInvalid = errors.New("invalid Customer review request")

type Request struct {
	ContractVersion        string    `json:"contract_version"`
	RequestID              string    `json:"request_id"`
	CustomerID             string    `json:"customer_id"`
	ExpectedReviewRevision int64     `json:"expected_review_revision"`
	IdempotencyKey         string    `json:"idempotency_key"`
	ReasonCode             string    `json:"reason_code"`
	RequestedAt            time.Time `json:"requested_at"`
}

func Segment(value string) bool {
	return adminread.Opaque(value) && !strings.ContainsAny(value, "/%\\")
}
func CanonicalID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func (r Request) Validate(now time.Time) error {
	if r.ContractVersion != Contract || !CanonicalID(r.RequestID) || !CanonicalID(r.CustomerID) || r.ExpectedReviewRevision < 1 || r.ExpectedReviewRevision == int64(^uint64(0)>>1) || !Segment(r.IdempotencyKey) || !adminread.Opaque(r.ReasonCode) || r.RequestedAt.IsZero() || r.RequestedAt.Before(now.Add(-5*time.Minute)) || r.RequestedAt.After(now.Add(time.Second)) {
		return ErrInvalid
	}
	return nil
}
func (r Request) Digest(staff, purpose string, scope adminpolicy.Scope) string {
	r.RequestedAt = r.RequestedAt.UTC()
	raw, _ := json.Marshal(struct {
		Request        Request
		Staff, Purpose string
		Scope          adminpolicy.Scope
	}{r, staff, purpose, scope})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type Receipt struct {
	ContractVersion        string            `json:"contract_version"`
	Status                 string            `json:"status"`
	ReviewRequestID        string            `json:"review_request_id"`
	RequestID              string            `json:"request_id"`
	CustomerID             string            `json:"customer_id"`
	IdempotencyKey         string            `json:"idempotency_key"`
	IntentDigest           string            `json:"intent_digest"`
	RecordedReviewRevision int64             `json:"recorded_review_revision"`
	Scope                  adminpolicy.Scope `json:"scope"`
	RecordedAt             time.Time         `json:"recorded_at"`
}

// DecodeRequest accepts only the exact flat contract. JSON case aliases,
// duplicate keys and multiple documents cannot change the signed intent.
func DecodeRequest(raw []byte) (Request, error) {
	var out Request
	if len(raw) == 0 || len(raw) > 16384 {
		return out, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return out, ErrInvalid
	}
	allowed := map[string]bool{"contract_version": true, "request_id": true, "customer_id": true, "expected_review_revision": true, "idempotency_key": true, "reason_code": true, "requested_at": true}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return out, ErrInvalid
		}
		name, ok := token.(string)
		if !ok || !allowed[name] || seen[name] {
			return out, ErrInvalid
		}
		seen[name] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return out, ErrInvalid
		}
	}
	if _, err = d.Token(); err != nil {
		return out, ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return out, ErrInvalid
	}
	if len(seen) != len(allowed) {
		return out, ErrInvalid
	}
	if json.Unmarshal(raw, &out) != nil {
		return out, ErrInvalid
	}
	return out, nil
}
