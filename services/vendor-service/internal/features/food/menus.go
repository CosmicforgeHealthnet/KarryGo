package food

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Menus
// ---------------------------------------------------------------------------

// CreateMenuInput is the validated shape of a menu creation command.
type CreateMenuInput struct {
	BranchID       string
	CapabilityType string
	Name           string
	Description    string
}

// MenuListOptions bounds the list surface. Every list endpoint in this package
// paginates; none can be asked for an unbounded page.
type MenuListOptions struct {
	BranchID       string
	CapabilityType string
	Status         string
	Limit          int
	Offset         int
}

func (o MenuListOptions) bounded() MenuListOptions {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	if o.Limit > 100 {
		o.Limit = 100
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

// CreateMenu records a new DRAFT menu for a branch and food capability.
//
// A menu is created DRAFT and nothing else: catalogue preparation must be
// possible while the business is still onboarding, but publishing is a
// separate command with its own prerequisites (see ActivateMenu).
func (s *Service) CreateMenu(ctx context.Context, actor Actor, scope Scope, in CreateMenuInput) (Menu, error) {
	if err := s.requireWrites(); err != nil {
		return Menu{}, err
	}
	branchID := strings.TrimSpace(in.BranchID)
	capability := strings.ToUpper(strings.TrimSpace(in.CapabilityType))

	var violations []apperrors.FieldViolation
	if branchID == "" {
		violations = append(violations, apperrors.FieldViolation{Field: "branch_id", Message: "Branch is required."})
	}
	name, violation := validateName("name", in.Name, 2, 160)
	if violation != nil {
		violations = append(violations, *violation)
	}
	description, violation := validateDescription("description", in.Description, 2000)
	if violation != nil {
		violations = append(violations, *violation)
	}
	if len(violations) > 0 {
		return Menu{}, validation(violations...)
	}
	if !IsFoodCapability(capability) {
		return Menu{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "capability_type", Message: "Section 9A supports RESTAURANT and FAST_FOOD menus only."}}),
			CodeCapabilityNotSupported)
	}
	if !scope.Covers(branchID) {
		return Menu{}, errForbidden()
	}

	var menu Menu
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// The branch must exist in this business and must actually hold the
		// capability. Section 6/7 remains the authority for both; this only
		// reads their state.
		if err := s.requireBranchCapability(ctx, tx, actor.VendorID, branchID, capability, false); err != nil {
			return err
		}
		row, err := scanMenu(tx.QueryRow(ctx, `
			INSERT INTO food_menus (id,business_id,branch_id,capability_type,name,description,status)
			VALUES ($1,$2,$3,$4,$5,$6,'DRAFT') RETURNING `+menuCols,
			newID("fmn_"), actor.VendorID, branchID, capability, name, description))
		if err != nil {
			return mapDB(err, "Menu", CodeMenuNotFound)
		}
		menu = row
		return s.record(ctx, tx, actor, evidence{
			action: "food.menu.created", eventType: "food.menu.created",
			aggregateType: "food_menu", aggregateID: menu.ID, branchID: menu.BranchID,
			version: menu.Version, after: MenuViewOf(menu), payload: MenuViewOf(menu),
		})
	})
	return menu, err
}

