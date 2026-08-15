package shop

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Fresh-goods configuration
// ---------------------------------------------------------------------------
//
// What a customer may ask for when the goods are alive, perishable or cut to
// order: ripeness for plantain, size grade for yam, cut and cleaning for meat,
// gutting and descaling and head-on/head-off for fish, washed or chopped for
// leafy vegetables.
//
// Not every attribute applies to every product. Rice has a quality grade and a
// cleaning preference; it has no ripeness and no head. The product class
// decides, so an inapplicable option cannot be attached at all.

const freshAttributeCols = `id,business_id,product_id,attribute_type,selection_required,position,
	status,created_at,updated_at,archived_at,version`

func scanFreshAttribute(row rowScanner) (FreshAttribute, error) {
	var a FreshAttribute
	err := row.Scan(&a.ID, &a.BusinessID, &a.ProductID, &a.Type, &a.Required, &a.Position,
		&a.Status, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt, &a.Version)
	return a, err
}

const freshOptionCols = `id,business_id,attribute_id,option_value,display_name,is_default,position,
	status,created_at,updated_at,version`

func scanFreshOption(row rowScanner) (FreshAttributeOption, error) {
	var o FreshAttributeOption
	err := row.Scan(&o.ID, &o.BusinessID, &o.AttributeID, &o.Value, &o.DisplayName, &o.IsDefault,
		&o.Position, &o.Status, &o.CreatedAt, &o.UpdatedAt, &o.Version)
	return o, err
}

func loadFreshAttribute(ctx context.Context, q querier, businessID, productID, attributeID string, lock bool) (FreshAttribute, error) {
	sql := `SELECT ` + freshAttributeCols + ` FROM shop_fresh_attributes
		WHERE business_id=$1 AND product_id=$2 AND id=$3`
	if lock {
		sql += ` FOR UPDATE`
	}
	attribute, err := scanFreshAttribute(q.QueryRow(ctx, sql, businessID,
		strings.TrimSpace(productID), strings.TrimSpace(attributeID)))
	if err != nil {
		return FreshAttribute{}, mapDB(err, "Fresh attribute", CodeFreshAttributeNotFound)
	}
	return attribute, nil
}

