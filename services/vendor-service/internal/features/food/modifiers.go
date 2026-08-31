package food

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Modifier groups
// ---------------------------------------------------------------------------
//
// Groups are business-level reusable definitions: "Add extras" is written once
// and attached to as many items as the business likes, rather than copied into
// every item. That choice is what makes editing a shared group a business-wide
// act, which is in turn why editing one needs a business-wide grant.

type CreateModifierGroupInput struct {
	Name        string
	Description string
	MinSelect   int
	MaxSelect   int
}

// CreateModifierGroup records a reusable choice group for the business.
func (s *Service) CreateModifierGroup(ctx context.Context, actor Actor, scope Scope, in CreateModifierGroupInput) (ModifierGroup, error) {
	if err := s.requireWrites(); err != nil {
		return ModifierGroup{}, err
	}
	if err := requireBusinessWide(scope); err != nil {
		return ModifierGroup{}, err
	}
	var violations []apperrors.FieldViolation
	name, violation := validateName("name", in.Name, 2, 160)
	if violation != nil {
		violations = append(violations, *violation)
	}
	description, violation := validateDescription("description", in.Description, 2000)
	if violation != nil {
		violations = append(violations, *violation)
	}
	if violation = validateSelection(in.MinSelect, in.MaxSelect); violation != nil {
		violations = append(violations, *violation)
	}
	if len(violations) > 0 {
		return ModifierGroup{}, validation(violations...)
	}

	var group ModifierGroup
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row, err := scanGroup(tx.QueryRow(ctx, `
			INSERT INTO food_modifier_groups (id,business_id,name,description,min_select,max_select,status)
			VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE') RETURNING `+groupCols,
			newID("fmg_"), actor.VendorID, name, description, in.MinSelect, in.MaxSelect))
		if err != nil {
			return mapDB(err, "Modifier group", CodeModifierGroupNotFound)
		}
		group = row
		return s.record(ctx, tx, actor, evidence{
			action: "food.modifier_group.created", eventType: "food.modifier_group.created",
			aggregateType: "food_modifier_group", aggregateID: group.ID,
			version: group.Version, after: ModifierGroupViewOf(group), payload: ModifierGroupViewOf(group),
		})
	})
	return group, err
}

