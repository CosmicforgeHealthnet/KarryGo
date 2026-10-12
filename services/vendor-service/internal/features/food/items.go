package food

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Food items
// ---------------------------------------------------------------------------

// CreateItemInput is one item as the vendor configures it. Prices are integer
// minor units of the business's currency; there is no float anywhere in this
// package's pricing path.
type CreateItemInput struct {
	Name           string
	Description    string
	BasePriceMinor int64
	DietaryCodes   []string
	AllergenCodes  []string
}

type validatedItemFields struct {
	name        string
	description string
	dietary     []string
	allergens   []string
}

func validateItemFields(name, description string, dietary, allergens []string) (validatedItemFields, error) {
	var violations []apperrors.FieldViolation
	fields := validatedItemFields{}

	value, violation := validateName("name", name, 2, 160)
	if violation != nil {
		violations = append(violations, *violation)
	}
	fields.name = value

	value, violation = validateDescription("description", description, 4000)
	if violation != nil {
		violations = append(violations, *violation)
	}
	fields.description = value

	codes, violation := normalizeCodes("dietary_codes", dietary, MaxDietaryCodes)
	if violation != nil {
		violations = append(violations, *violation)
	}
	fields.dietary = codes

	codes, violation = normalizeCodes("allergen_codes", allergens, MaxAllergenCodes)
	if violation != nil {
		violations = append(violations, *violation)
	}
	fields.allergens = codes

	if len(violations) > 0 {
		return validatedItemFields{}, validation(violations...)
	}
	return fields, nil
}

// CreateItem adds an item to the end of a section's ordering.
//
// Dietary and allergen codes are declarative vendor-supplied catalogue
// information. This service validates their shape and nothing else: it makes
// no medical claim, computes no nutrition and certifies nothing.
func (s *Service) CreateItem(ctx context.Context, actor Actor, scope Scope, menuID, sectionID string, in CreateItemInput) (Item, error) {
	if err := s.requireWrites(); err != nil {
		return Item{}, err
	}
	fields, err := validateItemFields(in.Name, in.Description, in.DietaryCodes, in.AllergenCodes)
	if err != nil {
		return Item{}, err
	}
	if violation := validatePriceMinor("base_price_minor", in.BasePriceMinor); violation != nil {
		return Item{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{*violation}), CodeInvalidReorder)
	}

	// Section 9B: establishing the base price is a pricing act even when the
	// chosen amount is zero. A free item is still a commercial decision, so a
	// kitchen/editorial grant cannot create one and thereby bypass the pricing
	// permission split.
	menu, menuErr := s.GetMenu(ctx, actor, scope, menuID)
	if menuErr != nil {
		return Item{}, menuErr
	}
	if err = s.requirePricingAuthority(ctx, actor, menu.BranchID); err != nil {
		return Item{}, err
	}

	var item Item
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		if menu.Status == StatusArchived {
			return errArchived("Menu")
		}
		section, err := loadSection(ctx, tx, actor.VendorID, menu.ID, sectionID, true)
		if err != nil {
			return err
		}
		if section.Status == ChildArchived {
			return errArchived("Menu section")
		}
		position, err := nextPosition(ctx, tx, itemsOrdering, section.ID)
		if err != nil {
			return err
		}
		dietary, _ := json.Marshal(fields.dietary)
		allergens, _ := json.Marshal(fields.allergens)
		item, err = scanItem(tx.QueryRow(ctx, `
			INSERT INTO food_items
				(id,business_id,branch_id,menu_id,section_id,name,description,base_price_minor,
				 dietary_codes,allergen_codes,status,position)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'DRAFT',$11) RETURNING `+itemCols,
			newID("fit_"), actor.VendorID, menu.BranchID, menu.ID, section.ID,
			fields.name, fields.description, in.BasePriceMinor, dietary, allergens, position))
		if err != nil {
			return mapDB(err, "Food item", CodeItemNotFound)
		}
		if err = resequence(ctx, tx, itemsOrdering, section.ID); err != nil {
			return err
		}
		// Section 9B: the item's opening price is its first immutable price
		// version, so there is no price this catalogue has ever carried that
		// history cannot account for.
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		catalogue := catalogueScope{BusinessID: menu.BusinessID, BranchID: menu.BranchID,
			Capability: menu.CapabilityType}
		branch := item.BranchID
		itemRef := item.ID
		priceVersionRef, err := s.recordPrice(ctx, tx, actor, priceRecord{
			scope: catalogue, branchID: &branch, subjectType: PriceSubjectItem,
			subjectRef: item.ID, itemRef: &itemRef, currency: currency,
			previous: nil, amount: &item.BasePriceMinor,
			changeReason: PriceChangeCreated, version: item.Version,
		})
		if err != nil {
			return err
		}
		revision, err := bumpRevision(ctx, tx, catalogue)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.created", eventType: "food.item.created",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: item.Version, after: ItemViewOf(item, "", nil),
			payload: map[string]any{"item": ItemViewOf(item, currency, nil),
				"price_version_ref": priceVersionRef, "catalogue_revision": revision},
		})
	})
	return item, err
}

