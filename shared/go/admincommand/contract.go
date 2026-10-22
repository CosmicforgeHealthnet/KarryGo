// Package admincommand is the finite source-owned administrative command
// contract shared by the private Admin Bridge and each enrolled source owner.
//
// It is deliberately not a generic state setter. Every owner publishes a
// closed action dictionary (see specs.go); an action names a domain transition
// that the owner implements against its own state machine, inside its own
// transaction, with its own version fence. The Bridge never chooses a
// resulting state: it only carries a validated intent, binds an independent
// HQ approval where the owner requires one, and reconciles the owner's
// immutable receipt by original identity.
package admincommand

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid administrative command")
var ErrUnavailable = errors.New("administrative command outcome unavailable; use original-identity readback")
var ErrRefused = errors.New("administrative command refused by source owner")
var ErrAbsent = errors.New("administrative command outcome not yet found; not proof of rejection")

const maxParams = 16
const maxBody = 16384

// Approval is an independently decided HQ approval bound to the exact intent
// digest. The approver must differ from the requester; the source owner
// re-checks both facts and records the approval identity exactly once.
type Approval struct {
	ApprovalID   string    `json:"approval_id"`
	ApproverRef  string    `json:"approver_ref"`
	IntentDigest string    `json:"intent_digest"`
	ApprovedAt   time.Time `json:"approved_at"`
}

// Command is the exact signed body. Unknown keys, case aliases and duplicate
// keys are rejected before any field is interpreted.
type Command struct {
	ContractVersion string            `json:"contract_version"`
	OperationID     string            `json:"operation_id"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectID       string            `json:"subject_id"`
	CaseRef         string            `json:"case_ref"`
	Action          string            `json:"action"`
	ExpectedVersion int64             `json:"expected_version"`
	IdempotencyKey  string            `json:"idempotency_key"`
	ReasonCode      string            `json:"reason_code"`
	Params          map[string]string `json:"params"`
	Approval        *Approval         `json:"approval"`
	RequestedAt     time.Time         `json:"requested_at"`
}

// Receipt is the owner's immutable decision record. RecordedVersion is the
// subject's new administrative version; it is always ExpectedVersion+1.
type Receipt struct {
	ContractVersion string            `json:"contract_version"`
	Owner           string            `json:"owner"`
	OperationID     string            `json:"operation_id"`
	DecisionID      string            `json:"decision_id"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectID       string            `json:"subject_id"`
	CaseRef         string            `json:"case_ref"`
	Action          string            `json:"action"`
	IdempotencyKey  string            `json:"idempotency_key"`
	IntentDigest    string            `json:"intent_digest"`
	ApprovalID      string            `json:"approval_id"`
	PriorState      string            `json:"prior_state"`
	ResultingState  string            `json:"resulting_state"`
	RecordedVersion int64             `json:"recorded_version"`
	Outcome         string            `json:"outcome"`
	Scope           adminpolicy.Scope `json:"scope"`
	RecordedAt      time.Time         `json:"recorded_at"`
}

