// Package adminowner supplies the existing Admin boundary's owner-side read
// adapter primitives. It is embedded in each source service, never a proxy or a
// new administration service. All queries run against that owner's database.
package adminowner

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/jackc/pgx/v5"
)

const ContractVersion = "karryam.owner-resources.v1"

type Dimension struct {
	Name  string  `json:"name"`
	Value *string `json:"value"`
	Flag  *bool   `json:"flag"`
}
type Counter struct {
	Name  string `json:"name"`
	Value *int64 `json:"value"`
}
type TimelineEvent struct {
	Reference string     `json:"reference"`
	Type      string     `json:"type"`
	At        *time.Time `json:"at"`
}
type Reference struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}
type Resource struct {
	Reference         string            `json:"reference"`
	Kind              string            `json:"kind"`
	Status            *string           `json:"status"`
	SourceVersion     *int64            `json:"source_version"`
	SnapshotETag      string            `json:"snapshot_etag"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Dimensions        []Dimension       `json:"dimensions"`
	Counters          []Counter         `json:"counters"`
	References        []Reference       `json:"references"`
	Timeline          []TimelineEvent   `json:"timeline"`
	TimelineTruncated bool              `json:"timeline_truncated"`
	Custody           *CustodyInventory `json:"custody,omitempty"`
}
type Snapshot struct {
	ContractVersion string            `json:"contract_version"`
	Owner           string            `json:"owner"`
	Kind            string            `json:"kind"`
	Scope           adminpolicy.Scope `json:"scope"`
	PolicyID        string            `json:"policy_id"`
	PolicyVersion   int64             `json:"policy_version"`
	PolicyChecksum  string            `json:"policy_checksum"`
	AsOf            time.Time         `json:"as_of"`
	Coverage        string            `json:"coverage"`
	Items           []Resource        `json:"items"`
	NextCursor      *string           `json:"next_cursor"`
}

// Definition is a compile-time route and disclosure allowlist. No request may
// choose an owner, SQL table, destination or a new output field.
type Definition struct {
	Kind, Capability, SourceContract string
	Dimensions, Counters, References []string
	Custody                          bool
}
type Source interface {
	admincontext.BindingReader
	// The final flag requests bounded detail enrichment. Read models never
	// lock or mutate business state; native command usecases own those guards.
	Read(context.Context, pgx.Tx, string, string, bool) (Resource, error)
}