// ListItems returns a section's items in deterministic order, with their media
// resolved in one extra query rather than one per item.
func (s *Service) ListItems(ctx context.Context, actor Actor, scope Scope, menuID, sectionID string, includeArchived bool, limit, offset int) ([]Item, map[string][]ItemMedia, string, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	menu, err := s.GetMenu(ctx, actor, scope, menuID)
	if err != nil {
		return nil, nil, "", false, err
	}
	section, err := loadSection(ctx, s.db, actor.VendorID, menu.ID, sectionID, false)
	if err != nil {
		return nil, nil, "", false, err
	}
	currency, err := s.businessCurrency(ctx, s.db, actor.VendorID)
	if err != nil {
		return nil, nil, "", false, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT `+itemCols+` FROM food_items
		WHERE business_id=$1 AND section_id=$2 AND ($3 OR status <> 'ARCHIVED')
		ORDER BY (status = 'ARCHIVED'), position, id
		LIMIT $4 OFFSET $5`,
		actor.VendorID, section.ID, includeArchived, limit+1, offset)
	if err != nil {
		return nil, nil, "", false, err
	}
	items := []Item{}
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			rows.Close()
			return nil, nil, "", false, scanErr
		}
		items = append(items, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, nil, "", false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	media, err := s.mediaForItems(ctx, s.db, actor.VendorID, ids)
	if err != nil {
		return nil, nil, "", false, err
	}
	return items, media, currency, hasMore, nil
}

// GetItem reads one item by id, refusing anything outside the caller's branch
// reach.
func (s *Service) GetItem(ctx context.Context, actor Actor, scope Scope, itemID string) (Item, []ItemMedia, string, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return Item{}, nil, "", err
	}
	if !scope.Covers(item.BranchID) {
		return Item{}, nil, "", errNotFound("Food item", CodeItemNotFound)
	}
	currency, err := s.businessCurrency(ctx, s.db, actor.VendorID)
	if err != nil {
		return Item{}, nil, "", err
	}
	media, err := s.mediaForItems(ctx, s.db, actor.VendorID, []string{item.ID})
	if err != nil {
		return Item{}, nil, "", err
	}
	return item, media[item.ID], currency, nil
}

// lockedItem is the common preamble of every item mutation.
func (s *Service) lockedItem(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, itemID string) (Item, error) {
	item, err := loadItem(ctx, tx, actor.VendorID, itemID, true)
	if err != nil {
		return Item{}, err
	}
	if !scope.Covers(item.BranchID) {
		return Item{}, errNotFound("Food item", CodeItemNotFound)
	}
	return item, nil
}

// UpdateItemInput carries only what an item edit may change. Section and menu
// are absent: moving an item is a different operation with different ordering
// consequences and is not part of Section 9A.
type UpdateItemInput struct {
	Name            *string
	Description     *string
	BasePriceMinor  *int64
	DietaryCodes    *[]string
	AllergenCodes   *[]string
	Status          *string
	ExpectedVersion int64
}

// itemTransition is the item lifecycle. As with menus, ARCHIVED is terminal.
// PAUSED here is administrative and is not Section 9B's sold-out state.
func itemTransition(from, to string) error {
	if from == StatusArchived {
		return errArchived("Food item")
	}
	switch to {
	case StatusDraft:
		if from != StatusPaused && from != StatusDraft {
			return coded(apperrors.Conflict("A published item cannot be returned to draft.", nil), CodeMenuNotEditable)
		}
	case StatusActive, StatusPaused:
		// Any non-archived item may be published or withheld.
	case StatusArchived:
		return coded(apperrors.Conflict("Use the archive action to retire an item.", nil), CodeMenuNotEditable)
	default:
		return coded(apperrors.Conflict("Unsupported item status.", nil), CodeMenuNotEditable)
	}
	return nil
}

// UpdateItem edits an item under optimistic concurrency.
func (s *Service) UpdateItem(ctx context.Context, actor Actor, scope Scope, itemID string, in UpdateItemInput) (Item, error) {
	if err := s.requireWrites(); err != nil {
		return Item{}, err
	}
	var updated Item
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		if item.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		name, description := item.Name, item.Description
		dietary, allergens := item.DietaryCodes, item.AllergenCodes
		if in.Name != nil {
			name = *in.Name
		}
		if in.Description != nil {
			description = *in.Description
		}
		if in.DietaryCodes != nil {
			dietary = *in.DietaryCodes
		}
		if in.AllergenCodes != nil {
			allergens = *in.AllergenCodes
		}
		fields, err := validateItemFields(name, description, dietary, allergens)
		if err != nil {
			return err
		}

		price := item.BasePriceMinor
		if in.BasePriceMinor != nil {
			if violation := validatePriceMinor("base_price_minor", *in.BasePriceMinor); violation != nil {
				return validation(*violation)
			}
			price = *in.BasePriceMinor
		}
		// Section 9B: changing what something costs needs the pricing grant on
		// top of the catalogue one. Checked against the value that would
		// actually change, so a request that merely echoes the current price
		// back is not treated as a price mutation.
		if price != item.BasePriceMinor {
			if err = s.requirePricingAuthority(ctx, actor, item.BranchID); err != nil {
				return err
			}
		}
		status := item.Status
		if in.Status != nil {
			status = strings.ToUpper(strings.TrimSpace(*in.Status))
			if err = itemTransition(item.Status, status); err != nil {
				return err
			}
			if status == StatusActive {
				// An item cannot be published under a menu that is not itself
				// releasable, otherwise "active" would mean two different things
				// depending on where you looked.
				menu, menuErr := loadMenu(ctx, tx, actor.VendorID, item.MenuID, false)
				if menuErr != nil {
					return menuErr
				}
				if menu.Status == StatusArchived {
					return errArchived("Menu")
				}
			}
		}

		dietaryJSON, _ := json.Marshal(fields.dietary)
		allergenJSON, _ := json.Marshal(fields.allergens)
		updated, err = scanItem(tx.QueryRow(ctx, `
			UPDATE food_items SET name=$3, description=$4, base_price_minor=$5,
				dietary_codes=$6, allergen_codes=$7, status=$8, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+itemCols,
			actor.VendorID, item.ID, fields.name, fields.description, price,
			dietaryJSON, allergenJSON, status))
		if err != nil {
			return mapDB(err, "Food item", CodeItemNotFound)
		}

		catalogue, err := scopeOfItem(ctx, tx, updated)
		if err != nil {
			return err
		}
		var priceVersionRef string
		if updated.BasePriceMinor != item.BasePriceMinor {
			currency, curErr := s.businessCurrency(ctx, tx, actor.VendorID)
			if curErr != nil {
				return curErr
			}
			branch := updated.BranchID
			itemRef := updated.ID
			previous := item.BasePriceMinor
			priceVersionRef, err = s.recordPrice(ctx, tx, actor, priceRecord{
				scope: catalogue, branchID: &branch, subjectType: PriceSubjectItem,
				subjectRef: updated.ID, itemRef: &itemRef, currency: currency,
				previous: &previous, amount: &updated.BasePriceMinor,
				changeReason: PriceChanged, version: updated.Version,
			})
			if err != nil {
				return err
			}
		}
		revision, err := bumpRevision(ctx, tx, catalogue)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.updated", eventType: "food.item.updated",
			aggregateType: "food_item", aggregateID: updated.ID, branchID: updated.BranchID,
			version: updated.Version, before: ItemViewOf(item, "", nil), after: ItemViewOf(updated, "", nil),
			payload: map[string]any{"item": ItemViewOf(updated, "", nil),
				"price_version_ref": stringPtrOrNil(priceVersionRef), "catalogue_revision": revision},
		})
	})
	return updated, err
}