// ListModifierGroups returns the business's groups. Reading is not restricted
// to business-wide grants: a branch-scoped manager must be able to see which
// groups exist in order to attach one to their own item.
func (s *Service) ListModifierGroups(ctx context.Context, actor Actor, includeArchived bool, limit, offset int) ([]ModifierGroup, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+groupCols+` FROM food_modifier_groups
		WHERE business_id=$1 AND ($2 OR status <> 'ARCHIVED')
		ORDER BY (status = 'ARCHIVED'), name, id
		LIMIT $3 OFFSET $4`, actor.VendorID, includeArchived, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	groups := []ModifierGroup{}
	for rows.Next() {
		group, scanErr := scanGroup(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		groups = append(groups, group)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(groups) > limit
	if hasMore {
		groups = groups[:limit]
	}
	return groups, hasMore, nil
}

// GetModifierGroup reads one group within the tenant.
func (s *Service) GetModifierGroup(ctx context.Context, actor Actor, groupID string) (ModifierGroup, error) {
	return loadGroup(ctx, s.db, actor.VendorID, groupID, false)
}

type UpdateModifierGroupInput struct {
	Name            *string
	Description     *string
	MinSelect       *int
	MaxSelect       *int
	ExpectedVersion int64
}

// UpdateModifierGroup edits a shared group under optimistic concurrency.
func (s *Service) UpdateModifierGroup(ctx context.Context, actor Actor, scope Scope, groupID string, in UpdateModifierGroupInput) (ModifierGroup, error) {
	if err := s.requireWrites(); err != nil {
		return ModifierGroup{}, err
	}
	if err := requireBusinessWide(scope); err != nil {
		return ModifierGroup{}, err
	}
	var updated ModifierGroup
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		group, err := loadGroup(ctx, tx, actor.VendorID, groupID, true)
		if err != nil {
			return err
		}
		if group.Status == ChildArchived {
			return errArchived("Modifier group")
		}
		if group.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		name, description := group.Name, group.Description
		minSelect, maxSelect := group.MinSelect, group.MaxSelect
		var violations []apperrors.FieldViolation
		if in.Name != nil {
			value, violation := validateName("name", *in.Name, 2, 160)
			if violation != nil {
				violations = append(violations, *violation)
			}
			name = value
		}
		if in.Description != nil {
			value, violation := validateDescription("description", *in.Description, 2000)
			if violation != nil {
				violations = append(violations, *violation)
			}
			description = value
		}
		if in.MinSelect != nil {
			minSelect = *in.MinSelect
		}
		if in.MaxSelect != nil {
			maxSelect = *in.MaxSelect
		}
		if violation := validateSelection(minSelect, maxSelect); violation != nil {
			violations = append(violations, *violation)
		}
		if len(violations) > 0 {
			return validation(violations...)
		}

		updated, err = scanGroup(tx.QueryRow(ctx, `
			UPDATE food_modifier_groups SET name=$3, description=$4, min_select=$5, max_select=$6,
				updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+groupCols,
			actor.VendorID, group.ID, name, description, minSelect, maxSelect))
		if err != nil {
			return mapDB(err, "Modifier group", CodeModifierGroupNotFound)
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.modifier_group.updated", eventType: "food.modifier_group.updated",
			aggregateType: "food_modifier_group", aggregateID: updated.ID,
			version: updated.Version, before: ModifierGroupViewOf(group), after: ModifierGroupViewOf(updated),
			payload: ModifierGroupViewOf(updated),
		})
	})
	return updated, err
}

// ArchiveModifierGroup retires a group and its modifiers, and detaches it from
// every item.
//
// The attachments are relationship rows with no historical authority of their
// own, so removing them is the correct cleanup; the group itself is archived
// rather than deleted because anything that already referenced it must stay
// resolvable.
func (s *Service) ArchiveModifierGroup(ctx context.Context, actor Actor, scope Scope, groupID string, expectedVersion int64) (ModifierGroup, error) {
	if err := s.requireWrites(); err != nil {
		return ModifierGroup{}, err
	}
	if err := requireBusinessWide(scope); err != nil {
		return ModifierGroup{}, err
	}
	var archived ModifierGroup
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		group, err := loadGroup(ctx, tx, actor.VendorID, groupID, true)
		if err != nil {
			return err
		}
		if group.Status == ChildArchived {
			return errArchived("Modifier group")
		}
		if group.Version != expectedVersion {
			return errStaleVersion()
		}
		archived, err = scanGroup(tx.QueryRow(ctx, `
			UPDATE food_modifier_groups SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+groupCols, actor.VendorID, group.ID))
		if err != nil {
			return mapDB(err, "Modifier group", CodeModifierGroupNotFound)
		}
		if _, err = tx.Exec(ctx, `
			UPDATE food_modifiers SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND modifier_group_id=$2 AND status <> 'ARCHIVED'`, actor.VendorID, group.ID); err != nil {
			return err
		}
		if err = resequence(ctx, tx, modifiersOrdering, group.ID); err != nil {
			return err
		}

		rows, err := tx.Query(ctx, `
			DELETE FROM food_item_modifier_groups WHERE business_id=$1 AND modifier_group_id=$2
			RETURNING item_id`, actor.VendorID, group.ID)
		if err != nil {
			return err
		}
		var detached []string
		for rows.Next() {
			var itemID string
			if err = rows.Scan(&itemID); err != nil {
				rows.Close()
				return err
			}
			detached = append(detached, itemID)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, itemID := range detached {
			if err = resequence(ctx, tx, attachedOrdering, itemID); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.modifier_group.archived", eventType: "food.modifier_group.archived",
			aggregateType: "food_modifier_group", aggregateID: archived.ID,
			version: archived.Version, before: ModifierGroupViewOf(group), after: ModifierGroupViewOf(archived),
			payload: map[string]any{"group": ModifierGroupViewOf(archived), "detached_item_ids": detached},
			reason:  "cascade_detached_items",
		})
	})
	return archived, err
}

// ---------------------------------------------------------------------------
// Modifiers
// ---------------------------------------------------------------------------

type CreateModifierInput struct {
	Name            string
	PriceDeltaMinor int64
}

