// Package models holds the Section 25 search projection domain.
//
// A search document is a read-side projection over another feature's
// aggregate — never an authority. Nothing here imports gin, pgx or an HTTP
// client, and a search document never claims to be more current than its
// SourceVersion says it is.
package models

import (
	"errors"
	"strings"
	"time"
)

// JSONMap is an untyped jsonb column.
type JSONMap map[string]any

// Clone returns a defensive copy so a stored value cannot be edited through
// the map that produced it.
func (m JSONMap) Clone() JSONMap {
	if m == nil {
		return nil
	}
	out := make(JSONMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ErrInvalidDocument is returned when an index request is missing a value
// the projection cannot function without.
var ErrInvalidDocument = errors.New("search: subjectType, subjectReference, title and requiredPermission are required")

// Document is one indexed, tenant-scoped search projection row.
//
// RequiredPermission is not optional and is never inferred: the row is
// simply not searchable by anyone until the indexing caller states which
// permission gates it, which is what makes "permission checks occur before a
// result is returned" a property of the query rather than of code that runs
// after.
type Document struct {
	ID                   string
	OrganizationID       string
	BranchID             string
	SubjectType          string
	SubjectReference     string
	Title                string
	SearchableText       string
	ExactReferenceFields JSONMap
	Status               string
	RequiredPermission   string
	Visibility           string
	SourceVersion        int64
	IndexedAt            time.Time
	Available            bool
}

// Validate checks the fields the schema itself cannot express as a CHECK
// without also depending on the caller having supplied a non-empty
// organization/actor, which belongs to the usecase layer, not here.
func (d Document) Validate() error {
	if strings.TrimSpace(d.SubjectType) == "" || strings.TrimSpace(d.SubjectReference) == "" ||
		strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.RequiredPermission) == "" {
		return ErrInvalidDocument
	}
	return nil
}

const (
	maxSubjectTypeLen = 64
	maxTitleLen       = 500
	maxSearchableLen  = 20000
)

// Bounded reports whether the document's free-text fields stay inside the
// limits that keep an index entry from becoming a way to smuggle an
// unbounded payload through this API.
func (d Document) Bounded() bool {
	return len(d.SubjectType) <= maxSubjectTypeLen &&
		len(d.Title) <= maxTitleLen &&
		len(d.SearchableText) <= maxSearchableLen
}
