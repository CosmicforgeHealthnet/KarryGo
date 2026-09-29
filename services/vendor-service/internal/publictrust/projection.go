// Package publictrust renders bounded facts from Vendor's existing authority
// projection. It makes no verification decisions and never reads case evidence.
package publictrust

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxAge bounds a badge even if callbacks and reconciliation both stop.
const MaxAge = 5 * time.Minute

type Fact struct {
	Dimension   string `json:"dimension"`
	SubjectType string `json:"subject_type"`
	SubjectRef  string `json:"subject_ref"`
	Status      string `json:"status"`
}

type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}
type Reader func(context.Context, string, string) ([]Fact, error)

// Read joins the requested business/branch before selecting dimensions. A
// branch's brand is not a Shop product's manufacturer brand. Empty facts mean
// no current public assertion, not a new verification status.
func Read(ctx context.Context, db Queryer, business, branch string) ([]Fact, error) {
	rows, err := db.Query(ctx, `SELECT p.dimension,p.subject_type,p.subject_ref,
	 CASE WHEN p.status='VERIFIED' AND p.expires_at<=now() THEN 'EXPIRED' ELSE p.status END
	 FROM vendor_branches br JOIN vendor_businesses b ON b.id=br.business_id
	 JOIN vendor_verification_projections p ON p.business_id=b.id
	 WHERE br.id=$2 AND b.id=$1 AND br.status='ACTIVE' AND b.status='ACTIVE'
	 AND p.source_service='verification-compliance-service'
	 AND p.updated_at>now()-($3 * interval '1 second')
	 AND ((p.dimension='BUSINESS' AND p.subject_type='BUSINESS' AND p.subject_ref=b.id)
	 OR (p.dimension='LOCATION' AND p.subject_type='BRANCH' AND p.subject_ref=br.id)
	 OR (p.dimension='BRAND' AND p.subject_type='BRAND' AND p.subject_ref=br.brand_id)
	 OR (p.dimension='FOOD_KITCHEN' AND p.subject_type='BRANCH' AND p.subject_ref=br.id)
	 OR (p.dimension='FOOD_KITCHEN' AND p.subject_type='BRANCH_CAPABILITY' AND EXISTS
	 (SELECT 1 FROM vendor_branch_capabilities bc WHERE bc.id=p.subject_ref AND bc.business_id=b.id AND bc.branch_id=br.id AND bc.status='ACTIVE')))
	 ORDER BY p.dimension,p.subject_type,p.subject_ref`, business, branch, MaxAge.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := []Fact{}
	for rows.Next() {
		var f Fact
		if err := rows.Scan(&f.Dimension, &f.SubjectType, &f.SubjectRef, &f.Status); err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}