// validateModifierPrice refuses a negative delta.
//
// A negative modifier would be a discount, and discounts belong to promotions.
// Allowing one here would turn the catalogue into a hidden promotion system
// that no promotion authority could see or govern.
func validateModifierPrice(value int64) *apperrors.FieldViolation {
	if value < 0 {
		return &apperrors.FieldViolation{Field: "price_delta_minor",
			Message: "A modifier price must not be negative. Discounts are handled by promotions, not modifiers."}
	}
	if value > MaxPriceMinor {
		return &apperrors.FieldViolation{Field: "price_delta_minor", Message: "Price is outside the supported range."}
	}
	return nil
}

// CreateModifier adds a choice to the end of a group's ordering.
func (s *Service) CreateModifier(ctx context.Context, actor Actor, scope Scope, groupID string, in CreateModifierInput) (Modifier, error) {
	if err := s.requireWrites(); err != nil {
		return Modifier{}, err
	}
	if err := requireBusinessWide(scope); err != nil {
		return Modifier{}, err
	}
	name, violation := validateName("name", in.Name, 1, 120)
	if violation != nil {
		return Modifier{}, validation(*violation)
	}
	if violation = validateModifierPrice(in.PriceDeltaMinor); violation != nil {
		return Modifier{}, validation(*violation)
	}
	// A priced modifier is a price. A zero-delta choice ("no sauce") is not.
	if in.PriceDeltaMinor > 0 {
		if err := s.requirePricingAuthority(ctx, actor, ""); err != nil {
			return Modifier{}, err
		}
	}

	var modifier Modifier
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		group, err := loadGroup(ctx, tx, actor.VendorID, groupID, true)
		if err != nil {
			return err
		}
		if group.Status == ChildArchived {
			return errArchived("Modifier group")
		}
		position, err := nextPosition(ctx, tx, modifiersOrdering, group.ID)
		if err != nil {
			return err
		}
		modifier, err = scanModifier(tx.QueryRow(ctx, `
			INSERT INTO food_modifiers (id,business_id,modifier_group_id,name,price_delta_minor,position,status)
			VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE') RETURNING `+modifierCols,
			newID("fmd_"), actor.VendorID, group.ID, name, in.PriceDeltaMinor, position))
		if err != nil {
			return mapDB(err, "Modifier", CodeModifierNotFound)
		}
		if err = resequence(ctx, tx, modifiersOrdering, group.ID); err != nil {
			return err
		}
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		// A modifier belongs to a business-level group and therefore to no
		// single branch or item; its history row records neither rather than
		// guessing one.
		priceVersionRef, err := s.recordPrice(ctx, tx, actor, priceRecord{
			scope: catalogueScope{BusinessID: actor.VendorID}, subjectType: PriceSubjectModifier,
			subjectRef: modifier.ID, currency: currency,
			previous: nil, amount: &modifier.PriceDeltaMinor,
			changeReason: PriceChangeCreated, version: modifier.Version,
		})
		if err != nil {
			return err
		}
		revisions, err := bumpRevisionsForModifierGroup(ctx, tx, actor.VendorID, group.ID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.modifier.created", eventType: "food.modifier.created",
			aggregateType: "food_modifier", aggregateID: modifier.ID,
			version: modifier.Version, after: ModifierViewOf(modifier),
			payload: map[string]any{"modifier": ModifierViewOf(modifier),
				"price_version_ref": priceVersionRef, "catalogue_revisions": revisions},
		})
	})
	return modifier, err
}