// ArchiveItem retires an item and its variants.
//
// Nothing is deleted: an order, a dispute or an analytics record may already
// name this item, and those references must stay resolvable.
func (s *Service) ArchiveItem(ctx context.Context, actor Actor, scope Scope, itemID string, expectedVersion int64) (Item, error) {
	if err := s.requireWrites(); err != nil {
		return Item{}, err
	}
	var archived Item
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		if item.Version != expectedVersion {
			return errStaleVersion()
		}
		archived, err = scanItem(tx.QueryRow(ctx, `
			UPDATE food_items SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+itemCols,
			actor.VendorID, item.ID))
		if err != nil {
			return mapDB(err, "Food item", CodeItemNotFound)
		}
		if _, err = tx.Exec(ctx, `
			UPDATE food_item_variants SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND item_id=$2 AND status <> 'ARCHIVED'`, actor.VendorID, item.ID); err != nil {
			return err
		}
		if err = resequence(ctx, tx, variantsOrdering, item.ID); err != nil {
			return err
		}
		if err = resequence(ctx, tx, itemsOrdering, item.SectionID); err != nil {
			return err
		}
		revision, err := s.bumpForItem(ctx, tx, archived)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.archived", eventType: "food.item.archived",
			aggregateType: "food_item", aggregateID: archived.ID, branchID: archived.BranchID,
			version: archived.Version, before: ItemViewOf(item, "", nil), after: ItemViewOf(archived, "", nil),
			payload: map[string]any{"item": ItemViewOf(archived, "", nil), "catalogue_revision": revision},
		})
	})
	return archived, err
}

// ReorderItems applies a complete new ordering within one section.
func (s *Service) ReorderItems(ctx context.Context, actor Actor, scope Scope, menuID, sectionID string, orderedIDs []string) ([]Item, error) {
	if err := s.requireWrites(); err != nil {
		return nil, err
	}
	var items []Item
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
		if err = applyReorder(ctx, tx, itemsOrdering, section.ID, orderedIDs); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+itemCols+` FROM food_items
			WHERE business_id=$1 AND section_id=$2 AND status <> 'ARCHIVED' ORDER BY position, id`,
			actor.VendorID, section.ID)
		if err != nil {
			return err
		}
		items = []Item{}
		for rows.Next() {
			item, scanErr := scanItem(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			items = append(items, item)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		order := make([]string, 0, len(items))
		for _, item := range items {
			order = append(order, item.ID)
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.reordered", eventType: "food.item.reordered",
			aggregateType: "food_menu_section", aggregateID: section.ID, branchID: menu.BranchID,
			version: section.Version, after: order,
			payload: map[string]any{"section_id": section.ID, "ordered_item_ids": order},
		})
	})
	return items, err
}

// ---------------------------------------------------------------------------
// Item media
// ---------------------------------------------------------------------------

// MediaRefInput is one requested reference and its cover flag.
type MediaRefInput struct {
	MediaRef  string
	IsPrimary bool
}

// mediaForItems loads media for a page of items in one query, so a list
// endpoint never issues one query per item.
func (s *Service) mediaForItems(ctx context.Context, q querier, businessID string, itemIDs []string) (map[string][]ItemMedia, error) {
	out := map[string][]ItemMedia{}
	if len(itemIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `
		SELECT id,business_id,item_id,media_ref,position,is_primary,created_at
		FROM food_item_media WHERE business_id=$1 AND item_id = ANY($2)
		ORDER BY item_id, position, id`, businessID, itemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry ItemMedia
		if err = rows.Scan(&entry.ID, &entry.BusinessID, &entry.ItemID, &entry.MediaRef,
			&entry.Position, &entry.IsPrimary, &entry.CreatedAt); err != nil {
			return nil, err
		}
		out[entry.ItemID] = append(out[entry.ItemID], entry)
	}
	return out, rows.Err()
}

// ReplaceItemMedia sets the item's complete ordered media reference list.
//
// Replace rather than add/remove, because order and the cover designation are
// properties of the set: every write is then a transition between two valid
// states instead of a sequence that can pass through an invalid one.
//
// Vendor Service stores the opaque reference and nothing else. Ownership,
// completion and safety are proven against Media File Service's existing
// private API — the same contract Section 7 uses — and a service with no media
// authority configured refuses the write rather than storing an unverified
// reference.
func (s *Service) ReplaceItemMedia(ctx context.Context, actor Actor, scope Scope, itemID string, refs []MediaRefInput, expectedVersion int64) (Item, []ItemMedia, error) {
	if err := s.requireWrites(); err != nil {
		return Item{}, nil, err
	}
	if len(refs) > MaxMediaRefs {
		return Item{}, nil, validation(apperrors.FieldViolation{Field: "media", Message: "Too many media references were supplied."})
	}
	seen := map[string]bool{}
	primaries := 0
	cleaned := make([]MediaRefInput, 0, len(refs))
	for _, ref := range refs {
		value := strings.TrimSpace(ref.MediaRef)
		if !mediaRefPattern.MatchString(value) {
			return Item{}, nil, validation(apperrors.FieldViolation{Field: "media.media_ref", Message: "Media reference is not a valid identifier."})
		}
		if seen[value] {
			return Item{}, nil, validation(apperrors.FieldViolation{Field: "media.media_ref", Message: "The same media reference was supplied more than once."})
		}
		seen[value] = true
		if ref.IsPrimary {
			primaries++
		}
		cleaned = append(cleaned, MediaRefInput{MediaRef: value, IsPrimary: ref.IsPrimary})
	}
	if primaries > 1 {
		return Item{}, nil, validation(apperrors.FieldViolation{Field: "media.is_primary", Message: "Only one media reference can be the cover."})
	}
	if primaries == 0 && len(cleaned) > 0 {
		// A set with no declared cover gets the first entry, so the item always
		// has a deterministic cover rather than an arbitrary one.
		cleaned[0].IsPrimary = true
	}

	if len(cleaned) > 0 {
		if s.media == nil {
			return Item{}, nil, coded(apperrors.Unavailable("Media ownership validation is not configured.", nil), CodeMediaUnavailable)
		}
		for _, ref := range cleaned {
			object, err := s.media.GetObject(ctx, actor.VendorID, ref.MediaRef)
			if err != nil {
				return Item{}, nil, coded(apperrors.Unavailable("Media reference could not be validated.", err), CodeMediaUnavailable)
			}
			if object.UploadPurpose == "PRODUCT_IMAGE" && (object.OwnerService != "vendor-service" || object.OwnerReference != actor.VendorID || object.BindingType != "ASSET" || object.BindingID != "FOOD_ITEM:"+itemID || object.Visibility != "PRIVATE" || object.SafetyStatus != "SAFE") {
				return Item{}, nil, validation(apperrors.FieldViolation{Field: "media.media_ref", Message: "Choose a safe image uploaded for this food item."})
			}
			if object.ID != ref.MediaRef || object.UploadStatus != "COMPLETED" || object.DeletionState != "ACTIVE" ||
				object.SafetyStatus == "UNSAFE" || object.SafetyStatus == "QUARANTINED" {
				return Item{}, nil, validation(apperrors.FieldViolation{Field: "media.media_ref",
					Message: "Media object is not an active completed object owned by this business."})
			}
		}
	}

	var (
		item  Item
		media []ItemMedia
	)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("Food item")
		}
		if current.Version != expectedVersion {
			return errStaleVersion()
		}
		before, err := s.mediaForItems(ctx, tx, actor.VendorID, []string{current.ID})
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM food_item_media WHERE business_id=$1 AND item_id=$2`,
			actor.VendorID, current.ID); err != nil {
			return err
		}
		for index, ref := range cleaned {
			if _, err = tx.Exec(ctx, `
				INSERT INTO food_item_media (id,business_id,item_id,media_ref,position,is_primary)
				VALUES ($1,$2,$3,$4,$5,$6)`,
				newID("fim_"), actor.VendorID, current.ID, ref.MediaRef, index, ref.IsPrimary); err != nil {
				return mapDB(err, "Item media", CodeItemNotFound)
			}
		}
		// The item's own version moves, so a concurrent editor that loaded the
		// item before the media change cannot overwrite it blindly.
		item, err = scanItem(tx.QueryRow(ctx, `
			UPDATE food_items SET updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+itemCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Food item", CodeItemNotFound)
		}
		refreshed, err := s.mediaForItems(ctx, tx, actor.VendorID, []string{current.ID})
		if err != nil {
			return err
		}
		media = refreshed[current.ID]
		return s.record(ctx, tx, actor, evidence{
			action: "food.item.media_changed", eventType: "food.item.media_changed",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: item.Version,
			before:  mediaRefsOf(before[current.ID]), after: mediaRefsOf(media),
			payload: map[string]any{"item_id": item.ID, "media": mediaViewsOf(media)},
		})
	})
	return item, media, err
}

func mediaRefsOf(entries []ItemMedia) []string {
	refs := make([]string, 0, len(entries))
	for _, entry := range entries {
		refs = append(refs, entry.MediaRef)
	}
	return refs
}

func mediaViewsOf(entries []ItemMedia) []ItemMediaView {
	views := make([]ItemMediaView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, ItemMediaView{MediaRef: entry.MediaRef, Position: entry.Position, IsPrimary: entry.IsPrimary})
	}
	return views
}

// ListItemMedia returns the item's ordered references.
func (s *Service) ListItemMedia(ctx context.Context, actor Actor, scope Scope, itemID string) ([]ItemMedia, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return nil, err
	}
	if !scope.Covers(item.BranchID) {
		return nil, errNotFound("Food item", CodeItemNotFound)
	}
	media, err := s.mediaForItems(ctx, s.db, actor.VendorID, []string{item.ID})
	if err != nil {
		return nil, err
	}
	return media[item.ID], nil
}

// ---------------------------------------------------------------------------
// Variants
// ---------------------------------------------------------------------------

type CreateVariantInput struct {
	Name               string
	PriceOverrideMinor *int64
}

// CreateVariant adds a size/portion to an item.
func (s *Service) CreateVariant(ctx context.Context, actor Actor, scope Scope, itemID string, in CreateVariantInput) (Variant, error) {
	if err := s.requireWrites(); err != nil {
		return Variant{}, err
	}
	name, violation := validateName("name", in.Name, 1, 120)
	if violation != nil {
		return Variant{}, validation(*violation)
	}
	if in.PriceOverrideMinor != nil {
		if v := validatePriceMinor("price_override_minor", *in.PriceOverrideMinor); v != nil {
			return Variant{}, validation(*v)
		}
		// An explicit variant price is a price. A variant that simply inherits
		// the item's is not, and needs no pricing grant.
		if *in.PriceOverrideMinor > 0 {
			item, itemErr := s.scopedItem(ctx, actor, scope, itemID)
			if itemErr != nil {
				return Variant{}, itemErr
			}
			if err := s.requirePricingAuthority(ctx, actor, item.BranchID); err != nil {
				return Variant{}, err
			}
		}
	}

	var variant Variant
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		position, err := nextPosition(ctx, tx, variantsOrdering, item.ID)
		if err != nil {
			return err
		}
		variant, err = scanVariant(tx.QueryRow(ctx, `
			INSERT INTO food_item_variants (id,business_id,item_id,name,price_override_minor,position,status)
			VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE') RETURNING `+variantCols,
			newID("fiv_"), actor.VendorID, item.ID, name, in.PriceOverrideMinor, position))
		if err != nil {
			return mapDB(err, "Variant", CodeVariantNotFound)
		}
		if err = resequence(ctx, tx, variantsOrdering, item.ID); err != nil {
			return err
		}
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		catalogue, err := scopeOfItem(ctx, tx, item)
		if err != nil {
			return err
		}
		branch := item.BranchID
		itemRef := item.ID
		priceVersionRef, err := s.recordPrice(ctx, tx, actor, priceRecord{
			scope: catalogue, branchID: &branch, subjectType: PriceSubjectVariant,
			subjectRef: variant.ID, itemRef: &itemRef, currency: currency,
			previous: nil, amount: variant.PriceOverrideMinor,
			prevInherit: false, inherit: variant.PriceOverrideMinor == nil,
			changeReason: PriceChangeCreated, version: variant.Version,
		})
		if err != nil {
			return err
		}
		revision, err := bumpRevision(ctx, tx, catalogue)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.variant.created", eventType: "food.variant.created",
			aggregateType: "food_item_variant", aggregateID: variant.ID, branchID: item.BranchID,
			version: variant.Version, after: VariantViewOf(variant),
			payload: map[string]any{"variant": VariantViewOf(variant),
				"price_version_ref": priceVersionRef, "catalogue_revision": revision},
		})
	})
	return variant, err
}

