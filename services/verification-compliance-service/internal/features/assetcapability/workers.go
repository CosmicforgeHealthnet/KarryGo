package assetcapability

import (
	"context"
	"time"
)

// SweepResult counts what one expiry pass changed.
type SweepResult struct {
	Certifications int `json:"certifications"`
	Inspections    int `json:"inspections"`
	Documents      int `json:"documents"`
	Providers      int `json:"providers"`
}

// defaultSweepLimit bounds one pass so a large backlog is worked through over
// several ticks instead of holding one long transaction open.
const defaultSweepLimit = 200

// RunExpirySweep lapses everything whose validity has run out and publishes an
// explicit event for each certification it retires.
//
// The important half is the publishing. Marking a row EXPIRED here and saying
// nothing would leave the three driver services holding an ELIGIBLE row they
// have no reason to doubt: eligibility would quietly outlive its certification
// in exactly the service that decides who gets dispatched. Section 5.14 calls
// this out, and it is the reason the sweep writes to the outbox in the same
// transaction shape the decision path uses.
//
// The sweep is safe to run from several processes at once: every claim uses FOR
// UPDATE SKIP LOCKED, so two schedulers overlap harmlessly rather than double-
// publishing.
func (s *Service) RunExpirySweep(ctx context.Context) (SweepResult, error) {
	var result SweepResult
	if s == nil || s.repo == nil {
		return result, nil
	}
	// The sweep runs even when the feature flag is off. A disabled flag stops
	// NEW certifications being issued; it must not freeze the clock on ones
	// already issued, because a lapsed certification that never lapses is the
	// fail-open Section 5.16 explicitly forbids.
	now := s.now().UTC()

	expired, err := s.repo.ExpireCertifications(ctx, now, defaultSweepLimit)
	if err != nil {
		return result, err
	}
	for _, certification := range expired {
		if err = s.publishExpiry(ctx, certification); err != nil {
			return result, err
		}
		result.Certifications++
	}

	inspections, err := s.repo.ExpireInspections(ctx, now, defaultSweepLimit)
	if err != nil {
		return result, err
	}
	for _, inspection := range inspections {
		if err = s.publishInspectionChange(ctx, inspection); err != nil {
			return result, err
		}
		result.Inspections++
	}

	documents, err := s.repo.ExpireDocuments(ctx, now, defaultSweepLimit)
	if err != nil {
		return result, err
	}
	result.Documents = int(documents)

	providers, err := s.repo.ExpireProviderVerifications(ctx, now, defaultSweepLimit)
	if err != nil {
		return result, err
	}
	result.Providers = int(providers)

	return result, nil
}

func (s *Service) publishExpiry(ctx context.Context, certification Certification) error {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err = s.repo.PublishEvent(ctx, tx, certification.CertificationID,
		EventServiceTierStatusChanged, certification.SourceVersion, certification.Authority,
		s.expiredCertificationPayload(certification)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) publishInspectionChange(ctx context.Context, inspection Inspection) error {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err = s.repo.PublishEvent(ctx, tx, inspection.InspectionRef, EventInspectionStatusChanged,
		inspection.SourceVersion, "", s.inspectionPayload(inspection)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ExpirySweepInterval is how often a deployment should run the sweep.
//
// Five minutes is a deliberate compromise. Certification windows are measured
// in months, so the sweep does not need to be prompt to be correct; but a lapse
// that goes unpublished is an asset still being dispatched, so it should not be
// hours either.
const ExpirySweepInterval = 5 * time.Minute