// ListModifiers returns a group's modifiers in deterministic order.
func (s *Service) ListModifiers(ctx context.Context, actor Actor, groupID string, includeArchived bool) ([]Modifier, error) {
	group, err := loadGroup(ctx, s.db, actor.VendorID, groupID, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+modifierCols+` FROM food_modifiers
		WHERE business_id=$1 AND modifier_group_id=$2 AND ($3 OR status <> 'ARCHIVED')
		ORDER BY (status = 'ARCHIVED'), position, id`, actor.VendorID, group.ID, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	modifiers := []Modifier{}
	for rows.Next() {
		modifier, scanErr := scanModifier(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		modifiers = append(modifiers, modifier)
	}
	return modifiers, rows.Err()
}

type UpdateModifierInput struct {
	Name            *string
	PriceDeltaMinor *int64
	ExpectedVersion int64
}

// UpdateModifier edits a modifier addressed through its own group.
func (s *Service) UpdateModifier(ctx context.Context, actor Actor, scope Scope, groupID, modifierID string, in UpdateModifierInput) (Modifier, error) {
	if err := s.requireWrites(); err != nil {
		return Modifier{}, err
	}
	if err := requireBusinessWide(scope); err != nil {
		return Modifier{}, err
	}
	var updated Modifier
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		group, err := loadGroup(ctx, tx, actor.VendorID, groupID, true)
		if err != nil {
			return err
		}
		modifier, err := loadModifier(ctx, tx, actor.VendorID, group.ID, modifierID, true)
		if err != nil {
			return err
		}
		if modifier.Status == ChildArchived {
			return errArchived("Modifier")
		}
		if modifier.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		name := modifier.Name
		if in.Name != nil {
			value, violation := validateName("name", *in.Name, 1, 120)
			if violation != nil {
				return validation(*violation)
			}
			name = value
		}
		delta := modifier.PriceDeltaMinor
		if in.PriceDeltaMinor != nil {
			if violation := validateModifierPrice(*in.PriceDeltaMinor); violation != nil {
				return validation(*violation)
			}
			delta = *in.PriceDeltaMinor
		}
		if delta != modifier.PriceDeltaMinor {
			if err = s.requirePricingAuthority(ctx, actor, ""); err != nil {
				return err
			}
		}

		updated, err = scanModifier(tx.QueryRow(ctx, `
			UPDATE food_modifiers SET name=$3, price_delta_minor=$4, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+modifierCols,
			actor.VendorID, modifier.ID, name, delta))
		if err != nil {
			return mapDB(err, "Modifier", CodeModifierNotFound)
		}

		var priceVersionRef string
		if updated.PriceDeltaMinor != modifier.PriceDeltaMinor {
			currency, curErr := s.businessCurrency(ctx, tx, actor.VendorID)
			if curErr != nil {
				return curErr
			}
			previous := modifier.PriceDeltaMinor
			priceVersionRef, err = s.recordPrice(ctx, tx, actor, priceRecord{
				scope: catalogueScope{BusinessID: actor.VendorID}, subjectType: PriceSubjectModifier,
				subjectRef: updated.ID, currency: currency,
				previous: &previous, amount: &updated.PriceDeltaMinor,
				changeReason: PriceChanged, version: updated.Version,
			})
			if err != nil {
				return err
			}
		}
		// A shared modifier's price moves every branch that attaches its group,
		// so the revision bump fans out over exactly those scopes rather than
		// telling uninvolved branches their catalogue changed.
		revisions, err := bumpRevisionsForModifierGroup(ctx, tx, actor.VendorID, group.ID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.modifier.updated", eventType: "food.modifier.updated",
			aggregateType: "food_modifier", aggregateID: updated.ID,
			version: updated.Version, before: ModifierViewOf(modifier), after: ModifierViewOf(updated),
			payload: map[string]any{"modifier": ModifierViewOf(updated),
				"price_version_ref": stringPtrOrNil(priceVersionRef), "catalogue_revisions": revisions},
		})
	})
	return updated, err
}

// ArchiveModifier retires one choice.
func (s *Service) ArchiveModifier(ctx context.Context, actor Actor, scope Scope, groupID, modifierID string, expectedVersion int64) (Modifier, error) {
	if err := s.requireWrites(); err != nil {
		return Modifier{}, err
	}
	if err := requireBusinessWide(scope); err != nil {
		return Modifier{}, err
	}
	var archived Modifier
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		group, err := loadGroup(ctx, tx, actor.VendorID, groupID, true)
		if err != nil {
			return err
		}
		modifier, err := loadModifier(ctx, tx, actor.VendorID, group.ID, modifierID, true)
		if err != nil {
			return err
		}
		if modifier.Status == ChildArchived {
			return errArchived("Modifier")
		}
		if modifier.Version != expectedVersion {
			return errStaleVersion()
		}
		archived, err = scanModifier(tx.QueryRow(ctx, `
			UPDATE food_modifiers SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+modifierCols, actor.VendorID, modifier.ID))
		if err != nil {
			return mapDB(err, "Modifier", CodeModifierNotFound)
		}
		if err = resequence(ctx, tx, modifiersOrdering, group.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.modifier.archived", eventType: "food.modifier.archived",
			aggregateType: "food_modifier", aggregateID: archived.ID,
			version: archived.Version, before: ModifierViewOf(modifier), after: ModifierViewOf(archived),
			payload: ModifierViewOf(archived),
		})
	})
	return archived, err
}