// ListVariants returns an item's variants in deterministic order.
func (s *Service) ListVariants(ctx context.Context, actor Actor, scope Scope, itemID string, includeArchived bool) ([]Variant, error) {
	item, err := loadItem(ctx, s.db, actor.VendorID, itemID, false)
	if err != nil {
		return nil, err
	}
	if !scope.Covers(item.BranchID) {
		return nil, errNotFound("Food item", CodeItemNotFound)
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+variantCols+` FROM food_item_variants
		WHERE business_id=$1 AND item_id=$2 AND ($3 OR status <> 'ARCHIVED')
		ORDER BY (status = 'ARCHIVED'), position, id`,
		actor.VendorID, item.ID, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variants := []Variant{}
	for rows.Next() {
		variant, scanErr := scanVariant(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		variants = append(variants, variant)
	}
	return variants, rows.Err()
}

// UpdateVariantInput distinguishes "leave the override alone" from "clear it".
// Without ClearPriceOverride a nil pointer would be ambiguous, and a variant
// could never be returned to inheriting the item's base price.
type UpdateVariantInput struct {
	Name               *string
	PriceOverrideMinor *int64
	ClearPriceOverride bool
	ExpectedVersion    int64
}

// UpdateVariant edits a variant addressed through its own item.
func (s *Service) UpdateVariant(ctx context.Context, actor Actor, scope Scope, itemID, variantID string, in UpdateVariantInput) (Variant, error) {
	if err := s.requireWrites(); err != nil {
		return Variant{}, err
	}
	var updated Variant
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		variant, err := loadVariant(ctx, tx, actor.VendorID, item.ID, variantID, true)
		if err != nil {
			return err
		}
		if variant.Status == ChildArchived {
			return errArchived("Variant")
		}
		if variant.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		name := variant.Name
		if in.Name != nil {
			value, violation := validateName("name", *in.Name, 1, 120)
			if violation != nil {
				return validation(*violation)
			}
			name = value
		}
		override := variant.PriceOverrideMinor
		switch {
		case in.ClearPriceOverride:
			override = nil
		case in.PriceOverrideMinor != nil:
			if violation := validatePriceMinor("price_override_minor", *in.PriceOverrideMinor); violation != nil {
				return validation(*violation)
			}
			value := *in.PriceOverrideMinor
			override = &value
		}
		// Clearing an override back to inheritance changes what the variant
		// costs just as much as setting one does, so both need the grant.
		if priceChanged(variant.PriceOverrideMinor, override) {
			if err = s.requirePricingAuthority(ctx, actor, item.BranchID); err != nil {
				return err
			}
		}

		updated, err = scanVariant(tx.QueryRow(ctx, `
			UPDATE food_item_variants SET name=$3, price_override_minor=$4, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+variantCols,
			actor.VendorID, variant.ID, name, override))
		if err != nil {
			return mapDB(err, "Variant", CodeVariantNotFound)
		}

		catalogue, err := scopeOfItem(ctx, tx, item)
		if err != nil {
			return err
		}
		var priceVersionRef string
		if priceChanged(variant.PriceOverrideMinor, updated.PriceOverrideMinor) {
			currency, curErr := s.businessCurrency(ctx, tx, actor.VendorID)
			if curErr != nil {
				return curErr
			}
			reason := PriceChanged
			if updated.PriceOverrideMinor == nil {
				reason = PriceChangeOverrideCleared
			} else if variant.PriceOverrideMinor == nil {
				reason = PriceChangeOverrideSet
			}
			branch := item.BranchID
			itemRef := item.ID
			priceVersionRef, err = s.recordPrice(ctx, tx, actor, priceRecord{
				scope: catalogue, branchID: &branch, subjectType: PriceSubjectVariant,
				subjectRef: updated.ID, itemRef: &itemRef, currency: currency,
				previous: variant.PriceOverrideMinor, amount: updated.PriceOverrideMinor,
				prevInherit:  variant.PriceOverrideMinor == nil,
				inherit:      updated.PriceOverrideMinor == nil,
				changeReason: reason, version: updated.Version,
			})
			if err != nil {
				return err
			}
		}
		revision, err := bumpRevision(ctx, tx, catalogue)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.variant.updated", eventType: "food.variant.updated",
			aggregateType: "food_item_variant", aggregateID: updated.ID, branchID: item.BranchID,
			version: updated.Version, before: VariantViewOf(variant), after: VariantViewOf(updated),
			payload: map[string]any{"variant": VariantViewOf(updated),
				"price_version_ref": stringPtrOrNil(priceVersionRef), "catalogue_revision": revision},
		})
	})
	return updated, err
}

// ArchiveVariant retires a variant, keeping it resolvable for anything that
// already references it.
func (s *Service) ArchiveVariant(ctx context.Context, actor Actor, scope Scope, itemID, variantID string, expectedVersion int64) (Variant, error) {
	if err := s.requireWrites(); err != nil {
		return Variant{}, err
	}
	var archived Variant
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		variant, err := loadVariant(ctx, tx, actor.VendorID, item.ID, variantID, true)
		if err != nil {
			return err
		}
		if variant.Status == ChildArchived {
			return errArchived("Variant")
		}
		if variant.Version != expectedVersion {
			return errStaleVersion()
		}
		archived, err = scanVariant(tx.QueryRow(ctx, `
			UPDATE food_item_variants SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+variantCols,
			actor.VendorID, variant.ID))
		if err != nil {
			return mapDB(err, "Variant", CodeVariantNotFound)
		}
		if err = resequence(ctx, tx, variantsOrdering, item.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.variant.archived", eventType: "food.variant.archived",
			aggregateType: "food_item_variant", aggregateID: archived.ID, branchID: item.BranchID,
			version: archived.Version, before: VariantViewOf(variant), after: VariantViewOf(archived),
			payload: VariantViewOf(archived),
		})
	})
	return archived, err
}

