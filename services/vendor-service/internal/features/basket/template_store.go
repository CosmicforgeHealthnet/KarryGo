package basket

// Recurring template persistence.
//
// Separated from store.go because templates answer to a different concern
// from proposals. A proposal is short-lived evidence; a template is a
// customer's standing preferences, edited over months by possibly more than
// one person in a household, which is why every write here is guarded by an
// expected version rather than by last-write-wins.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const templateSelect = `
	SELECT id, customer_id, name, currency, address_ref, requirements,
	       fulfilment_preference, objective, approval_policy, ceiling_minor,
	       cadence, timezone, local_hour, local_minute, weekday,
	       COALESCE(day_of_month, 0), enabled, version, created_at, updated_at
	  FROM commerce_recurring_basket_templates`

func scanTemplate(row pgx.Row) (Template, error) {
	var (
		template     Template
		requirements []byte
		objective    string
		policy       string
		cadence      string
	)

	err := row.Scan(
		&template.ID, &template.CustomerID, &template.Name,
		&template.Currency, &template.AddressRef, &requirements,
		&template.FulfilmentPreference, &objective, &policy,
		&template.CeilingMinor, &cadence, &template.Timezone,
		&template.LocalHour, &template.LocalMinute, &template.Weekday,
		&template.DayOfMonth, &template.Enabled, &template.Version,
		&template.CreatedAt, &template.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrTemplateNotFound
	}
	if err != nil {
		return Template{}, err
	}

	template.Objective = Objective(objective)
	template.ApprovalPolicy = ApprovalPolicy(policy)
	template.Cadence = Cadence(cadence)

	if len(requirements) > 0 {
		if err = json.Unmarshal(
			requirements, &template.Requirements,
		); err != nil {
			return Template{}, err
		}
	}

	return template, nil
}

// CreateTemplate writes a new recurring basket.
func (s *Store) CreateTemplate(
	ctx context.Context,
	template Template,
) (Template, error) {
	if s == nil || s.db == nil {
		return Template{}, ErrDependencyMissing
	}

	requirements, err := json.Marshal(template.Requirements)
	if err != nil {
		return Template{}, err
	}

	template.ID = "rbt_" + uuid.NewString()

	var dayOfMonth *int

	if template.Cadence == CadenceMonthly {
		day := template.DayOfMonth
		dayOfMonth = &day
	}

	err = s.db.QueryRow(ctx, `
		INSERT INTO commerce_recurring_basket_templates (
			id, customer_id, name, currency, address_ref, requirements,
			fulfilment_preference, objective, approval_policy, ceiling_minor,
			cadence, timezone, local_hour, local_minute, weekday,
			day_of_month, enabled
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING version, created_at, updated_at`,
		template.ID, template.CustomerID, template.Name, template.Currency,
		template.AddressRef, requirements,
		strings.ToUpper(strings.TrimSpace(template.FulfilmentPreference)),
		string(template.Objective), string(template.ApprovalPolicy),
		template.CeilingMinor, string(template.Cadence), template.Timezone,
		template.LocalHour, template.LocalMinute, template.Weekday,
		dayOfMonth, template.Enabled,
	).Scan(&template.Version, &template.CreatedAt, &template.UpdatedAt)
	if err != nil {
		return Template{}, err
	}

	return template, nil
}

// GetTemplate reads one template the customer owns.
func (s *Store) GetTemplate(
	ctx context.Context,
	customerID, templateID string,
) (Template, error) {
	if s == nil || s.db == nil {
		return Template{}, ErrDependencyMissing
	}

	// Both predicates. A template carries a customer's shopping habits, their
	// delivery address reference and their spending ceiling; an id alone must
	// never be enough to read one.
	row := s.db.QueryRow(ctx, templateSelect+`
		 WHERE id = $1 AND customer_id = $2`,
		strings.TrimSpace(templateID), strings.TrimSpace(customerID),
	)

	return scanTemplate(row)
}

// ListTemplates reads a customer's recurring baskets.
func (s *Store) ListTemplates(
	ctx context.Context,
	customerID string,
	limit int,
) ([]Template, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.Query(ctx, templateSelect+`
		 WHERE customer_id = $1
		 ORDER BY created_at DESC, id
		 LIMIT $2`,
		strings.TrimSpace(customerID), limit,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []Template

	for rows.Next() {
		template, scanErr := scanTemplate(rows)

		if scanErr != nil {
			return nil, scanErr
		}

		out = append(out, template)
	}

	return out, rows.Err()
}

// UpdateTemplate applies an edit, guarded by the expected version.
//
// Two people editing one household's recurring basket must not silently
// overwrite each other. The expected-version predicate turns the second write
// into a refusal the caller can act on rather than into a preference somebody
// set and then lost.
func (s *Store) UpdateTemplate(
	ctx context.Context,
	template Template,
	expectedVersion int64,
) (Template, error) {
	if s == nil || s.db == nil {
		return Template{}, ErrDependencyMissing
	}

	requirements, err := json.Marshal(template.Requirements)
	if err != nil {
		return Template{}, err
	}

	var dayOfMonth *int

	if template.Cadence == CadenceMonthly {
		day := template.DayOfMonth
		dayOfMonth = &day
	}

	row := s.db.QueryRow(ctx, `
		UPDATE commerce_recurring_basket_templates
		   SET name = $3, currency = $4, address_ref = $5, requirements = $6,
		       fulfilment_preference = $7, objective = $8,
		       approval_policy = $9, ceiling_minor = $10, cadence = $11,
		       timezone = $12, local_hour = $13, local_minute = $14,
		       weekday = $15, day_of_month = $16, enabled = $17,
		       version = version + 1, updated_at = now()
		 WHERE id = $1 AND customer_id = $2 AND version = $18
		RETURNING id, customer_id, name, currency, address_ref, requirements,
		          fulfilment_preference, objective, approval_policy,
		          ceiling_minor, cadence, timezone, local_hour, local_minute,
		          weekday, COALESCE(day_of_month, 0), enabled, version,
		          created_at, updated_at`,
		template.ID, template.CustomerID, template.Name, template.Currency,
		template.AddressRef, requirements,
		strings.ToUpper(strings.TrimSpace(template.FulfilmentPreference)),
		string(template.Objective), string(template.ApprovalPolicy),
		template.CeilingMinor, string(template.Cadence), template.Timezone,
		template.LocalHour, template.LocalMinute, template.Weekday,
		dayOfMonth, template.Enabled, expectedVersion,
	)

	updated, err := scanTemplate(row)

	if errors.Is(err, ErrTemplateNotFound) {
		// Either the template does not exist for this customer, or the
		// version moved. Both are reported as a conflict rather than as a
		// missing template: telling a customer their own recurring basket
		// does not exist because somebody edited it a second ago would be a
		// worse answer than telling them to reload.
		return Template{}, ErrTemplateConflict
	}

	return updated, err
}

// SetTemplateEnabled enables or disables a template.
//
// A disabled template generates nothing. It is a separate operation from
// deletion on purpose: a customer pausing their weekly shop for a holiday
// should not lose the list they spent time building.
func (s *Store) SetTemplateEnabled(
	ctx context.Context,
	customerID, templateID string,
	enabled bool,
	expectedVersion int64,
) (Template, error) {
	if s == nil || s.db == nil {
		return Template{}, ErrDependencyMissing
	}

	row := s.db.QueryRow(ctx, `
		UPDATE commerce_recurring_basket_templates
		   SET enabled = $3, version = version + 1, updated_at = now()
		 WHERE id = $1 AND customer_id = $2 AND version = $4
		RETURNING id, customer_id, name, currency, address_ref, requirements,
		          fulfilment_preference, objective, approval_policy,
		          ceiling_minor, cadence, timezone, local_hour, local_minute,
		          weekday, COALESCE(day_of_month, 0), enabled, version,
		          created_at, updated_at`,
		strings.TrimSpace(templateID), strings.TrimSpace(customerID),
		enabled, expectedVersion,
	)

	template, err := scanTemplate(row)

	if errors.Is(err, ErrTemplateNotFound) {
		return Template{}, ErrTemplateConflict
	}

	return template, err
}

// ListDueTemplates reads enabled templates for the recurrence worker.
//
// It deliberately does NOT filter by a stored "next occurrence" column. The
// occurrence is computed from the schedule every time, so there is no
// derived timestamp to drift out of step with the customer's actual settings
// after a timezone change, a cadence edit or a daylight-saving boundary.
func (s *Store) ListDueTemplates(
	ctx context.Context,
	limit int,
) ([]Template, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := s.db.Query(ctx, templateSelect+`
		 WHERE enabled
		 ORDER BY updated_at, id
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []Template

	for rows.Next() {
		template, scanErr := scanTemplate(rows)

		if scanErr != nil {
			return nil, scanErr
		}

		out = append(out, template)
	}

	return out, rows.Err()
}
