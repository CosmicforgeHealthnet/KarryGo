package repositories

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
)

// ---- Shift templates ----

const templateCols = `id,organization_id,branch_id,name,role,timezone,weekdays,start_local_time,end_local_time,required_capacity,break_rules,status,version,created_at,updated_at`

func scanTemplate(row pgx.Row) (models.ShiftTemplate, error) {
	var v models.ShiftTemplate
	var days []int16
	var rules []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.Name, &v.Role, &v.Timezone, &days, &v.StartLocalTime,
		&v.EndLocalTime, &v.RequiredCapacity, &rules, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	for _, d := range days {
		v.Weekdays = append(v.Weekdays, int(d))
	}
	_ = json.Unmarshal(rules, &v.BreakRules)
	return v, e
}

// InsertTemplate stores a shift template.
func (r *Repository) InsertTemplate(ctx context.Context, q database.Querier, v models.ShiftTemplate, rules []byte, createdBy string) (models.ShiftTemplate, error) {
	return scanTemplate(q.QueryRow(ctx, `INSERT INTO fleet_partner_shift_templates(id,organization_id,branch_id,name,role,timezone,weekdays,start_local_time,end_local_time,required_capacity,break_rules,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+templateCols,
		ids.New("sht"), v.OrganizationID, v.BranchID, v.Name, v.Role, v.Timezone, v.Weekdays, v.StartLocalTime, v.EndLocalTime, v.RequiredCapacity, rules, createdBy))
}

// ListTemplates returns one page of templates by (name, id). A keyset page
// continues after the row AfterID names.
func (r *Repository) ListTemplates(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.ShiftTemplate, error) {
	var rows pgx.Rows
	var e error
	if f.Keyset {
		rows, e = q.Query(ctx, `SELECT `+templateCols+` FROM fleet_partner_shift_templates WHERE organization_id=$1 AND($2=''OR branch_id=$2) AND($3=''OR status=$3)
			AND (COALESCE(cardinality($5::text[]),0)=0 OR branch_id=ANY($5::text[]))
			AND ($6='' OR (name,id) > ((SELECT name FROM fleet_partner_shift_templates WHERE organization_id=$1 AND id=$6),$6))
			ORDER BY name,id LIMIT $4`,
			org, f.BranchID, f.Status, keysetLimit(f.Limit), branchScope(f), f.AfterID)
	} else {
		rows, e = q.Query(ctx, `SELECT `+templateCols+` FROM fleet_partner_shift_templates WHERE organization_id=$1 AND($2=''OR branch_id=$2) AND($3=''OR status=$3) ORDER BY name,id LIMIT $4 OFFSET $5`,
			org, f.BranchID, f.Status, limit(f.Limit), pagination.ClampOffset(f.Offset))
	}
	if e != nil {
		return nil, e
	}
	return collect(rows, scanTemplate)
}

// GetTemplate loads one template.
func (r *Repository) GetTemplate(ctx context.Context, q database.Querier, org, id string) (models.ShiftTemplate, error) {
	return scanTemplate(q.QueryRow(ctx, `SELECT `+templateCols+` FROM fleet_partner_shift_templates WHERE organization_id=$1 AND id=$2`, org, id))
}

// UpdateTemplate stores a guarded template change; pgx.ErrNoRows means the
// version did not match.
func (r *Repository) UpdateTemplate(ctx context.Context, q database.Querier, org, id string, expected int64, next models.ShiftTemplate, rules []byte) (models.ShiftTemplate, error) {
	return scanTemplate(q.QueryRow(ctx, `UPDATE fleet_partner_shift_templates SET name=$4,role=$5,weekdays=$6,start_local_time=$7,end_local_time=$8,required_capacity=$9,break_rules=$10,status=$11,version=version+1,updated_at=now()
			WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+templateCols,
		org, id, expected, next.Name, next.Role, next.Weekdays, next.StartLocalTime, next.EndLocalTime, next.RequiredCapacity, rules, next.Status))
}

// ---- Training enrolments ----