// History is a bounded, newest-first immutable decision list for one subject.
type History struct {
	ContractVersion string            `json:"contract_version"`
	Owner           string            `json:"owner"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectID       string            `json:"subject_id"`
	CurrentVersion  int64             `json:"current_version"`
	CurrentState    string            `json:"current_state"`
	Decisions       []Receipt         `json:"decisions"`
	Truncated       bool              `json:"truncated"`
	Scope           adminpolicy.Scope `json:"scope"`
	AsOf            time.Time         `json:"as_of"`
}

var token = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
var dateValue = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var digestValue = regexp.MustCompile(`^[0-9a-f]{64}$`)

func Segment(value string) bool {
	return adminread.Opaque(value) && !strings.ContainsAny(value, "/%\\")
}
func CanonicalID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func Digest(value string) bool { return digestValue.MatchString(value) }

// ValidateParam applies the owner-declared closed parameter kind. No free
// form value reaches an owner without one of these bounded shapes.
func (p ParamSpec) valid(value string) bool {
	switch p.Kind {
	case ParamEnum:
		for _, allowed := range p.Enum {
			if value == allowed {
				return true
			}
		}
		return false
	case ParamUUID:
		return CanonicalID(value)
	case ParamRef:
		return Segment(value)
	case ParamDate:
		if !dateValue.MatchString(value) {
			return false
		}
		_, err := time.Parse("2006-01-02", value)
		return err == nil
	case ParamUint:
		if value == "" || len(value) > 12 || value[0] == '0' && value != "0" {
			return false
		}
		for _, r := range value {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	case ParamText:
		limit := p.Max
		if limit <= 0 || limit > 512 {
			limit = 512
		}
		if len(value) == 0 || len(value) > limit || strings.TrimSpace(value) != value {
			return false
		}
		for _, r := range value {
			if r < 0x20 || r == 0x7f {
				return false
			}
		}
		return true
	}
	return false
}

// Validate checks the envelope against the fixed owner dictionary. Approval
// presence is enforced for actions that declare RequiresApproval; the digest
// binding is checked by both the Bridge and the source owner.
func (c Command) Validate(spec Spec, now time.Time) (ActionSpec, error) {
	action, ok := spec.Actions[c.Action]
	if c.ContractVersion != spec.Contract || !ok || !CanonicalID(c.OperationID) || !Segment(c.SubjectID) || !Segment(c.IdempotencyKey) || !token.MatchString(c.ReasonCode) || c.ExpectedVersion < 0 || c.ExpectedVersion > 1<<62 || c.RequestedAt.IsZero() || c.RequestedAt.After(now.Add(time.Minute)) || c.RequestedAt.Before(now.Add(-5*time.Minute)) {
		return ActionSpec{}, ErrInvalid
	}
	kindOK := false
	for _, kind := range action.SubjectKinds {
		kindOK = kindOK || kind == c.SubjectKind
	}
	if !kindOK {
		return ActionSpec{}, ErrInvalid
	}
	if action.RequiresCase && !Segment(c.CaseRef) || !action.RequiresCase && c.CaseRef != "" && !Segment(c.CaseRef) {
		return ActionSpec{}, ErrInvalid
	}
	if len(c.Params) > maxParams {
		return ActionSpec{}, ErrInvalid
	}
	for key, value := range c.Params {
		p, ok := action.Params[key]
		if !ok || !p.valid(value) {
			return ActionSpec{}, ErrInvalid
		}
	}
	for key, p := range action.Params {
		if _, present := c.Params[key]; p.Required && !present {
			return ActionSpec{}, ErrInvalid
		}
	}
	if action.RequiresApproval && c.Approval == nil {
		return ActionSpec{}, ErrInvalid
	}
	if c.Approval != nil {
		a := c.Approval
		if !CanonicalID(a.ApprovalID) || !adminread.Opaque(a.ApproverRef) || !Digest(a.IntentDigest) || a.ApprovedAt.IsZero() || a.ApprovedAt.After(now.Add(time.Minute)) || a.ApprovedAt.Before(now.Add(-24*time.Hour)) {
			return ActionSpec{}, ErrInvalid
		}
	}
	return action, nil
}

func sortedParams(values map[string]string) [][2]string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2]string{k, values[k]})
	}
	return out
}
func hash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// IntentDigest binds what is decided and by whom, independent of transport
// identity. An HQ approval is issued for exactly this digest, so an approval
// for one subject, version, action or parameter set cannot authorize another.
func (c Command) IntentDigest(requester, purpose string, scope adminpolicy.Scope) string {
	return hash(struct {
		Contract, SubjectKind, SubjectID, CaseRef, Action, ReasonCode string
		ExpectedVersion                                               int64
		Params                                                        [][2]string
		Requester, Purpose                                            string
		Scope                                                         adminpolicy.Scope
	}{c.ContractVersion, c.SubjectKind, c.SubjectID, c.CaseRef, c.Action, c.ReasonCode, c.ExpectedVersion, sortedParams(c.Params), requester, purpose, scope})
}

// OperationDigest additionally binds the immutable operation identity. A
// reused idempotency key with any different body is an identity conflict.
func (c Command) OperationDigest(requester, purpose string, scope adminpolicy.Scope) string {
	approval := ""
	if c.Approval != nil {
		approval = c.Approval.ApprovalID + ":" + c.Approval.ApproverRef + ":" + c.Approval.IntentDigest
	}
	return hash(struct {
		Intent, OperationID, IdempotencyKey, Approval string
	}{c.IntentDigest(requester, purpose, scope), c.OperationID, c.IdempotencyKey, approval})
}

// strict rejects duplicate keys, non-lowercase keys, unknown keys and trailing
// documents. Nested objects are walked with their own fixed key set.
func strict(raw []byte, out any, fields []string, nested map[string][]string) error {
	if len(raw) == 0 || len(raw) > maxBody {
		return ErrInvalid
	}
	if err := walk(raw, fields, nested); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}
func walk(raw []byte, fields []string, nested map[string][]string) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return ErrInvalid
	}
	allowed := map[string]bool{}
	for _, f := range fields {
		allowed[f] = true
	}
	seen := map[string]bool{}
	for d.More() {
		t, err = d.Token()
		key, ok := t.(string)
		if err != nil || !ok || seen[key] || strings.ToLower(key) != key || (fields != nil && !allowed[key]) {
			return ErrInvalid
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return ErrInvalid
		}
		if sub, ok := nested[key]; ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			if err := walk(value, sub, nil); err != nil {
				return err
			}
		}
	}
	if _, err = d.Token(); err != nil {
		return ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return ErrInvalid
	}
	if fields != nil && len(seen) != len(allowed) {
		return ErrInvalid
	}
	return nil
}

var commandFields = []string{"contract_version", "operation_id", "subject_kind", "subject_id", "case_ref", "action", "expected_version", "idempotency_key", "reason_code", "params", "approval", "requested_at"}
var approvalFields = []string{"approval_id", "approver_ref", "intent_digest", "approved_at"}
var receiptFields = []string{"contract_version", "owner", "operation_id", "decision_id", "subject_kind", "subject_id", "case_ref", "action", "idempotency_key", "intent_digest", "approval_id", "prior_state", "resulting_state", "recorded_version", "outcome", "scope", "recorded_at"}
var scopeFields = []string{"product", "market", "zone", "organization", "tenant"}

// DecodeCommand accepts only the exact flat contract. Params are an object of
// string values; their keys are validated by the owner dictionary.
func DecodeCommand(raw []byte) (Command, error) {
	var c Command
	if err := strict(raw, &c, commandFields, map[string][]string{"approval": approvalFields, "params": nil}); err != nil {
		return Command{}, err
	}
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	return c, nil
}
func DecodeReceipt(raw []byte) (Receipt, error) {
	var r Receipt
	if err := strict(raw, &r, receiptFields, map[string][]string{"scope": scopeFields}); err != nil {
		return Receipt{}, err
	}
	if err := r.check(); err != nil {
		return Receipt{}, err
	}
	return r, nil
}
func (r Receipt) check() error {
	spec, ok := ForOwner(r.Owner)
	if !ok || r.ContractVersion != spec.Contract || !CanonicalID(r.OperationID) || !CanonicalID(r.DecisionID) || !Segment(r.SubjectID) || !Segment(r.IdempotencyKey) || !Digest(r.IntentDigest) || r.RecordedVersion < 1 || r.RecordedAt.IsZero() || r.RecordedAt.After(time.Now().Add(time.Minute)) || !token.MatchString(r.PriorState) || !token.MatchString(r.ResultingState) || !token.MatchString(r.Outcome) || r.ApprovalID != "" && !CanonicalID(r.ApprovalID) || r.CaseRef != "" && !Segment(r.CaseRef) {
		return ErrInvalid
	}
	action, ok := spec.Actions[r.Action]
	if !ok {
		return ErrInvalid
	}
	kindOK := false
	for _, kind := range action.SubjectKinds {
		kindOK = kindOK || kind == r.SubjectKind
	}
	if !kindOK || action.RequiresApproval && r.ApprovalID == "" {
		return ErrInvalid
	}
	return nil
}
func DecodeHistory(raw []byte) (History, error) {
	var h History
	if len(raw) > 262144 {
		return History{}, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&h) != nil {
		return History{}, ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return History{}, ErrInvalid
	}
	spec, ok := ForOwner(h.Owner)
	if !ok || h.ContractVersion != spec.Contract || !Segment(h.SubjectID) || h.CurrentVersion < 0 || len(h.Decisions) > 100 || !token.MatchString(h.CurrentState) || h.AsOf.IsZero() {
		return History{}, ErrInvalid
	}
	for _, d := range h.Decisions {
		if d.Owner != h.Owner || d.SubjectKind != h.SubjectKind || d.SubjectID != h.SubjectID || d.Scope != h.Scope || d.check() != nil {
			return History{}, ErrInvalid
		}
	}
	return h, nil
}