// ReorderModifiers applies a complete new ordering within one group.
func (s *Service) ReorderModifiers(ctx context.Context, actor Actor, scope Scope, groupID string, orderedIDs []string) ([]Modifier, error) {
	if err := s.requireWrites(); err != nil {
		return nil, err
	}
	if err := requireBusinessWide(scope); err != nil {
		return nil, err
	}
	var modifiers []Modifier
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		group, err := loadGroup(ctx, tx, actor.VendorID, groupID, true)
		if err != nil {
			return err
		}
		if group.Status == ChildArchived {
			return errArchived("Modifier group")
		}
		if err = applyReorder(ctx, tx, modifiersOrdering, group.ID, orderedIDs); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+modifierCols+` FROM food_modifiers
			WHERE business_id=$1 AND modifier_group_id=$2 AND status <> 'ARCHIVED' ORDER BY position, id`,
			actor.VendorID, group.ID)
		if err != nil {
			return err
		}
		modifiers = []Modifier{}
		for rows.Next() {
			modifier, scanErr := scanModifier(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			modifiers = append(modifiers, modifier)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		order := make([]string, 0, len(modifiers))
		for _, modifier := range modifiers {
			order = append(order, modifier.ID)
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.modifier.reordered", eventType: "food.modifier.reordered",
			aggregateType: "food_modifier_group", aggregateID: group.ID,
			version: group.Version, after: order,
			payload: map[string]any{"modifier_group_id": group.ID, "ordered_modifier_ids": order},
		})
	})
	return modifiers, err
}

// ---------------------------------------------------------------------------
// Item <-> modifier group relationships
// ---------------------------------------------------------------------------

// AttachModifierGroup links a shared group to an item.
//
// Branch-scoped staff may do this for their own branch's items: attaching is a
// decision about one item, not an edit of the shared definition.
func (s *Service) AttachModifierGroup(ctx context.Context, actor Actor, scope Scope, itemID, groupID string) (ItemModifierGroup, error) {
	if err := s.requireWrites(); err != nil {
		return ItemModifierGroup{}, err
	}
	var attachment ItemModifierGroup
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		// Loaded within the tenant, so a group belonging to another business is
		// simply absent rather than refused with a message that confirms it
		// exists somewhere.
		group, err := loadGroup(ctx, tx, actor.VendorID, groupID, true)
		if err != nil {
			return err
		}
		if group.Status == ChildArchived {
			return errArchived("Modifier group")
		}
		position, err := nextPosition(ctx, tx, attachedOrdering, item.ID)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO food_item_modifier_groups (id,business_id,item_id,modifier_group_id,position)
			VALUES ($1,$2,$3,$4,$5)
			RETURNING id,business_id,item_id,modifier_group_id,position,created_at,updated_at,version`,
			newID("fig_"), actor.VendorID, item.ID, group.ID, position)
		if err = row.Scan(&attachment.ID, &attachment.BusinessID, &attachment.ItemID, &attachment.ModifierGroupID,
			&attachment.Position, &attachment.CreatedAt, &attachment.UpdatedAt, &attachment.Version); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return coded(apperrors.Conflict("This modifier group is already attached to the item.", nil),
					CodeDuplicateRelationship)
			}
			return mapDB(err, "Modifier group attachment", CodeModifierGroupNotFound)
		}
		if err = resequence(ctx, tx, attachedOrdering, item.ID); err != nil {
			return err
		}
		// Attaching a group changes what the item can be configured as, and
		// therefore what it can cost, so it is a discovery-visible change.
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.modifier_group_attached", eventType: "food.item.modifier_group_attached",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: item.Version,
			after:   map[string]any{"modifier_group_id": group.ID, "position": attachment.Position},
			payload: map[string]any{"item_id": item.ID, "modifier_group_id": group.ID,
				"position": attachment.Position, "catalogue_revision": revision},
		})
	})
	return attachment, err
}

// DetachModifierGroup removes only the relationship. The group itself, and
// every other item using it, are untouched.
func (s *Service) DetachModifierGroup(ctx context.Context, actor Actor, scope Scope, itemID, groupID string) error {
	if err := s.requireWrites(); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			DELETE FROM food_item_modifier_groups
			WHERE business_id=$1 AND item_id=$2 AND modifier_group_id=$3`,
			actor.VendorID, item.ID, strings.TrimSpace(groupID))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound("Modifier group attachment", CodeModifierGroupNotFound)
		}
		if err = resequence(ctx, tx, attachedOrdering, item.ID); err != nil {
			return err
		}
		revision, err := s.bumpForItem(ctx, tx, item)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.modifier_group_detached", eventType: "food.item.modifier_group_detached",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: item.Version,
			before:  map[string]any{"modifier_group_id": strings.TrimSpace(groupID)},
			payload: map[string]any{"item_id": item.ID, "modifier_group_id": strings.TrimSpace(groupID),
				"catalogue_revision": revision},
		})
	})
}