// requireBranchCapability reads Section 6/7 state rather than duplicating it.
//
// forPublish tightens the same check: a DRAFT catalogue may be prepared for a
// branch whose capability is still being set up, but publishing one demands
// that the business, the branch and the capability are all genuinely
// operational.
func (s *Service) requireBranchCapability(ctx context.Context, q querier, businessID, branchID, capability string, forPublish bool) error {
	var branchStatus, businessStatus string
	err := q.QueryRow(ctx, `
		SELECT b.status, biz.status FROM vendor_branches b
		JOIN vendor_businesses biz ON biz.id = b.business_id
		WHERE b.business_id=$1 AND b.id=$2`, businessID, branchID).Scan(&branchStatus, &businessStatus)
	if err != nil {
		return mapDB(err, "Branch", CodeMenuNotFound)
	}

	var capabilityStatus string
	err = q.QueryRow(ctx, `
		SELECT status FROM vendor_branch_capabilities
		WHERE business_id=$1 AND branch_id=$2 AND capability_type=$3`, businessID, branchID, capability).Scan(&capabilityStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coded(apperrors.Conflict("This branch does not have the "+capability+" capability.", nil),
				CodeCapabilityNotSupported)
		}
		return err
	}
	if !forPublish {
		return nil
	}
	if businessStatus != "ACTIVE" {
		return coded(apperrors.Conflict("The business is not active, so its menu cannot be published.", nil), CodeNotPublishable)
	}
	if branchStatus != "ACTIVE" {
		return coded(apperrors.Conflict("The branch is not active, so its menu cannot be published.", nil), CodeNotPublishable)
	}
	if capabilityStatus != "ACTIVE" {
		return coded(apperrors.Conflict("The branch capability is not active, so its menu cannot be published.", nil), CodeNotPublishable)
	}
	return nil
}