func hydrateFreshAttribute(ctx context.Context, q querier, attribute *FreshAttribute) error {
	rows, err := q.Query(ctx, `SELECT `+freshOptionCols+` FROM shop_fresh_attribute_options
		WHERE attribute_id=$1 ORDER BY position, id`, attribute.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	attribute.Options = []FreshAttributeOption{}
	for rows.Next() {
		option, scanErr := scanFreshOption(rows)
		if scanErr != nil {
			return scanErr
		}
		attribute.Options = append(attribute.Options, option)
	}
	return rows.Err()
}

func hydrateFreshAttributes(ctx context.Context, q querier, attributes []FreshAttribute) error {
	if len(attributes) == 0 {
		return nil
	}
	byID := make(map[string]*FreshAttribute, len(attributes))
	ids := make([]string, 0, len(attributes))
	for index := range attributes {
		attributes[index].Options = []FreshAttributeOption{}
		byID[attributes[index].ID] = &attributes[index]
		ids = append(ids, attributes[index].ID)
	}
	rows, err := q.Query(ctx, `SELECT `+freshOptionCols+` FROM shop_fresh_attribute_options
		WHERE attribute_id=ANY($1) ORDER BY attribute_id,position,id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		option, scanErr := scanFreshOption(rows)
		if scanErr != nil {
			return scanErr
		}
		if attribute := byID[option.AttributeID]; attribute != nil {
			attribute.Options = append(attribute.Options, option)
		}
	}
	return rows.Err()
}

// FreshOptionInput is one bounded option in the configured order.
type FreshOptionInput struct {
	Value       string
	DisplayName string
	IsDefault   bool
}

// ConfigureFreshAttributeInput creates or replaces one attribute's options.
type ConfigureFreshAttributeInput struct {
	AttributeType   string
	Required        *bool
	Position        *int
	Options         []FreshOptionInput
	ExpectedVersion int64
}

// ConfigureFreshAttribute attaches or reconfigures one fresh attribute.
//
// The whole option set is supplied at once, because a partial update cannot
// express what should happen to the options it does not name.
func (s *Service) ConfigureFreshAttribute(ctx context.Context, actor Actor, scope Scope, productID string, in ConfigureFreshAttributeInput) (FreshAttribute, error) {
	if err := s.requireWrites(); err != nil {
		return FreshAttribute{}, err
	}
	if err := requireBusinessWide(scope, "Fresh attributes"); err != nil {
		return FreshAttribute{}, err
	}
	attributeType := strings.ToUpper(strings.TrimSpace(in.AttributeType))
	allowedOptions, known := FreshAttributeOptions[attributeType]
	if !known {
		return FreshAttribute{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "attribute_type", Message: "Choose a supported fresh attribute."}}), CodeFreshAttributeInvalid)
	}
	if len(in.Options) == 0 {
		return FreshAttribute{}, fieldError("options", "Provide at least one option.")
	}
	if len(in.Options) > MaxFreshOptions {
		return FreshAttribute{}, fieldError("options", "Provide at most "+itoa(MaxFreshOptions)+" options.")
	}
	seen := map[string]bool{}
	defaults := 0
	options := make([]FreshOptionInput, 0, len(in.Options))
	for _, option := range in.Options {
		value := strings.ToUpper(strings.TrimSpace(option.Value))
		if !optionPattern.MatchString(value) {
			return FreshAttribute{}, fieldError("options", "Option values are uppercase codes.")
		}
		if !contains(allowedOptions, value) {
			return FreshAttribute{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "options", Message: value + " is not a valid " + attributeType + " option."}}),
				CodeFreshAttributeInvalid)
		}
		if seen[value] {
			return FreshAttribute{}, fieldError("options", "The same option was supplied more than once.")
		}
		seen[value] = true
		displayName := strings.TrimSpace(option.DisplayName)
		if displayName == "" {
			displayName = humanizeOption(value)
		}
		if len([]rune(displayName)) > 80 {
			return FreshAttribute{}, fieldError("options", "Option names may be at most 80 characters.")
		}
		if option.IsDefault {
			defaults++
		}
		options = append(options, FreshOptionInput{Value: value, DisplayName: displayName, IsDefault: option.IsDefault})
	}
	if defaults > 1 {
		return FreshAttribute{}, fieldError("options", "Only one option may be the default.")
	}

	var configured FreshAttribute
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		product, err := loadProduct(ctx, tx, actor.VendorID, productID, true)
		if err != nil {
			return err
		}
		if product.Status == StatusArchived {
			return errArchived("This product")
		}
		if !AttributeAllowedForClass(product.ProductClass, attributeType) {
			return coded(apperrors.Conflict(
				attributeType+" does not apply to a "+product.ProductClass+" product.", nil),
				CodeFreshAttributeInvalid)
		}

		existing, err := scanFreshAttribute(tx.QueryRow(ctx, `SELECT `+freshAttributeCols+`
			FROM shop_fresh_attributes WHERE business_id=$1 AND product_id=$2 AND attribute_type=$3 FOR UPDATE`,
			actor.VendorID, product.ID, attributeType))
		found := err == nil
		if err != nil && !isNoRows(err) {
			return mapDB(err, "Fresh attribute", CodeFreshAttributeNotFound)
		}

		required := boolOrDefault(in.Required, false)
		position := 0
		if in.Position != nil && *in.Position >= 0 {
			position = *in.Position
		}
		var before any
		if found {
			if err = expectedVersionMatches(existing.Version, in.ExpectedVersion); err != nil {
				return err
			}
			if err = hydrateFreshAttribute(ctx, tx, &existing); err != nil {
				return err
			}
			before = FreshAttributeViewOf(existing)
			if in.Position == nil {
				position = existing.Position
			}
			row, updateErr := scanFreshAttribute(tx.QueryRow(ctx, `
				UPDATE shop_fresh_attributes
				SET selection_required=$3, position=$4, status='ACTIVE', archived_at=NULL,
				    updated_at=now(), version=version+1
				WHERE business_id=$1 AND id=$2 RETURNING `+freshAttributeCols,
				actor.VendorID, existing.ID, required, position))
			if updateErr != nil {
				return mapDB(updateErr, "Fresh attribute", CodeFreshAttributeNotFound)
			}
			configured = row
		} else {
			if in.Position == nil {
				if err = tx.QueryRow(ctx, `SELECT count(*) FROM shop_fresh_attributes WHERE product_id=$1`,
					product.ID).Scan(&position); err != nil {
					return err
				}
			}
			row, insertErr := scanFreshAttribute(tx.QueryRow(ctx, `
				INSERT INTO shop_fresh_attributes
					(id,business_id,product_id,attribute_type,selection_required,position,status)
				VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE') RETURNING `+freshAttributeCols,
				newID("sfa_"), actor.VendorID, product.ID, attributeType, required, position))
			if insertErr != nil {
				return mapDB(insertErr, "Fresh attribute", CodeFreshAttributeNotFound)
			}
			configured = row
		}

		if _, err = tx.Exec(ctx, `DELETE FROM shop_fresh_attribute_options WHERE attribute_id=$1`,
			configured.ID); err != nil {
			return err
		}
		for index, option := range options {
			if _, err = tx.Exec(ctx, `
				INSERT INTO shop_fresh_attribute_options
					(id,business_id,attribute_id,option_value,display_name,is_default,position,status)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'ACTIVE')`,
				newID("sfo_"), actor.VendorID, configured.ID, option.Value, option.DisplayName,
				option.IsDefault, index); err != nil {
				return mapDB(err, "Fresh attribute option", CodeFreshAttributeNotFound)
			}
		}
		if err = hydrateFreshAttribute(ctx, tx, &configured); err != nil {
			return err
		}
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, product.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.fresh_attributes.configured", eventType: "shop.fresh_attributes.changed",
			aggregateType: "shop_fresh_attribute", aggregateID: configured.ID, version: configured.Version,
			before: before, after: FreshAttributeViewOf(configured), payload: FreshAttributeViewOf(configured),
		})
	})
	return configured, err
}

