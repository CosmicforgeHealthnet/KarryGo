package fulfilment

// Candidate persistence.
//
// Alternatives are written in the same transaction as the plan they belong
// to. That is not a performance choice: a plan whose alternatives failed to
// save would present its selected candidate as though it had been the only
// one, which is worse than storing nothing at all - it looks like a complete
// answer and is not.
//
// The database, not this file, is what makes a hard refusal unrepresentable
// as a price rejection. The CHECK constraints on the table are the authority;
// everything here simply hands them values.

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// insertCandidate writes one alternative.
func insertCandidate(
	ctx context.Context,
	tx pgx.Tx,
	plan Plan,
	candidate PlanCandidate,
) error {
	legs, err := json.Marshal(candidate.Legs)
	if err != nil {
		return err
	}

	if len(candidate.Legs) == 0 {
		legs = []byte("[]")
	}

	capabilities, err := json.Marshal(
		candidateCapabilityMap(candidate.ServiceCapabilities),
	)
	if err != nil {
		return err
	}

	id := candidate.ID

	if id == "" {
		id = "vfc_" + uuid.NewString()
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO commerce_fulfilment_plan_candidates (
			id, plan_id, customer_id, kind, mode, status, reason_code,
			policy_version, mission_count, vendor_group_count, currency,
			delivery_gross_minor, delivery_customer_minor,
			completion_minutes, detour_minutes, multi_stop_required,
			service_capabilities, legs, fingerprint
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
		          $17,$18,$19)
		ON CONFLICT (plan_id, fingerprint) DO NOTHING`,
		id, plan.ID, plan.CustomerID,
		string(candidate.Kind), string(candidate.Mode),
		string(candidate.Status), candidate.ReasonCode,
		candidate.PolicyVersion, candidate.MissionCount,
		candidate.VendorGroupCount, candidate.Currency,
		candidate.DeliveryGrossMinor, candidate.DeliveryCustomerMinor,
		candidate.CompletionMinutes, candidate.DetourMinutes,
		candidate.MultiStopRequired, capabilities, legs,
		candidate.Fingerprint,
	)

	return err
}

// candidateCapabilityMap converts the typed capability map for storage.
//
// The empty map is stored as an empty object rather than as null, so a reader
// can distinguish "no authority supported multi-stop" from "nobody recorded
// the answer". They are different facts and only one of them is reassuring.
func candidateCapabilityMap(source map[Service]bool) map[string]bool {
	out := make(map[string]bool, len(source))

	for service, supported := range source {
		out[string(service)] = supported
	}

	return out
}

// loadCandidates reads a plan's alternatives back.
//
// The customer predicate is not optional. A candidate carries pickup
// locations, vendor groupings and delivery fees, and a plan id alone must
// never be enough to read another customer's.
func loadCandidates(
	ctx context.Context,
	q interface {
		Query(context.Context, string, ...any) (pgx.Rows, error)
	},
	customerID, planID string,
) ([]PlanCandidate, error) {
	rows, err := q.Query(ctx, `
		SELECT id, kind, mode, status, reason_code, policy_version,
		       mission_count, vendor_group_count, currency,
		       delivery_gross_minor, delivery_customer_minor,
		       completion_minutes, detour_minutes, multi_stop_required,
		       service_capabilities, legs, fingerprint
		  FROM commerce_fulfilment_plan_candidates
		 WHERE plan_id = $1 AND customer_id = $2
		 ORDER BY
		       CASE status
		           WHEN 'SELECTED' THEN 0
		           WHEN 'REJECTED' THEN 1
		           ELSE 2
		       END,
		       kind, fingerprint`,
		planID, customerID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var candidates []PlanCandidate

	for rows.Next() {
		var (
			candidate    PlanCandidate
			kind         string
			mode         string
			status       string
			capabilities []byte
			legs         []byte
		)

		if err = rows.Scan(
			&candidate.ID, &kind, &mode, &status, &candidate.ReasonCode,
			&candidate.PolicyVersion, &candidate.MissionCount,
			&candidate.VendorGroupCount, &candidate.Currency,
			&candidate.DeliveryGrossMinor, &candidate.DeliveryCustomerMinor,
			&candidate.CompletionMinutes, &candidate.DetourMinutes,
			&candidate.MultiStopRequired, &capabilities, &legs,
			&candidate.Fingerprint,
		); err != nil {
			return nil, err
		}

		candidate.Kind = CandidateKind(kind)
		candidate.Mode = Mode(mode)
		candidate.Status = CandidateStatus(status)

		if len(legs) > 0 {
			if err = json.Unmarshal(legs, &candidate.Legs); err != nil {
				return nil, err
			}
		}

		if len(capabilities) > 0 {
			raw := map[string]bool{}

			if err = json.Unmarshal(capabilities, &raw); err != nil {
				return nil, err
			}

			candidate.ServiceCapabilities = make(map[Service]bool, len(raw))

			for service, supported := range raw {
				candidate.ServiceCapabilities[Service(service)] = supported
			}
		}

		candidates = append(candidates, candidate)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return candidates, nil
}

// prepareCandidates assigns ids and removes duplicates before persistence.
//
// Deduplication happens HERE rather than only in the database, because the
// database answer to a duplicate is to discard one silently. Two candidates
// with the same fingerprint are the same plan offered twice, and the correct
// response is to offer it once - keeping the one with the more decisive
// status, since SELECTED and INELIGIBLE both say more than REJECTED.
func prepareCandidates(candidates []PlanCandidate) []PlanCandidate {
	byFingerprint := map[string]int{}

	var unique []PlanCandidate

	for _, candidate := range candidates {
		if candidate.Fingerprint == "" {
			continue
		}

		existing, seen := byFingerprint[candidate.Fingerprint]

		if !seen {
			candidate.ID = "vfc_" + uuid.NewString()
			byFingerprint[candidate.Fingerprint] = len(unique)
			unique = append(unique, candidate)

			continue
		}

		if candidateStatusRank(candidate.Status) <
			candidateStatusRank(unique[existing].Status) {
			id := unique[existing].ID
			candidate.ID = id
			unique[existing] = candidate
		}
	}

	sort.SliceStable(unique, func(i, j int) bool {
		if candidateStatusRank(unique[i].Status) !=
			candidateStatusRank(unique[j].Status) {
			return candidateStatusRank(unique[i].Status) <
				candidateStatusRank(unique[j].Status)
		}

		if unique[i].Kind != unique[j].Kind {
			return unique[i].Kind < unique[j].Kind
		}

		return unique[i].Fingerprint < unique[j].Fingerprint
	})

	return unique
}

func candidateStatusRank(status CandidateStatus) int {
	switch status {
	case StatusCandidateSelected:
		return 0
	case StatusCandidateIneligible:
		return 1
	default:
		return 2
	}
}