// ListMenus returns the menus the caller's scope reaches, newest first.
func (s *Service) ListMenus(ctx context.Context, actor Actor, scope Scope, opts MenuListOptions) ([]Menu, bool, error) {
	opts = opts.bounded()
	status := strings.ToUpper(strings.TrimSpace(opts.Status))
	switch status {
	case "", StatusDraft, StatusActive, StatusPaused, StatusArchived:
	default:
		return nil, false, validation(apperrors.FieldViolation{Field: "status", Message: "Unsupported status filter."})
	}
	capability := strings.ToUpper(strings.TrimSpace(opts.CapabilityType))
	if capability != "" && !IsFoodCapability(capability) {
		return nil, false, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "capability_type", Message: "Section 9A supports RESTAURANT and FAST_FOOD menus only."}}),
			CodeCapabilityNotSupported)
	}
	branchFilter := strings.TrimSpace(opts.BranchID)
	if branchFilter != "" && !scope.Covers(branchFilter) {
		return nil, false, errForbidden()
	}

	// A branch-scoped caller is constrained in SQL, not by filtering afterwards:
	// the rows for another branch never leave the database.
	var branches []string
	if !scope.Global {
		branches = scope.BranchIDs
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+menuCols+` FROM food_menus
		WHERE business_id=$1
		  AND ($2 = '' OR branch_id = $2)
		  AND ($3::text[] IS NULL OR branch_id = ANY($3))
		  AND ($4 = '' OR capability_type = $4)
		  AND ($5 = '' OR status = $5)
		ORDER BY created_at DESC, id
		LIMIT $6 OFFSET $7`,
		actor.VendorID, branchFilter, branches, capability, status, opts.Limit+1, opts.Offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	menus := make([]Menu, 0, opts.Limit)
	for rows.Next() {
		menu, scanErr := scanMenu(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		menus = append(menus, menu)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(menus) > opts.Limit
	if hasMore {
		menus = menus[:opts.Limit]
	}
	return menus, hasMore, nil
}

// GetMenu reads one menu inside the caller's reach.
func (s *Service) GetMenu(ctx context.Context, actor Actor, scope Scope, menuID string) (Menu, error) {
	menu, err := loadMenu(ctx, s.db, actor.VendorID, menuID, false)
	if err != nil {
		return Menu{}, err
	}
	if !scope.Covers(menu.BranchID) {
		// Indistinguishable from absent, so a branch-scoped actor cannot probe
		// another branch's menu ids.
		return Menu{}, errNotFound("Menu", CodeMenuNotFound)
	}
	return menu, nil
}

// UpdateMenuInput carries only the fields a menu edit may change. Branch and
// capability are absent on purpose: moving a menu between branches would
// silently re-home every item under it.
type UpdateMenuInput struct {
	Name            *string
	Description     *string
	ExpectedVersion int64
}

// UpdateMenu edits menu metadata under optimistic concurrency.
func (s *Service) UpdateMenu(ctx context.Context, actor Actor, scope Scope, menuID string, in UpdateMenuInput) (Menu, error) {
	if err := s.requireWrites(); err != nil {
		return Menu{}, err
	}
	var updated Menu
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		if menu.Status == StatusArchived {
			return errArchived("Menu")
		}
		if menu.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		name, description := menu.Name, menu.Description
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
		if len(violations) > 0 {
			return validation(violations...)
		}

		updated, err = scanMenu(tx.QueryRow(ctx, `
			UPDATE food_menus SET name=$3, description=$4, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+menuCols,
			actor.VendorID, menu.ID, name, description))
		if err != nil {
			return mapDB(err, "Menu", CodeMenuNotFound)
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.menu.updated", eventType: "food.menu.updated",
			aggregateType: "food_menu", aggregateID: updated.ID, branchID: updated.BranchID,
			version: updated.Version, before: MenuViewOf(menu), after: MenuViewOf(updated),
			payload: MenuViewOf(updated),
		})
	})
	return updated, err
}

// lockedMenu is the common preamble of every menu mutation: read it inside the
// transaction with a row lock, and refuse it outright if the caller's scope
// does not reach its branch.
func (s *Service) lockedMenu(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, menuID string) (Menu, error) {
	menu, err := loadMenu(ctx, tx, actor.VendorID, menuID, true)
	if err != nil {
		return Menu{}, err
	}
	if !scope.Covers(menu.BranchID) {
		return Menu{}, errNotFound("Menu", CodeMenuNotFound)
	}
	return menu, nil
}

// menuTransition is the closed lifecycle. ARCHIVED is terminal: an archived
// menu is retained for historical reference and is never reactivated, because
// something may already point at it as it was.
func menuTransition(from, to string) error {
	if from == to {
		return coded(apperrors.Conflict("The menu is already "+strings.ToLower(to)+".", nil), CodeMenuNotEditable)
	}
	if from == StatusArchived {
		return errArchived("Menu")
	}
	switch to {
	case StatusActive:
		if from != StatusDraft && from != StatusPaused {
			return coded(apperrors.Conflict("Only a draft or paused menu can be activated.", nil), CodeMenuNotEditable)
		}
	case StatusPaused:
		if from != StatusActive {
			return coded(apperrors.Conflict("Only an active menu can be paused.", nil), CodeMenuNotEditable)
		}
	case StatusArchived:
		// Any non-archived state may be retired.
	default:
		return coded(apperrors.Conflict("Unsupported menu status.", nil), CodeMenuNotEditable)
	}
	return nil
}

// SetMenuStatus applies activate/pause/archive.
//
// Activation is the one transition with real prerequisites, and it reuses
// Section 6/7's business, branch and capability state rather than creating a
// second onboarding authority.
func (s *Service) SetMenuStatus(ctx context.Context, actor Actor, scope Scope, menuID, target string, expectedVersion int64) (Menu, error) {
	if err := s.requireWrites(); err != nil {
		return Menu{}, err
	}
	var updated Menu
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		if menu.Version != expectedVersion {
			return errStaleVersion()
		}
		if err = menuTransition(menu.Status, target); err != nil {
			return err
		}
		if target == StatusActive {
			if err = s.requireBranchCapability(ctx, tx, actor.VendorID, menu.BranchID, menu.CapabilityType, true); err != nil {
				return err
			}
		}

		archivedAt := "NULL"
		if target == StatusArchived {
			archivedAt = "now()"
		}
		updated, err = scanMenu(tx.QueryRow(ctx, `
			UPDATE food_menus SET status=$3, archived_at=`+archivedAt+`, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+menuCols,
			actor.VendorID, menu.ID, target))
		if err != nil {
			return mapDB(err, "Menu", CodeMenuNotFound)
		}

		// Section 9B: publication state is the first thing a future reader
		// consults, so it moves the catalogue revision in the same transaction.
		revision, err := bumpRevision(ctx, tx, scopeOfMenu(updated))
		if err != nil {
			return err
		}
		action := map[string]string{StatusActive: "activated", StatusPaused: "paused", StatusArchived: "archived"}[target]
		return s.record(ctx, tx, actor, evidence{
			action: "food.menu." + action, eventType: "food.menu." + action,
			aggregateType: "food_menu", aggregateID: updated.ID, branchID: updated.BranchID,
			version: updated.Version, before: MenuViewOf(menu), after: MenuViewOf(updated),
			payload: map[string]any{"menu": MenuViewOf(updated), "catalogue_revision": revision},
		})
	})
	return updated, err
}

