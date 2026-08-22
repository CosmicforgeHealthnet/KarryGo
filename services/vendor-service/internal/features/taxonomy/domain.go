package taxonomy

import (
	"encoding/json"
	"time"
)

type NamespaceCode string

const (
	NamespaceFood NamespaceCode = "FOOD"
	NamespaceShop NamespaceCode = "SHOP"
)

type NamespaceStatus string

const (
	NamespaceStatusActive  NamespaceStatus = "ACTIVE"
	NamespaceStatusRetired NamespaceStatus = "RETIRED"
)

type VersionStatus string

const (
	VersionStatusDraft     VersionStatus = "DRAFT"
	VersionStatusPublished VersionStatus = "PUBLISHED"
	VersionStatusRetired   VersionStatus = "RETIRED"
)

type FacetStructureType string

const (
	FacetStructureFlat         FacetStructureType = "FLAT"
	FacetStructureHierarchical FacetStructureType = "HIERARCHICAL"
)

type RecordStatus string

const (
	RecordStatusActive   RecordStatus = "ACTIVE"
	RecordStatusArchived RecordStatus = "ARCHIVED"
)

type Namespace struct {
	ID          string
	Code        NamespaceCode
	DisplayName string
	Status      NamespaceStatus
	Revision    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Version struct {
	ID                  string
	NamespaceID         string
	VersionNumber       int64
	Status              VersionStatus
	SourceVersionID     *string
	Revision            int64
	CreatedByActorRef   string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	PublishedByActorRef *string
	PublishedAt         *time.Time
	RetiredByActorRef   *string
	RetiredAt           *time.Time
}

type Facet struct {
	ID            string
	NamespaceID   string
	VersionID     string
	Code          string
	DisplayName   string
	Description   string
	StructureType FacetStructureType
	Position      int
	Status        RecordStatus
	Metadata      json.RawMessage
	Revision      int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ArchivedAt    *time.Time
}

type Node struct {
	ID            string
	NamespaceID   string
	VersionID     string
	FacetID       string
	CanonicalCode string
	DisplayName   string
	Description   string
	ParentID      *string
	Position      int
	Status        RecordStatus
	Metadata      json.RawMessage
	Revision      int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ArchivedAt    *time.Time
}

type Actor struct {
	ActorRef      string
	CommandRef    string
	CorrelationID string
	CausationID   string
}

type VersionComparison struct {
	NamespaceCode NamespaceCode
	FromVersion   Version
	ToVersion     Version

	AddedFacets    []Facet
	RemovedFacets  []Facet
	ModifiedFacets []FacetChange

	AddedNodes    []Node
	RemovedNodes  []Node
	ModifiedNodes []NodeChange
}

type FacetChange struct {
	Before Facet
	After  Facet
}

type NodeChange struct {
	Before Node
	After  Node
}
