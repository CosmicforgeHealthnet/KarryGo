// Package rollout owns server-side cohort selection and the immutable policy
// snapshot committed when Dispatch admits protected work.
package rollout

import "strings"

const PolicyVersion = "dispatch_rollout_v1"

const (
	ModeLegacy    = "LEGACY"
	ModeProtected = "PROTECTED"
)

type Context struct {
	CustomerID  string
	SourceType  string
	SourceRef   string
	ServiceTier string
}

type Selector struct {
	customerIDs  map[string]struct{}
	sourceTypes  map[string]struct{}
	sourceRefs   map[string]struct{}
	serviceTiers map[string]struct{}
}

func NewSelector(customerIDs, sourceTypes, sourceRefs, serviceTiers string) Selector {
	return Selector{
		customerIDs:  values(customerIDs),
		sourceTypes:  values(sourceTypes),
		sourceRefs:   values(sourceRefs),
		serviceTiers: values(serviceTiers),
	}
}

func (s Selector) Configured() bool {
	return len(s.customerIDs)+len(s.sourceTypes)+len(s.sourceRefs) > 0
}

func (s Selector) Selected(in Context) bool {
	if !s.Configured() {
		return false
	}
	identitySelected := contains(s.customerIDs, in.CustomerID) ||
		contains(s.sourceTypes, in.SourceType) || contains(s.sourceRefs, in.SourceRef)
	if !identitySelected {
		return false
	}
	return len(s.serviceTiers) == 0 || contains(s.serviceTiers, in.ServiceTier)
}

type Protection struct {
	Mode                     string
	PolicyVersion            string
	HandoverRequired         bool
	PickupEvidenceRequired   bool
	DeliveryEvidenceRequired bool
	ProviderTransferRequired bool
}

func Legacy() Protection {
	return Protection{Mode: ModeLegacy, PolicyVersion: PolicyVersion}
}

type Policy struct {
	Selector                  Selector
	HandoverAvailable         bool
	PickupEvidenceAvailable   bool
	DeliveryEvidenceAvailable bool
	ProviderTransferAvailable bool
	Version                   string
}

func (p Policy) Protect(in Context) Protection {
	version := strings.TrimSpace(p.Version)
	if version == "" {
		version = PolicyVersion
	}
	if !p.HandoverAvailable || !p.Selector.Selected(in) {
		result := Legacy()
		result.PolicyVersion = version
		return result
	}
	return Protection{
		Mode:                     ModeProtected,
		PolicyVersion:            version,
		HandoverRequired:         true,
		PickupEvidenceRequired:   p.PickupEvidenceAvailable,
		DeliveryEvidenceRequired: p.DeliveryEvidenceAvailable,
		ProviderTransferRequired: p.ProviderTransferAvailable,
	}
}

func values(raw string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func contains(values map[string]struct{}, value string) bool {
	_, ok := values[strings.ToUpper(strings.TrimSpace(value))]
	return ok
}