// ---------------------------------------------------------------------------
// Menu sections
// ---------------------------------------------------------------------------

type CreateSectionInput struct {
	Name        string
	Description string
}

// CreateSection appends a section to the end of a menu's ordering.
func (s *Service) CreateSection(ctx context.Context, actor Actor, scope Scope, menuID string, in CreateSectionInput) (Section, error) {
	if err := s.requireWrites(); err != nil {
		return Section{}, err
	}
	name, violation := validateName("name", in.Name, 2, 160)
	if violation != nil {
		return Section{}, validation(*violation)
	}
	description, violation := validateDescription("description", in.Description, 2000)
	if violation != nil {
		return Section{}, validation(*violation)
	}

	var section Section
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		if menu.Status == StatusArchived {
			return errArchived("Menu")
		}
		position, err := nextPosition(ctx, tx, sectionsOrdering, menu.ID)
		if err != nil {
			return err
		}
		section, err = scanSection(tx.QueryRow(ctx, `
			INSERT INTO food_menu_sections (id,business_id,menu_id,name,description,position,status)
			VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE') RETURNING `+sectionCols,
			newID("fms_"), actor.VendorID, menu.ID, name, description, position))
		if err != nil {
			return mapDB(err, "Menu section", CodeSectionNotFound)
		}
		if err = resequence(ctx, tx, sectionsOrdering, menu.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.section.created", eventType: "food.section.created",
			aggregateType: "food_menu_section", aggregateID: section.ID, branchID: menu.BranchID,
			version: section.Version, after: SectionViewOf(section), payload: SectionViewOf(section),
		})
	})
	return section, err
}

