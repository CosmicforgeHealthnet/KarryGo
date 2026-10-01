package engine

import "fmt"

// RegulatoryChargesV1 is schema version 1 of route and statutory charges: tolls,
// airport access fees, levies and tax.
//
// V1 is deliberately EMPTY. The legacy services charge none of these, so adding
// any would break price-neutrality at cutover, and a tax rate must come from
// current law with an effective date rather than from a developer's memory. The
// kind exists now so that adding a charge later is a governed policy change with
// an approver, not a code change.
type RegulatoryChargesV1 struct {
	SchemaVersion int                `json:"schema_version"`
	Charges       []RegulatoryCharge `json:"charges"`
}

// RegulatoryCharge is one official charge.
type RegulatoryCharge struct {
	Code string `json:"code"`
	// AmountKobo is a flat charge; BPS is a percentage. Exactly one is used.
	AmountKobo int64  `json:"amount_kobo,omitempty"`
	BPS        int64  `json:"bps,omitempty"`
	AppliesTo  string `json:"applies_to"`
	// SourceReference is the official document or notice the charge comes from.
	// A competitor's help page can reveal that a fee exists; it can never be the
	// legal source of one.
	SourceReference string `json:"source_reference"`
}

// Kind returns the policy kind this body belongs to.
func (RegulatoryChargesV1) Kind() string { return KindRegulatoryCharges }

// Validate reports every problem with the body at once.
func (r RegulatoryChargesV1) Validate() error {
	var problems []string
	if r.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	for i, charge := range r.Charges {
		if charge.Code == "" {
			problems = append(problems, fmt.Sprintf("charges[%d].code is required", i))
		}
		if charge.AmountKobo == 0 && charge.BPS == 0 {
			problems = append(problems, fmt.Sprintf("charges[%d] has neither an amount nor a rate", i))
		}
		if charge.AmountKobo != 0 && charge.BPS != 0 {
			problems = append(problems, fmt.Sprintf("charges[%d] has both an amount and a rate; use one", i))
		}
		if charge.AmountKobo < 0 || charge.BPS < 0 {
			problems = append(problems, fmt.Sprintf("charges[%d] cannot be negative", i))
		}
		if charge.SourceReference == "" {
			problems = append(problems, fmt.Sprintf("charges[%d].source_reference is required: a charge needs an official source", i))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("regulatory.charges v1: %s", join(problems))
	}
	return nil
}