// ListAttachedModifierGroups returns an item's groups, in display order, with
// each group's definition joined in one query rather than fetched per row.
func (s *Service) ListAttachedModifierGroups(ctx context.Context, actor Actor, scope Scope, itemID string) ([]AttachedGroupView, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return nil, err
	}
	if !scope.Covers(item.BranchID) {
		return nil, errNotFound("Food item", CodeItemNotFound)
	}
	rows, err := s.db.Query(ctx, `
		SELECT link.modifier_group_id, link.position, link.version,
			g.id,g.business_id,g.name,g.description,g.min_select,g.max_select,g.status,
			g.created_at,g.updated_at,g.archived_at,g.version
		FROM food_item_modifier_groups link
		JOIN food_modifier_groups g ON g.id = link.modifier_group_id AND g.business_id = link.business_id
		WHERE link.business_id=$1 AND link.item_id=$2
		ORDER BY link.position, link.id`, actor.VendorID, item.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AttachedGroupView{}
	for rows.Next() {
		var view AttachedGroupView
		var group ModifierGroup
		if err = rows.Scan(&view.ModifierGroupID, &view.Position, &view.Version,
			&group.ID, &group.BusinessID, &group.Name, &group.Description, &group.MinSelect, &group.MaxSelect,
			&group.Status, &group.CreatedAt, &group.UpdatedAt, &group.ArchivedAt, &group.Version); err != nil {
			return nil, err
		}
		view.Group = ModifierGroupViewOf(group)
		out = append(out, view)
	}
	return out, rows.Err()
}

// ReorderAttachedModifierGroups applies a complete new display order.
func (s *Service) ReorderAttachedModifierGroups(ctx context.Context, actor Actor, scope Scope, itemID string, orderedGroupIDs []string) ([]AttachedGroupView, error) {
	if err := s.requireWrites(); err != nil {
		return nil, err
	}
	var order []string
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		// The reorder contract addresses attachments by GROUP id, which is what a
		// client naturally holds; they are translated to relationship-row ids
		// here so the shared reorder routine still validates against real rows.
		linkByGroup := map[string]string{}
		rows, err := tx.Query(ctx, `
			SELECT id, modifier_group_id FROM food_item_modifier_groups
			WHERE business_id=$1 AND item_id=$2 FOR UPDATE`, actor.VendorID, item.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var linkID, groupID string
			if err = rows.Scan(&linkID, &groupID); err != nil {
				rows.Close()
				return err
			}
			linkByGroup[groupID] = linkID
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}

		linkIDs := make([]string, 0, len(orderedGroupIDs))
		for _, raw := range orderedGroupIDs {
			groupID := strings.TrimSpace(raw)
			linkID, found := linkByGroup[groupID]
			if !found {
				return errInvalidReorder("One or more ids do not belong to this collection.")
			}
			linkIDs = append(linkIDs, linkID)
		}
		if err = applyReorder(ctx, tx, attachedOrdering, item.ID, linkIDs); err != nil {
			return err
		}
		ordered, err := tx.Query(ctx, `
			SELECT modifier_group_id FROM food_item_modifier_groups
			WHERE business_id=$1 AND item_id=$2 ORDER BY position, id`, actor.VendorID, item.ID)
		if err != nil {
			return err
		}
		order = []string{}
		for ordered.Next() {
			var groupID string
			if err = ordered.Scan(&groupID); err != nil {
				ordered.Close()
				return err
			}
			order = append(order, groupID)
		}
		ordered.Close()
		if err = ordered.Err(); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.modifier_groups_reordered", eventType: "food.item.modifier_groups_reordered",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: item.Version, after: order,
			payload: map[string]any{"item_id": item.ID, "ordered_modifier_group_ids": order},
		})
	})
	if err != nil {
		return nil, err
	}
	return s.ListAttachedModifierGroups(ctx, actor, scope, itemID)
}