func isNoRows(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result set")
}

// humanizeOption renders HEAD_ON as "Head On" so a vendor need not name every
// option by hand.
func humanizeOption(value string) string {
	parts := strings.Split(strings.ToLower(value), "_")
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

// ListFreshAttributes reads a product's fresh configuration.
func (s *Service) ListFreshAttributes(ctx context.Context, actor Actor, scope Scope, productID string, includeArchived bool) ([]FreshAttribute, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	sql := `SELECT ` + freshAttributeCols + ` FROM shop_fresh_attributes
		WHERE business_id=$1 AND product_id=$2`
	if !includeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	sql += ` ORDER BY position, id LIMIT 50`
	rows, err := s.db.Query(ctx, sql, actor.VendorID, strings.TrimSpace(productID))
	if err != nil {
		return nil, err
	}
	attributes := []FreshAttribute{}
	for rows.Next() {
		attribute, scanErr := scanFreshAttribute(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		attributes = append(attributes, attribute)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = hydrateFreshAttributes(ctx, s.db, attributes); err != nil {
		return nil, err
	}
	return attributes, nil
}

// ArchiveFreshAttribute retires one attribute configuration.
func (s *Service) ArchiveFreshAttribute(ctx context.Context, actor Actor, scope Scope, productID, attributeID string, expectedVersion int64) (FreshAttribute, error) {
	if err := s.requireWrites(); err != nil {
		return FreshAttribute{}, err
	}
	if err := requireBusinessWide(scope, "Fresh attributes"); err != nil {
		return FreshAttribute{}, err
	}
	var archived FreshAttribute
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadFreshAttribute(ctx, tx, actor.VendorID, productID, attributeID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if err = hydrateFreshAttribute(ctx, tx, &current); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		row, err := scanFreshAttribute(tx.QueryRow(ctx, `
			UPDATE shop_fresh_attributes SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+freshAttributeCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Fresh attribute", CodeFreshAttributeNotFound)
		}
		archived = row
		if err = hydrateFreshAttribute(ctx, tx, &archived); err != nil {
			return err
		}
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, current.ProductID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.fresh_attributes.archived", eventType: "shop.fresh_attributes.changed",
			aggregateType: "shop_fresh_attribute", aggregateID: archived.ID, version: archived.Version,
			before: FreshAttributeViewOf(current), after: FreshAttributeViewOf(archived),
			payload: FreshAttributeViewOf(archived),
		})
	})
	return archived, err
}

// ensureAttributesFitClass refuses a product-class change that would strand an
// attribute the new class does not admit — a fish's descaling option must not
// survive a reclassification to DRY_BULK.
func ensureAttributesFitClass(ctx context.Context, q querier, businessID, productID, productClass string) error {
	rows, err := q.Query(ctx, `SELECT attribute_type FROM shop_fresh_attributes
		WHERE business_id=$1 AND product_id=$2 AND status='ACTIVE'`, businessID, productID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var stranded []string
	for rows.Next() {
		var attributeType string
		if err = rows.Scan(&attributeType); err != nil {
			return err
		}
		if !AttributeAllowedForClass(productClass, attributeType) {
			stranded = append(stranded, attributeType)
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(stranded) > 0 {
		return coded(apperrors.Conflict(
			"Archive the fresh attributes that do not apply to a "+productClass+" product first: "+
				strings.Join(stranded, ", ")+".", nil), CodeFreshAttributeInvalid)
	}
	return nil
}