const enrollmentCols = `id,organization_id,course_id,course_version,membership_id,status,assigned_by,assigned_at,due_at,completed_at,score::float8,completion_id,version,created_at,updated_at`

func scanEnrollment(row pgx.Row) (models.Enrollment, error) {
	var v models.Enrollment
	e := row.Scan(&v.ID, &v.OrganizationID, &v.CourseID, &v.CourseVersion, &v.MembershipID, &v.Status, &v.AssignedBy,
		&v.AssignedAt, &v.DueAt, &v.CompletedAt, &v.Score, &v.CompletionID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// InsertEnrollment assigns a course version to a member. A unique violation
// means the member already has an open enrolment for the course.
func (r *Repository) InsertEnrollment(ctx context.Context, q database.Querier, org string, course models.Course, membership, assignedBy string, dueAt *time.Time) (models.Enrollment, error) {
	return scanEnrollment(q.QueryRow(ctx, `INSERT INTO fleet_partner_training_enrollments(id,organization_id,course_id,course_version,membership_id,assigned_by,due_at)
			VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+enrollmentCols,
		ids.New("tre"), org, course.ID, course.Version, membership, assignedBy, dueAt))
}

// ListEnrollments returns one page of enrolments, latest assignment first
// (ties by id ascending).
func (r *Repository) ListEnrollments(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Enrollment, error) {
	var rows pgx.Rows
	var e error
	if f.Keyset {
		rows, e = q.Query(ctx, `SELECT `+enrollmentCols+` FROM fleet_partner_training_enrollments
		WHERE organization_id=$1 AND($2=''OR membership_id=$2) AND($3=''OR status=$3)
		  AND ($5::timestamptz IS NULL OR assigned_at < $5::timestamptz OR (assigned_at = $5::timestamptz AND id > $6))
		ORDER BY assigned_at DESC,id LIMIT $4`,
			org, f.MembershipID, f.Status, keysetLimit(f.Limit), f.After, f.AfterID)
	} else {
		rows, e = q.Query(ctx, `SELECT `+enrollmentCols+` FROM fleet_partner_training_enrollments
		WHERE organization_id=$1 AND($2=''OR membership_id=$2) AND($3=''OR status=$3) ORDER BY assigned_at DESC,id LIMIT $4 OFFSET $5`,
			org, f.MembershipID, f.Status, limit(f.Limit), pagination.ClampOffset(f.Offset))
	}
	if e != nil {
		return nil, e
	}
	return collect(rows, scanEnrollment)
}

// GetEnrollment loads one enrolment.
func (r *Repository) GetEnrollment(ctx context.Context, q database.Querier, org, id string) (models.Enrollment, error) {
	return scanEnrollment(q.QueryRow(ctx, `SELECT `+enrollmentCols+` FROM fleet_partner_training_enrollments WHERE organization_id=$1 AND id=$2`, org, id))
}

// CompleteEnrollment closes an open enrolment with its course result;
// pgx.ErrNoRows means it was already closed.
func (r *Repository) CompleteEnrollment(ctx context.Context, q database.Querier, org, id, status string, completedAt time.Time, score float64, completionID string) (models.Enrollment, error) {
	return scanEnrollment(q.QueryRow(ctx, `UPDATE fleet_partner_training_enrollments SET status=$3,completed_at=$4,score=$5,completion_id=$6,version=version+1,updated_at=now()
			WHERE organization_id=$1 AND id=$2 AND status='ASSIGNED' RETURNING `+enrollmentCols,
		org, id, status, completedAt, score, completionID))
}

// CancelEnrollment withdraws an open enrolment; pgx.ErrNoRows means it is
// missing or already closed.
func (r *Repository) CancelEnrollment(ctx context.Context, q database.Querier, org, id string) (models.Enrollment, error) {
	return scanEnrollment(q.QueryRow(ctx, `UPDATE fleet_partner_training_enrollments SET status='CANCELLED',version=version+1,updated_at=now()
			WHERE organization_id=$1 AND id=$2 AND status='ASSIGNED' RETURNING `+enrollmentCols, org, id))
}