// ReorderVariants applies a complete new ordering within one item.
func (s *Service) ReorderVariants(ctx context.Context, actor Actor, scope Scope, itemID string, orderedIDs []string) ([]Variant, error) {
	if err := s.requireWrites(); err != nil {
		return nil, err
	}
	var variants []Variant
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := s.lockedItem(ctx, tx, actor, scope, itemID)
		if err != nil {
			return err
		}
		if item.Status == StatusArchived {
			return errArchived("Food item")
		}
		if err = applyReorder(ctx, tx, variantsOrdering, item.ID, orderedIDs); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+variantCols+` FROM food_item_variants
			WHERE business_id=$1 AND item_id=$2 AND status <> 'ARCHIVED' ORDER BY position, id`,
			actor.VendorID, item.ID)
		if err != nil {
			return err
		}
		variants = []Variant{}
		for rows.Next() {
			variant, scanErr := scanVariant(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			variants = append(variants, variant)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		order := make([]string, 0, len(variants))
		for _, variant := range variants {
			order = append(order, variant.ID)
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.variant.reordered", eventType: "food.variant.reordered",
			aggregateType: "food_item", aggregateID: item.ID, branchID: item.BranchID,
			version: item.Version, after: order,
			payload: map[string]any{"item_id": item.ID, "ordered_variant_ids": order},
		})
	})
	return variants, err
}
