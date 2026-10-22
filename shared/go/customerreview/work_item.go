package customerreview

import (
	"bytes"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"
)

const WorkItemContract = "customer.admin-review-work-item.v1"
const WorkItemCapability = "customer.review.work_item.transition"
const WorkItemReadCapability = "customer.review.work_item.read"
const WorkItemPath = "/internal/customer/v1/admin/review-work-item-transitions"

type WorkItemCommand struct {
	ContractVersion string    `json:"contract_version"`
	OperationID     string    `json:"operation_id"`
	ReviewRequestID string    `json:"review_request_id"`
	CustomerID      string    `json:"customer_id"`
	ExpectedVersion int64     `json:"expected_version"`
	Action          string    `json:"action"`
	IdempotencyKey  string    `json:"idempotency_key"`
	ReasonCode      string    `json:"reason_code"`
	RequestedAt     time.Time `json:"requested_at"`
}

func (c WorkItemCommand) Validate(now time.Time) error {
	if c.ContractVersion != WorkItemContract || !CanonicalID(c.OperationID) || !CanonicalID(c.ReviewRequestID) || !CanonicalID(c.CustomerID) || c.ExpectedVersion < 1 || c.ExpectedVersion > 9223372036854775806 || !Segment(c.IdempotencyKey) || !Segment(c.ReasonCode) || c.Action != "ACKNOWLEDGE" && c.Action != "CLOSE" || c.RequestedAt.IsZero() || c.RequestedAt.After(now.Add(time.Minute)) || c.RequestedAt.Before(now.Add(-5*time.Minute)) {
		return ErrInvalid
	}
	return nil
}
func (c WorkItemCommand) Digest(staff, purpose string, scope adminpolicy.Scope) string {
	raw, _ := json.Marshal(struct {
		Command        WorkItemCommand
		Staff, Purpose string
		Scope          adminpolicy.Scope
	}{c, staff, purpose, scope})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type WorkItemReceipt struct {
	ContractVersion string            `json:"contract_version"`
	OperationID     string            `json:"operation_id"`
	ReviewRequestID string            `json:"review_request_id"`
	CustomerID      string            `json:"customer_id"`
	IdempotencyKey  string            `json:"idempotency_key"`
	IntentDigest    string            `json:"intent_digest"`
	State           string            `json:"state"`
	RecordedVersion int64             `json:"recorded_version"`
	Outcome         string            `json:"outcome"`
	Scope           adminpolicy.Scope `json:"scope"`
	RecordedAt      time.Time         `json:"recorded_at"`
}

func strictWorkItem(raw []byte, out any, fields []string) error {
	if len(raw) == 0 || len(raw) > 16384 {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return ErrInvalid
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	for _, k := range fields {
		allowed[k] = true
	}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] || !allowed[key] || strings.ToLower(key) != key {
			return ErrInvalid
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return ErrInvalid
		}
		if key == "scope" {
			var scope adminpolicy.Scope
			if strictWorkItem(value, &scope, []string{"product", "market", "zone", "organization", "tenant"}) != nil {
				return ErrInvalid
			}
		}
	}
	if _, err = d.Token(); err != nil {
		return ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF || len(seen) != len(allowed) {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrInvalid
	}
	return nil
}
func DecodeWorkItemCommand(raw []byte) (WorkItemCommand, error) {
	var c WorkItemCommand
	err := strictWorkItem(raw, &c, []string{"contract_version", "operation_id", "review_request_id", "customer_id", "expected_version", "action", "idempotency_key", "reason_code", "requested_at"})
	return c, err
}
func DecodeWorkItemReceipt(raw []byte) (WorkItemReceipt, error) {
	var r WorkItemReceipt
	err := strictWorkItem(raw, &r, []string{"contract_version", "operation_id", "review_request_id", "customer_id", "idempotency_key", "intent_digest", "state", "recorded_version", "outcome", "scope", "recorded_at"})
	digest, e := hex.DecodeString(r.IntentDigest)
	if err != nil || e != nil || len(digest) != 32 || r.IntentDigest != hex.EncodeToString(digest) || r.ContractVersion != WorkItemContract || !CanonicalID(r.OperationID) || !CanonicalID(r.ReviewRequestID) || !CanonicalID(r.CustomerID) || !Segment(r.IdempotencyKey) || r.RecordedVersion < 2 || r.RecordedAt.IsZero() || r.RecordedAt.After(time.Now().Add(time.Minute)) || r.State != "ACKNOWLEDGED" && r.State != "CLOSED" || r.State == "CLOSED" && r.Outcome != "REVIEW_CLOSED_WITHOUT_ACCOUNT_CHANGE" || r.State == "ACKNOWLEDGED" && r.Outcome != "REVIEW_ACKNOWLEDGED" {
		return WorkItemReceipt{}, ErrInvalid
	}
	return r, nil
}
