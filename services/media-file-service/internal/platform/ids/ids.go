// Package ids provides opaque, prefixed, non-sequential public identifiers
// for the media domain.
//
// Database sequences are never exposed, and identities owned by other platform
// services (organizations, fleet documents, providers) are stored as opaque
// strings and are never re-minted here.
package ids

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Prefixes for locally owned media domains.
const (
	PrefixMediaObject   = "mobj"
	PrefixUploadAuth    = "mauz"
	PrefixHistory       = "mhis"
	PrefixAudit         = "maud"
	PrefixEvent         = "mevt"
	PrefixDeletion      = "mdel"
	PrefixScan          = "mscn"
	PrefixReconcileCase = "mrec"
)

var idPattern = regexp.MustCompile(`^[a-z]{2,8}_[0-9a-f]{32}$`)

// New generates a cryptographically random opaque ID with the given prefix.
func New(prefix string) string {
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// Validate reports whether value is a well-formed opaque ID with the expected
// prefix.
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
func IsWellFormed(value string) bool { return idPattern.MatchString(value) }
