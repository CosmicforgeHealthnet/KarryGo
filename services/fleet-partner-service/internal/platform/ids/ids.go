// Package ids provides opaque, prefixed, non-sequential public identifiers.
//
// Every locally owned resource is exposed with a domain-prefixed opaque ID
// (for example org_..., brn_...). Database sequences are never exposed.
// External services' IDs are stored as opaque strings and never assumed to
// follow this format.
package ids

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Prefixes for locally owned domains. External IDs are opaque and unprefixed.
const (
	PrefixOrganization      = "org"
	PrefixBranch            = "brn"
	PrefixDepartment        = "dep"
	PrefixEvent             = "evt"
	PrefixJob               = "job"
	PrefixAudit             = "aud"
	PrefixStatusChange      = "osh"
	PrefixFileRef           = "fil"
	PrefixRequest           = "req"
	PrefixCorrelation       = "cor"
	PrefixIdempotency       = "idm"
	PrefixServiceCredential = "svccred"
)

var idPattern = regexp.MustCompile(`^[a-z]{2,8}_[0-9a-f]{32}$`)

// New generates a cryptographically random opaque ID with the given prefix.
func New(prefix string) string {
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// Validate reports whether value is a well-formed opaque ID carrying the
// expected prefix.
func Validate(prefix string, value string) error {
	if !idPattern.MatchString(value) {
		return fmt.Errorf("identifier %q is not a valid opaque id", value)
	}
	if !strings.HasPrefix(value, prefix+"_") {
		return fmt.Errorf("identifier %q does not carry prefix %q", value, prefix)
	}
	return nil
}

// IsWellFormed reports whether value looks like any locally issued opaque ID.
func IsWellFormed(value string) bool {
	return idPattern.MatchString(value)
}
