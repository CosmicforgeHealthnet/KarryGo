// Package providerreceipt defines immutable decision read-back. A receipt proves
// that a decision was recorded, never that execution is currently permitted.
package providerreceipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

const Version = "fleet.provider-decision.v1"
const OfferVersion = "fleet.provider-offer-decision.v1"

// Intent contains the durable business decision, excluding transport/session
// metadata and the original optimistic precondition. Reordering or repeating
// evidence references does not change the decision. No sensitive text is sent
// back to an execution service: only this canonical digest is disclosed.
type Intent struct {
	OfferID          string   `json:"offerId,omitempty"`
	ReservationID    string   `json:"sourceReservationId,omitempty"`
	RequestVersion   int64    `json:"sourceRequestVersion,omitempty"`
	AssignmentID     string   `json:"assignmentId"`
	ProviderID       string   `json:"providerId"`
	ClientResponseID string   `json:"clientResponseId"`
	Decision         string   `json:"decision"`
	ReasonCode       string   `json:"reasonCode"`
	Note             string   `json:"note"`
	Evidence         []string `json:"evidence"`
}

func (i Intent) Digest() string {
	i.Evidence = append([]string{}, i.Evidence...)
	sort.Strings(i.Evidence)
	unique := i.Evidence[:0]
	for _, value := range i.Evidence {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	i.Evidence = unique
	raw, _ := json.Marshal(i)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type Receipt struct {
	OfferID          string    `json:"offerId,omitempty"`
	ContractVersion  string    `json:"contractVersion"`
	ReceiptID        string    `json:"receiptId"`
	AssignmentID     string    `json:"assignmentId"`
	ProviderID       string    `json:"providerId"`
	ClientResponseID string    `json:"clientResponseId"`
	Decision         string    `json:"decision"`
	IntentDigest     string    `json:"intentDigest"`
	RecordedAt       time.Time `json:"recordedAt"`
}
