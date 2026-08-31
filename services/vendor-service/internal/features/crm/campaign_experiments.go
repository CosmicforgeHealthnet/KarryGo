package crm

import (
	"crypto/sha256"
	"encoding/binary"
	"regexp"
	"sort"
)

// variantKeyPattern bounds an experiment cohort key to something that reads the
// same in a database, a URL and an analysis spreadsheet.
var variantKeyPattern = regexp.MustCompile(`^[A-Z0-9_]{1,32}$`)

// experimentNamespace separates campaign bucketing from every other hash this
// service computes. Without it, two features hashing the same profile id would
// produce correlated assignments and an experiment would silently stop being
// independent of whatever else bucketed the same customers.
const experimentNamespace = "vendor.crm.campaign.experiment.v1"

// AssignmentBuckets is the resolution of the assignment space, in basis points.
const AssignmentBuckets = TotalAllocationBps

// ExperimentAssignment is one relationship's cohort within one campaign version.
type ExperimentAssignment struct {
	VariantKey string
	IsControl  bool
	Bucket     int
}

// experimentBucket maps a campaign version and a relationship onto one of ten
// thousand stable buckets.
//
// The inputs are a campaign version id and a vendor-local profile id. Neither
// is personal data: the profile id is an opaque CRM reference, and an email
// address or phone number would not only be PII flowing somewhere it does not
// belong, it would also make assignment change the moment a customer updated
// their contact details.
//
// The result is deterministic and carries no runtime randomness, so the same
// campaign version always produces the same cohorts — which is what makes a
// re-prepared audience comparable to the one before it, and what lets an
// analyst reproduce an assignment months later without a stored lookup.
func experimentBucket(campaignVersionID, profileID string) int {
	sum := sha256.Sum256([]byte(campaignVersionID + "\x00" + profileID + "\x00" + experimentNamespace))
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(AssignmentBuckets))
}

// experimentAllocator lays a version's cohorts out over the bucket space once,
// so assigning a whole audience is a lookup rather than a re-sort per row.
type experimentAllocator struct {
	campaignVersionID string
	enabled           bool
	ranges            []variantRange
}

type variantRange struct {
	upperExclusive int
	key            string
	isControl      bool
}

// newExperimentAllocator builds the cumulative basis-point ranges for a version.
//
// Ordering is control first, then variant key ascending — the same canonical
// order the definition hash uses. A different ordering would produce different
// buckets for the same definition, which would make "stable within a campaign
// version" depend on the order a client happened to send its variants in.
func newExperimentAllocator(version CampaignVersion) experimentAllocator {
	allocator := experimentAllocator{campaignVersionID: version.ID, enabled: version.ExperimentEnabled}
	if !version.ExperimentEnabled {
		return allocator
	}
	variants := append([]ExperimentVariant(nil), version.Variants...)
	sort.SliceStable(variants, func(i, j int) bool {
		if variants[i].IsControl != variants[j].IsControl {
			return variants[i].IsControl
		}
		return variants[i].VariantKey < variants[j].VariantKey
	})
	cumulative := 0
	for _, variant := range variants {
		cumulative += variant.AllocationBps
		allocator.ranges = append(allocator.ranges, variantRange{
			upperExclusive: cumulative, key: variant.VariantKey, isControl: variant.IsControl,
		})
	}
	return allocator
}

// Assign returns the cohort for one relationship.
//
// A campaign with no experiment returns an empty assignment rather than a
// synthetic "everyone" variant: a target that was never part of an experiment
// must not later be read as a treatment group of one.
func (a experimentAllocator) Assign(profileID string) ExperimentAssignment {
	if !a.enabled || len(a.ranges) == 0 {
		return ExperimentAssignment{}
	}
	bucket := experimentBucket(a.campaignVersionID, profileID)
	for _, r := range a.ranges {
		if bucket < r.upperExclusive {
			return ExperimentAssignment{VariantKey: r.key, IsControl: r.isControl, Bucket: bucket}
		}
	}
	// Unreachable while allocations total exactly 10,000, which both the
	// validator and a database check enforce. Falling back to the last range
	// rather than panicking keeps a hypothetical future data defect from
	// taking a preparation worker down.
	last := a.ranges[len(a.ranges)-1]
	return ExperimentAssignment{VariantKey: last.key, IsControl: last.isControl, Bucket: bucket}
}

// ProjectedCohorts estimates how a bounded candidate set would split.
//
// It is computed by actually assigning each candidate rather than by
// multiplying the audience size by the allocation, so a preview reports the
// split that would really happen — including the ordinary lumpiness of a small
// audience — instead of an idealised one that the prepared snapshot would then
// contradict.
func (a experimentAllocator) ProjectedCohorts(profileIDs []string) (control int, treatment int, perVariant map[string]int) {
	perVariant = map[string]int{}
	if !a.enabled {
		return 0, len(profileIDs), perVariant
	}
	for _, profileID := range profileIDs {
		assignment := a.Assign(profileID)
		perVariant[assignment.VariantKey]++
		if assignment.IsControl {
			control++
			continue
		}
		treatment++
	}
	return control, treatment, perVariant
}