// ListSections returns a menu's sections in their deterministic order.
func (s *Service) ListSections(ctx context.Context, actor Actor, scope Scope, menuID string, includeArchived bool) ([]Section, error) {
	menu, err := s.GetMenu(ctx, actor, scope, menuID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+sectionCols+` FROM food_menu_sections
		WHERE business_id=$1 AND menu_id=$2 AND ($3 OR status <> 'ARCHIVED')
		ORDER BY (status = 'ARCHIVED'), position, id`,
		actor.VendorID, menu.ID, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sections := []Section{}
	for rows.Next() {
		section, scanErr := scanSection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sections = append(sections, section)
	}
	return sections, rows.Err()
}

type UpdateSectionInput struct {
	Name            *string
	Description     *string
	ExpectedVersion int64
}

// UpdateSection edits a section addressed through its own menu.
func (s *Service) UpdateSection(ctx context.Context, actor Actor, scope Scope, menuID, sectionID string, in UpdateSectionInput) (Section, error) {
	if err := s.requireWrites(); err != nil {
		return Section{}, err
	}
	var updated Section
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		section, err := loadSection(ctx, tx, actor.VendorID, menu.ID, sectionID, true)
		if err != nil {
			return err
		}
		if section.Status == ChildArchived {
			return errArchived("Menu section")
		}
		if section.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		name, description := section.Name, section.Description
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
		if len(violations) > 0 {
			return validation(violations...)
		}

		updated, err = scanSection(tx.QueryRow(ctx, `
			UPDATE food_menu_sections SET name=$3, description=$4, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+sectionCols,
			actor.VendorID, section.ID, name, description))
		if err != nil {
			return mapDB(err, "Menu section", CodeSectionNotFound)
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.section.updated", eventType: "food.section.updated",
			aggregateType: "food_menu_section", aggregateID: updated.ID, branchID: menu.BranchID,
			version: updated.Version, before: SectionViewOf(section), after: SectionViewOf(updated),
			payload: SectionViewOf(updated),
		})
	})
	return updated, err
}

// ArchiveSection retires a section and, with it, the items under it.
//
// Items are archived rather than orphaned or deleted: an item whose section
// disappeared would be unreachable through the catalogue while still being
// referenced by whatever already knows its id, which is exactly the state
// archive semantics exist to prevent.
func (s *Service) ArchiveSection(ctx context.Context, actor Actor, scope Scope, menuID, sectionID string, expectedVersion int64) (Section, error) {
	if err := s.requireWrites(); err != nil {
		return Section{}, err
	}
	var archived Section
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		section, err := loadSection(ctx, tx, actor.VendorID, menu.ID, sectionID, true)
		if err != nil {
			return err
		}
		if section.Status == ChildArchived {
			return errArchived("Menu section")
		}
		if section.Version != expectedVersion {
			return errStaleVersion()
		}

		archived, err = scanSection(tx.QueryRow(ctx, `
			UPDATE food_menu_sections SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+sectionCols,
			actor.VendorID, section.ID))
		if err != nil {
			return mapDB(err, "Menu section", CodeSectionNotFound)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE food_items SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND section_id=$2 AND status <> 'ARCHIVED'`, actor.VendorID, section.ID)
		if err != nil {
			return err
		}
		if err = resequence(ctx, tx, itemsOrdering, section.ID); err != nil {
			return err
		}
		if err = resequence(ctx, tx, sectionsOrdering, menu.ID); err != nil {
			return err
		}
		revision, err := bumpRevision(ctx, tx, scopeOfMenu(menu))
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.section.archived", eventType: "food.section.archived",
			aggregateType: "food_menu_section", aggregateID: archived.ID, branchID: menu.BranchID,
			version: archived.Version, before: SectionViewOf(section), after: SectionViewOf(archived),
			payload: map[string]any{"section": SectionViewOf(archived), "items_archived": tag.RowsAffected(),
				"catalogue_revision": revision},
			reason: "cascade_archived_items",
		})
	})
	return archived, err
}

// ReorderSections applies a complete new ordering atomically.
func (s *Service) ReorderSections(ctx context.Context, actor Actor, scope Scope, menuID string, orderedIDs []string) ([]Section, error) {
	if err := s.requireWrites(); err != nil {
		return nil, err
	}
	var sections []Section
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		if menu.Status == StatusArchived {
			return errArchived("Menu")
		}
		if err = applyReorder(ctx, tx, sectionsOrdering, menu.ID, orderedIDs); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+sectionCols+` FROM food_menu_sections
			WHERE business_id=$1 AND menu_id=$2 AND status <> 'ARCHIVED' ORDER BY position, id`,
			actor.VendorID, menu.ID)
		if err != nil {
			return err
		}
		sections = []Section{}
		for rows.Next() {
			section, scanErr := scanSection(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			sections = append(sections, section)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		order := make([]string, 0, len(sections))
		for _, section := range sections {
			order = append(order, section.ID)
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.section.reordered", eventType: "food.section.reordered",
			aggregateType: "food_menu", aggregateID: menu.ID, branchID: menu.BranchID,
			version: menu.Version, after: order,
			payload: map[string]any{"menu_id": menu.ID, "ordered_section_ids": order},
		})
	})
	return sections, err
}
