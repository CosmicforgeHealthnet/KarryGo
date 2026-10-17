package verification

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrFinalReviewNotReady is returned when any final-review condition changed
// between dossier preparation and the atomic reviewer action.
var ErrFinalReviewNotReady = errors.New("the verification case is not ready for final review")

// FinalCaseDecisionRepository updates every eligible step as ONE transaction.
// Reviewers never call multiple step-approval endpoints to finish onboarding.
type FinalCaseDecisionRepository interface {
	ApproveInHouseCase(ctx context.Context, providerID, reviewerID, notes string) ([]Step, error)
}

func (r *PostgresRepository) ApproveInHouseCase(ctx context.Context, providerID, reviewerID, notes string) ([]Step, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	// A single provider-scoped lock coordinates with other final reviewers.
	// Every required check below is performed again inside the transaction.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", providerID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT step, id::text, status, is_optional
		FROM verification_steps WHERE provider_id = $1
		ORDER BY step FOR UPDATE
	`, providerID)
	if err != nil { return nil, err }
	type state struct { id string; status StepStatus; optional bool }
	states := map[Step]state{}
	for rows.Next() {
		var step string
		var entry state
		if err := rows.Scan(&step, &entry.id, &entry.status, &entry.optional); err != nil {
			rows.Close()
			return nil, err
		}
		states[Step(step)] = entry
	}
	if err := rows.Err(); err != nil { rows.Close(); return nil, err }
	rows.Close()

	identity, hasIdentity := states[StepIdentity]
	face, hasFace := states[StepFace]
	vehicle, hasVehicle := states[StepVehicle]
	emergency, hasEmergency := states[StepEmergency]
	guarantor, hasGuarantor := states[StepGuarantor]
	if !hasIdentity || !hasFace || !hasVehicle || !hasEmergency || !hasGuarantor ||
		identity.status != StatusSubmitted ||
		face.status != StatusSubmitted ||
		vehicle.status != StatusApproved ||
		emergency.status != StatusApproved ||
		guarantor.status != StatusApproved {
		return nil, ErrFinalReviewNotReady
	}
	licence, hasLicence := states[StepLicence]
	if hasLicence && licence.status == StatusRejected { return nil, ErrFinalReviewNotReady }
	if hasLicence && !licence.optional && licence.status != StatusSubmitted &&
		licence.status != StatusApproved { return nil, ErrFinalReviewNotReady }

	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT is_active FROM providers WHERE id = $1 FOR UPDATE
	`, providerID).Scan(&active); err != nil { return nil, err }
	if !active { return nil, ErrFinalReviewNotReady }

	// Reject a case missing the separate government-ID and profile photo.
	var idRef string
	if err := tx.QueryRow(ctx, `
		SELECT file_url FROM verification_documents
		WHERE provider_id = $1 AND step_id = $2 AND document_type = 'govt_id'
		ORDER BY uploaded_at DESC, id DESC LIMIT 1
	`, providerID, identity.id).Scan(&idRef); err != nil { return nil, err }
	var profilePhotoRef string
	if err := tx.QueryRow(ctx, `
		SELECT file_url FROM verification_documents
		WHERE provider_id = $1 AND step_id = $2 AND document_type = 'profile_photo'
		ORDER BY uploaded_at DESC, id DESC LIMIT 1
	`, providerID, identity.id).Scan(&profilePhotoRef); err != nil { return nil, err }
	if strings.TrimSpace(idRef) == "" || strings.TrimSpace(profilePhotoRef) == "" {
		return nil, ErrFinalReviewNotReady
	}

	// The most recent selfie must still belong to the reviewed identity
	// document and be pending a human decision. No automatic match result.
	var checkID, faceCheckStepID, selfieRef, checkIDDocRef, providerUsed, checkStatus string
	if err := tx.QueryRow(ctx, `
		SELECT id::text, step_id::text, selfie_url, id_doc_url, provider_used, COALESCE(result,'')
		FROM face_checks WHERE provider_id = $1
		ORDER BY checked_at DESC, id DESC LIMIT 1 FOR UPDATE
	`, providerID).Scan(&checkID, &faceCheckStepID, &selfieRef, &checkIDDocRef, &providerUsed, &checkStatus); err != nil {
		return nil, err
	}
	if selfieRef == "" || faceCheckStepID != face.id || checkIDDocRef != idRef ||
		providerUsed != "karryam_manual_review" || checkStatus != "pending_review" {
		return nil, ErrFinalReviewNotReady
	}

	approveSteps := []Step{StepIdentity, StepFace}
	if hasLicence && licence.status == StatusSubmitted { approveSteps = append(approveSteps, StepLicence) }
	for _, step := range approveSteps {
		status, ok := states[step]
		if !ok || status.status != StatusSubmitted { return nil, ErrFinalReviewNotReady }
		tag, err := tx.Exec(ctx, `
			UPDATE verification_steps SET status = 'approved',
				reviewed_at = now(), reviewer_id = $3,
				confirm_method = 'manual', rejection_reason = NULL,
				updated_at = now()
			WHERE provider_id = $1 AND step = $2 AND status = 'submitted'
		`, providerID, step, reviewerID)
		if err != nil { return nil, err }
		if tag.RowsAffected() != 1 { return nil, ErrFinalReviewNotReady }
		_, err = tx.Exec(ctx, `
			INSERT INTO verification_audit
			(provider_id, step, action, from_status, to_status, performed_by, notes)
			VALUES ($1, $2, 'approved', 'submitted', 'approved', $3, $4)
		`, providerID, step, reviewerID, notes)
		if err != nil { return nil, fmt.Errorf("audit approved step %s: %w", step, err) }
	}

	tag, err := tx.Exec(ctx, `
		UPDATE face_checks SET result = 'manual_approved',
			match_score = NULL, checked_at = now()
		WHERE id = $1 AND result = 'pending_review' AND provider_used = 'karryam_manual_review'
	`, checkID)
	if err != nil { return nil, err }
	if tag.RowsAffected() != 1 { return nil, ErrFinalReviewNotReady }

	var remaining int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM verification_steps
		WHERE provider_id = $1 AND is_optional = false AND status != 'approved'
	`, providerID).Scan(&remaining); err != nil { return nil, err }
	if remaining != 0 { return nil, ErrFinalReviewNotReady }

	// Mark the rider verified in the SAME transaction as both ID and face
	// decisions. A crash cannot leave half the case approved.
	verified, err := tx.Exec(ctx, `
		UPDATE providers SET verification_status = 'verified', updated_at = now()
		WHERE id = $1 AND is_active = true AND verification_status != 'suspended'
	`, providerID)
	if err != nil { return nil, err }
	if verified.RowsAffected() != 1 { return nil, ErrFinalReviewNotReady }
	_, err = tx.Exec(ctx, `
		INSERT INTO verification_audit
		(provider_id, step, action, from_status, to_status, performed_by, notes)
		VALUES ($1, 'all', 'fully_approved', 'pending', 'approved', $2, $3)
	`, providerID, reviewerID, notes)
	if err != nil { return nil, err }

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return approveSteps, nil
}
