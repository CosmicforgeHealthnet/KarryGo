package repositories

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
)

// Provider self-view reads (FLEET-17.15). Each projection is built field by
// field; no money and no staff identifier is selected.

// ProviderOwnShifts returns the member's non-draft shift assignments
// overlapping [from, to).
func (r *Repository) ProviderOwnShifts(ctx context.Context, q database.Querier, org, membership string, from, to time.Time) ([]models.OwnShift, error) {
	rows, err := q.Query(ctx, `
			SELECT a.id, s.id, s.name, s.role, s.branch_id, COALESCE(b.name, ''), s.timezone, a.assignment_type,
			       a.status, s.status, a.expected_start, a.expected_end, a.checked_in_at, a.checked_out_at
			FROM fleet_partner_shift_assignments a
			JOIN fleet_partner_shifts s ON s.organization_id = a.organization_id AND s.id = a.shift_id
			LEFT JOIN fleet_partner_branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
			WHERE a.organization_id = $1 AND a.membership_id = $2 AND s.status <> 'DRAFT'
			  AND a.expected_start < $4 AND a.expected_end > $3
			ORDER BY a.expected_start, a.id LIMIT 500`, org, membership, from, to)
	if err != nil {
		return nil, err
	}
	out := []models.OwnShift{}
	for rows.Next() {
		var v models.OwnShift
		if err := rows.Scan(&v.ShiftAssignmentID, &v.ShiftID, &v.ShiftName, &v.Role, &v.BranchID, &v.BranchName,
			&v.Timezone, &v.AssignmentType, &v.Status, &v.ShiftStatus, &v.ExpectedStart, &v.ExpectedEnd,
			&v.CheckedInAt, &v.CheckedOutAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	rows.Close()
	return out, rows.Err()
}

// ProviderOwnLeave returns the member's leave overlapping [from, to).
func (r *Repository) ProviderOwnLeave(ctx context.Context, q database.Querier, org, membership string, from, to time.Time) ([]models.OwnLeave, error) {
	rows, err := q.Query(ctx, `
			SELECT id, leave_type, status, start_at, end_at FROM fleet_partner_leave_records
			WHERE organization_id = $1 AND membership_id = $2 AND start_at < $4 AND end_at > $3
			ORDER BY start_at, id LIMIT 200`, org, membership, from, to)
	if err != nil {
		return nil, err
	}
	out := []models.OwnLeave{}
	for rows.Next() {
		var v models.OwnLeave
		if err := rows.Scan(&v.LeaveID, &v.LeaveType, &v.Status, &v.StartAt, &v.EndAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	rows.Close()
	return out, rows.Err()
}

// ProviderOwnQualifications returns the member's qualification records plus
// every active qualification mandatory for their service category that they
// do not hold. Expired is left for the caller to derive.
func (r *Repository) ProviderOwnQualifications(ctx context.Context, q database.Querier, org, membership string) ([]models.OwnQualification, error) {
	rows, err := q.Query(ctx, `
		WITH member AS (
			SELECT service_category FROM fleet_partner_organization_provider_memberships
			WHERE organization_id = $1 AND id = $2
		)
		SELECT pq.id, q.id, q.code, q.name, pq.status,
		       q.status = 'ACTIVE' AND q.applicable_services ? COALESCE((SELECT service_category FROM member), ''),
		       pq.evidence_media_id <> '', pq.issued_at, pq.expires_at, pq.verified_at
		FROM fleet_partner_provider_qualifications pq
		JOIN fleet_partner_qualifications q ON q.organization_id = pq.organization_id AND q.id = pq.qualification_id
		WHERE pq.organization_id = $1 AND pq.membership_id = $2
		UNION ALL
		SELECT '', q.id, q.code, q.name, 'MISSING', true, false, NULL, NULL, NULL
		FROM fleet_partner_qualifications q
		WHERE q.organization_id = $1 AND q.status = 'ACTIVE'
		  AND q.applicable_services ? COALESCE((SELECT service_category FROM member), '')
		  AND NOT EXISTS (SELECT 1 FROM fleet_partner_provider_qualifications pq
		      WHERE pq.organization_id = $1 AND pq.membership_id = $2 AND pq.qualification_id = q.id)
		ORDER BY 3, 1`, org, membership)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.OwnQualification{}
	for rows.Next() {
		var v models.OwnQualification
		if err := rows.Scan(&v.ProviderQualificationID, &v.QualificationID, &v.Code, &v.Name, &v.Status, &v.Mandatory,
			&v.EvidenceProvided, &v.IssuedAt, &v.ExpiresAt, &v.VerifiedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ProviderOwnEnrollments returns the member's course enrolments.
func (r *Repository) ProviderOwnEnrollments(ctx context.Context, q database.Querier, org, membership string) ([]models.OwnTrainingEnrollment, error) {
	rows, err := q.Query(ctx, `
		SELECT e.id, c.id, c.code, c.title, e.course_version, e.status, e.assigned_at, e.due_at, e.completed_at
		FROM fleet_partner_training_enrollments e
		JOIN fleet_partner_training_courses c ON c.organization_id = e.organization_id AND c.id = e.course_id
		WHERE e.organization_id = $1 AND e.membership_id = $2
		ORDER BY e.assigned_at DESC, e.id LIMIT 200`, org, membership)
	if err != nil {
		return nil, err
	}
	out := []models.OwnTrainingEnrollment{}
	for rows.Next() {
		var v models.OwnTrainingEnrollment
		if err := rows.Scan(&v.EnrollmentID, &v.CourseID, &v.CourseCode, &v.CourseTitle, &v.CourseVersion, &v.Status,
			&v.AssignedAt, &v.DueAt, &v.CompletedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	rows.Close()
	return out, rows.Err()
}

// ProviderOwnCompletions returns the member's course results.
func (r *Repository) ProviderOwnCompletions(ctx context.Context, q database.Querier, org, membership string) ([]models.OwnTrainingCompletion, error) {
	rows, err := q.Query(ctx, `
		SELECT t.id, c.id, c.code, c.title, t.status, t.score::float8, t.certificate_media_id <> '', t.completed_at, t.expires_at
		FROM fleet_partner_training_completions t
		JOIN fleet_partner_training_courses c ON c.organization_id = t.organization_id AND c.id = t.course_id
		WHERE t.organization_id = $1 AND t.membership_id = $2
		ORDER BY t.completed_at DESC, t.id LIMIT 200`, org, membership)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.OwnTrainingCompletion{}
	for rows.Next() {
		var v models.OwnTrainingCompletion
		if err := rows.Scan(&v.CompletionID, &v.CourseID, &v.CourseCode, &v.CourseTitle, &v.Status, &v.Score,
			&v.CertificateOnFile, &v.CompletedAt, &v.CertificateExpires); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
