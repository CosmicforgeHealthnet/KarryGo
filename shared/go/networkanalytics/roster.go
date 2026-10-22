package networkanalytics

// Supply roster membership, produced by the operational authorities.
//
// A supply snapshot (supply.go) is an aggregate: it says how many providers
// were online in a zone and can never say WHICH providers. Scoped Admin
// analytics needs an individually verifiable roster instead: one fact per
// provider per roster bucket, recorded by the authority from the same
// availability records its matcher dispatches from. A roster member carries
// the provider identifier and the owner's eligibility/occupancy determination;
// it never carries a position, contact, money or free text.

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	sharedevents "cosmicforge/logistics/shared/go/events"
)

// EventSupplyRoster is one provider's roster membership at a bucket.
const EventSupplyRoster = "network.supply.roster.v1"

// SupplyRosterEntityType is the aggregate type every authority uses for roster
// facts, so analytics can never confuse a roster member with an aggregate.
const SupplyRosterEntityType = "supply_roster_provider"

// rosterNamespace derives the deterministic roster event identity.
var rosterNamespace = uuid.MustParse("9d4c1b7e-2f60-4c8a-b1e3-6a5f0d2c7e41")

// SupplyRosterMember is one provider in an authority's supply roster. Nil
// eligibility/occupancy means the authority could not determine it; that is
// UNKNOWN downstream, never false.
type SupplyRosterMember struct {
	Authority     string    `json:"authority"`
	ProductCode   string    `json:"product_code"`
	ProviderID    string    `json:"provider_id"`
	Eligible      *bool     `json:"eligible"`
	Occupied      *bool     `json:"occupied"`
	BucketStart   time.Time `json:"bucket_start"`
	PolicyVersion string    `json:"policy_version"`
}

// Valid reports whether a member is publishable: a canonical provider UUID, a
// known authority, an explicit product and a bucket-aligned instant.
func (m SupplyRosterMember) Valid() bool {
	id, err := uuid.Parse(m.ProviderID)
	return err == nil && id != uuid.Nil && id.String() == m.ProviderID && ValidAuthority(m.Authority) &&
		strings.TrimSpace(m.ProductCode) != "" && m.ProductCode == strings.ToUpper(m.ProductCode) &&
		!m.BucketStart.IsZero() && m.BucketStart.Equal(BucketStart(m.BucketStart)) && strings.TrimSpace(m.PolicyVersion) != ""
}

// Version is the bucket start in seconds: a later bucket supersedes an earlier
// one for the same provider and product, and a replay is a duplicate.
func (m SupplyRosterMember) Version() int64 { return m.BucketStart.UTC().Unix() }

// EntityID is the provider identifier: roster facts are about one provider.
func (m SupplyRosterMember) EntityID() string { return m.ProviderID }

// EventID is deterministic per authority, product, provider and bucket, so two
// replicas recording the same bucket record the same fact.
func (m SupplyRosterMember) EventID() string {
	return uuid.NewSHA1(rosterNamespace, []byte(strings.Join([]string{m.Authority, m.ProductCode, m.ProviderID,
		m.BucketStart.UTC().Format(time.RFC3339)}, "|"))).String()
}

// RosterEnvelope is the canonical envelope for a roster member.
func RosterEnvelope(sourceService string, m SupplyRosterMember) (sharedevents.Envelope, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return sharedevents.Envelope{}, err
	}
	return sharedevents.Envelope{EventID: m.EventID(), EventName: EventSupplyRoster, SchemaVersion: 1,
		SourceService: sourceService, SourceEntityType: SupplyRosterEntityType, SourceEntityID: m.EntityID(),
		SourceVersion: m.Version(), OccurredAt: m.BucketStart.UTC(), Payload: payload}, nil
}
