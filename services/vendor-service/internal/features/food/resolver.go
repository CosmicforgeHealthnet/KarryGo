package food

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Effective price
// ---------------------------------------------------------------------------
//
// Catalogue price composition, and only that. It is not checkout, not a quote
// and not settlement: Wallet remains the monetary authority and no route here
// moves money.
//
// Every value is loaded from the database. The client supplies references and
// context â€” which item, which variant, which modifiers â€” and this service
// supplies the numbers. A caller-supplied price is never trusted, because that
// is how a client ends up choosing what it pays.

// PriceComponent is one selected modifier's contribution.
type PriceComponent struct {
	ModifierRef      string `json:"modifier_ref"`
	ModifierGroupRef string `json:"modifier_group_ref"`
	Name             string `json:"name"`
	PriceDeltaMinor  int64  `json:"price_delta_minor"`
	ModifierVersion  int64  `json:"modifier_version"`
	PriceVersionRef  string `json:"price_version_ref,omitempty"`
}

// Base-price sources, in the precedence the resolver applies.
const (
	BaseSourceItem     = "ITEM"
	BaseSourceBranch   = "BRANCH_OVERRIDE"
	BaseSourceVariant  = "VARIANT_OVERRIDE"
	baseSourceInherits = "VARIANT_INHERITS"
)

// ResolvedPrice is the structured breakdown, not merely a number. A future
// Order authority snapshots these references to prove what the catalogue said
// at the moment of the order; a single total could not.
type ResolvedPrice struct {
	Currency string `json:"currency"`

	ItemRef     string `json:"item_ref"`
	ItemVersion int64  `json:"item_version"`
	VariantRef  string `json:"variant_ref,omitempty"`

	VariantVersion int64 `json:"variant_version,omitempty"`

	BaseSource     string `json:"base_source"`
	BasePriceMinor int64  `json:"base_price_minor"`

	Modifiers          []PriceComponent `json:"modifiers"`
	ModifierTotalMinor int64            `json:"modifier_total_minor"`

	EffectiveUnitPriceMinor int64 `json:"effective_unit_price_minor"`

	ItemPriceVersionRef           string `json:"item_price_version_ref,omitempty"`
	VariantPriceVersionRef        string `json:"variant_price_version_ref,omitempty"`
	BranchOverridePriceVersionRef string `json:"branch_override_price_version_ref,omitempty"`
	BranchOverrideRef             string `json:"branch_override_ref,omitempty"`
	BranchOverrideVersion         int64  `json:"branch_override_version,omitempty"`

	CatalogueRevision int64     `json:"catalogue_revision"`
	ResolvedAt        time.Time `json:"resolved_at"`
}

// ResolvePriceInput is what the caller selects. Nothing in it is a price.
type ResolvePriceInput struct {
	VariantRef   string
	ModifierRefs []string
}

// latestPriceVersionRef finds the newest immutable price record for a subject,
// which is the reference a snapshot would carry.
func latestPriceVersionRef(ctx context.Context, q querier, businessID, subjectType, subjectRef string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT id FROM food_price_versions
		WHERE business_id=$1 AND subject_type=$2 AND subject_ref=$3
		ORDER BY revision DESC LIMIT 1`, businessID, subjectType, subjectRef).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ResolvePrice composes the authoritative catalogue price.
//
// Precedence, deterministic and tested: an explicit variant price is the base;
// otherwise a branch base-price override is the base; otherwise the item's own
// base price. A variant with no price of its own inherits whatever the branch
// layer resolved â€” but a variant WITH an explicit price does not have the
// branch override added on top of it, because an absolute price is absolute.
func (s *Service) ResolvePrice(ctx context.Context, actor Actor, scope Scope, itemID string, in ResolvePriceInput) (ResolvedPrice, error) {
	item, err := s.scopedItem(ctx, actor, scope, itemID)
	if err != nil {
		return ResolvedPrice{}, err
	}
	currency, err := s.businessCurrency(ctx, s.db, actor.VendorID)
	if err != nil {
		return ResolvedPrice{}, err
	}
	override, err := loadOverride(ctx, s.db, item, false)
	if err != nil {
		return ResolvedPrice{}, err
	}

	resolved := ResolvedPrice{
		Currency: currency, ItemRef: item.ID, ItemVersion: item.Version,
		Modifiers: []PriceComponent{}, ResolvedAt: s.now(),
	}

	// --- base -------------------------------------------------------------
	switch {
	case override.BasePriceOverrideMinor != nil:
		resolved.BaseSource = BaseSourceBranch
		resolved.BasePriceMinor = *override.BasePriceOverrideMinor
		resolved.BranchOverrideRef = override.ID
		resolved.BranchOverrideVersion = override.Version
	default:
		resolved.BaseSource = BaseSourceItem
		resolved.BasePriceMinor = item.BasePriceMinor
	}

	variantRef := strings.TrimSpace(in.VariantRef)
	if variantRef != "" {
		variant, variantErr := loadVariant(ctx, s.db, actor.VendorID, item.ID, variantRef, false)
		if variantErr != nil {
			return ResolvedPrice{}, variantErr
		}
		if variant.Status == ChildArchived {
			return ResolvedPrice{}, errArchived("Variant")
		}
		resolved.VariantRef = variant.ID
		resolved.VariantVersion = variant.Version
		if variant.PriceOverrideMinor != nil {
			// An explicit variant price is the whole base. The branch override
			// is NOT added to it: "large pizza costs 7,000" means 7,000, not
			// 7,000 plus whatever this branch charges for a pizza.
			resolved.BaseSource = BaseSourceVariant
			resolved.BasePriceMinor = *variant.PriceOverrideMinor
			resolved.BranchOverrideRef = ""
			resolved.BranchOverrideVersion = 0
		}
		ref, refErr := latestPriceVersionRef(ctx, s.db, actor.VendorID, PriceSubjectVariant, variant.ID)
		if refErr != nil {
			return ResolvedPrice{}, refErr
		}
		resolved.VariantPriceVersionRef = ref
	}

	// --- modifiers --------------------------------------------------------
	components, total, err := s.resolveModifiers(ctx, actor, item, in.ModifierRefs)
	if err != nil {
		return ResolvedPrice{}, err
	}
	resolved.Modifiers = components
	resolved.ModifierTotalMinor = total

	unit, err := addMinor(resolved.BasePriceMinor, total)
	if err != nil {
		return ResolvedPrice{}, err
	}
	resolved.EffectiveUnitPriceMinor = unit

	// --- immutable references --------------------------------------------
	if resolved.ItemPriceVersionRef, err = latestPriceVersionRef(ctx, s.db, actor.VendorID,
		PriceSubjectItem, item.ID); err != nil {
		return ResolvedPrice{}, err
	}
	if resolved.BranchOverrideRef != "" {
		if resolved.BranchOverridePriceVersionRef, err = latestPriceVersionRef(ctx, s.db, actor.VendorID,
			PriceSubjectBranchOverride, resolved.BranchOverrideRef); err != nil {
			return ResolvedPrice{}, err
		}
	}
	catalogue, err := scopeOfItem(ctx, s.db, item)
	if err != nil {
		return ResolvedPrice{}, err
	}
	if resolved.CatalogueRevision, err = s.CatalogueRevision(ctx, s.db, catalogue); err != nil {
		return ResolvedPrice{}, err
	}
	return resolved, nil
}

// resolveModifiers validates a selection and prices it.
//
// One set-based query loads every selected modifier together with the
// attachment that authorises it, so selecting twenty modifiers costs one query
// rather than twenty. Anything the query does not return â€” a foreign modifier,
// one from a group that is not attached, one belonging to another business â€”
// is refused with the same message, because a price preview must not become an
// oracle for what exists elsewhere.
func (s *Service) resolveModifiers(ctx context.Context, actor Actor, item Item, refs []string) ([]PriceComponent, int64, error) {
	components := []PriceComponent{}
	if len(refs) == 0 {
		if err := s.requireSelectionMinimums(ctx, actor, item, map[string]int{}); err != nil {
			return nil, 0, err
		}
		return components, 0, nil
	}
	if len(refs) > 50 {
		return nil, 0, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "modifier_refs", Message: "Too many modifiers were selected."}}), CodeInvalidModifierSelect)
	}
	cleaned := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			return nil, 0, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "modifier_refs", Message: "Modifier references must not be blank."}}),
				CodeInvalidModifierSelect)
		}
		if seen[ref] {
			return nil, 0, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "modifier_refs", Message: "The same modifier was selected more than once."}}),
				CodeInvalidModifierSelect)
		}
		seen[ref] = true
		cleaned = append(cleaned, ref)
	}

	rows, err := s.db.Query(ctx, `
		SELECT md.id, md.modifier_group_id, md.name, md.price_delta_minor, md.version,
			COALESCE((SELECT pv.id FROM food_price_versions pv
				WHERE pv.business_id = md.business_id AND pv.subject_type = 'MODIFIER'
				  AND pv.subject_ref = md.id ORDER BY pv.revision DESC LIMIT 1), '')
		FROM food_modifiers md
		JOIN food_modifier_groups g
		  ON g.id = md.modifier_group_id AND g.business_id = md.business_id
		JOIN food_item_modifier_groups link
		  ON link.modifier_group_id = md.modifier_group_id AND link.business_id = md.business_id
		WHERE md.business_id = $1 AND link.item_id = $2 AND md.id = ANY($3)
		  AND md.status <> 'ARCHIVED' AND g.status <> 'ARCHIVED'
		ORDER BY link.position, md.position, md.id`,
		item.BusinessID, item.ID, cleaned)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	perGroup := map[string]int{}
	var total int64
	found := 0
	for rows.Next() {
		var component PriceComponent
		if err = rows.Scan(&component.ModifierRef, &component.ModifierGroupRef, &component.Name,
			&component.PriceDeltaMinor, &component.ModifierVersion, &component.PriceVersionRef); err != nil {
			return nil, 0, err
		}
		total, err = addMinor(total, component.PriceDeltaMinor)
		if err != nil {
			return nil, 0, err
		}
		perGroup[component.ModifierGroupRef]++
		components = append(components, component)
		found++
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	if found != len(cleaned) {
		// One message for every rejection: foreign modifier, unattached group,
		// archived either end, or another tenant's id.
		return nil, 0, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "modifier_refs",
				Message: "One or more modifiers are not available for this item."}}),
			CodeInvalidModifierSelect)
	}
	if err = s.requireSelectionMinimums(ctx, actor, item, perGroup); err != nil {
		return nil, 0, err
	}
	return components, total, nil
}

// requireSelectionMinimums enforces the attached groups' min/max rules.
//
// A configured selection that breaks them is not priced at all. Returning a
// number for an unorderable configuration would be worse than refusing: it
// would look like a quote.
func (s *Service) requireSelectionMinimums(ctx context.Context, actor Actor, item Item, perGroup map[string]int) error {
	rows, err := s.db.Query(ctx, `
		SELECT g.id, g.name, g.min_select, g.max_select
		FROM food_item_modifier_groups link
		JOIN food_modifier_groups g ON g.id = link.modifier_group_id AND g.business_id = link.business_id
		WHERE link.business_id=$1 AND link.item_id=$2 AND g.status <> 'ARCHIVED'
		ORDER BY link.position`, item.BusinessID, item.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var violations []apperrors.FieldViolation
	for rows.Next() {
		var groupID, name string
		var minSelect, maxSelect int
		if err = rows.Scan(&groupID, &name, &minSelect, &maxSelect); err != nil {
			return err
		}
		selected := perGroup[groupID]
		if selected < minSelect {
			violations = append(violations, apperrors.FieldViolation{Field: "modifier_refs",
				Message: "\"" + name + "\" requires at least " + itoa(minSelect) + " selection(s)."})
		}
		if selected > maxSelect {
			violations = append(violations, apperrors.FieldViolation{Field: "modifier_refs",
				Message: "\"" + name + "\" allows at most " + itoa(maxSelect) + " selection(s)."})
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(violations) > 0 {
		return coded(apperrors.Validation("Check your details.", violations), CodeInvalidModifierSelect)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Effective availability
// ---------------------------------------------------------------------------

// Sellability is the explainable answer to "can branch X sell item Y at time T".
type Sellability struct {
	Sellable    bool      `json:"sellable"`
	ReasonCode  string    `json:"reason_code"`
	Detail      string    `json:"detail,omitempty"`
	EvaluatedAt time.Time `json:"evaluated_at"`

	BranchID   string `json:"branch_id"`
	Timezone   string `json:"timezone"`
	MenuRef    string `json:"menu_ref"`
	SectionRef string `json:"section_ref"`
	ItemRef    string `json:"item_ref"`

	MatchedScheduleRef string `json:"matched_schedule_ref,omitempty"`

	OperationalStatus   string `json:"operational_status"`
	AvailabilityVersion int64  `json:"availability_version"`

	AvailabilityMode             string `json:"availability_mode"`
	InventoryAvailabilitySupport string `json:"inventory_availability_support"`

	CatalogueRevision int64 `json:"catalogue_revision"`
}

// sellabilityGraph is the single joined row every layer of the decision needs.
type sellabilityGraph struct {
	businessStatus   string
	branchStatus     string
	branchTimezone   string
	capabilityStatus *string
	capability       string
	menuStatus       string
	sectionStatus    string
}

// ResolveSellability answers the central Section 9B question, and fails closed.
//
// The layers are evaluated in one fixed order â€” business, branch, capability,
// menu, schedule, section, item, operational override, inventory â€” and a lower
// layer can only ever deny. Nothing below can resurrect a denial above it: an
// AVAILABLE override does not un-archive an item, a matching schedule does not
// reopen a closed branch, and an inventory source saying yes does not activate
// a paused menu.
func (s *Service) ResolveSellability(ctx context.Context, actor Actor, scope Scope, itemID string, at time.Time) (Sellability, error) {
	item, err := s.scopedItem(ctx, actor, scope, itemID)
	if err != nil {
		return Sellability{}, err
	}
	if at.IsZero() {
		at = s.now()
	}

	// One query for the whole parent graph, so the resolver never walks the
	// hierarchy a row at a time.
	var graph sellabilityGraph
	err = s.db.QueryRow(ctx, `
		SELECT biz.status, br.status, br.timezone, bc.status, menu.capability_type,
			menu.status, sec.status
		FROM food_items i
		JOIN food_menus menu ON menu.id = i.menu_id AND menu.business_id = i.business_id
		JOIN food_menu_sections sec ON sec.id = i.section_id AND sec.menu_id = menu.id
		JOIN vendor_branches br ON br.id = i.branch_id AND br.business_id = i.business_id
		JOIN vendor_businesses biz ON biz.id = i.business_id
		LEFT JOIN vendor_branch_capabilities bc
		  ON bc.business_id = i.business_id AND bc.branch_id = i.branch_id
		 AND bc.capability_type = menu.capability_type
		WHERE i.business_id=$1 AND i.id=$2`, item.BusinessID, item.ID).
		Scan(&graph.businessStatus, &graph.branchStatus, &graph.branchTimezone,
			&graph.capabilityStatus, &graph.capability, &graph.menuStatus, &graph.sectionStatus)
	if err != nil {
		return Sellability{}, mapDB(err, "Food item", CodeItemNotFound)
	}

	availability, err := loadAvailability(ctx, s.db, item, false)
	if err != nil {
		return Sellability{}, err
	}
	source, err := loadSource(ctx, s.db, item, false)
	if err != nil {
		return Sellability{}, err
	}
	catalogue := catalogueScope{BusinessID: item.BusinessID, BranchID: item.BranchID,
		Capability: graph.capability}
	revision, err := s.CatalogueRevision(ctx, s.db, catalogue)
	if err != nil {
		return Sellability{}, err
	}

	result := Sellability{
		EvaluatedAt: at, BranchID: item.BranchID, Timezone: graph.branchTimezone,
		MenuRef: item.MenuID, SectionRef: item.SectionID, ItemRef: item.ID,
		OperationalStatus: availability.Effective(at), AvailabilityVersion: availability.Version,
		AvailabilityMode: source.Mode, InventoryAvailabilitySupport: InventorySupportNotConfigured,
		CatalogueRevision: revision,
	}

	deny := func(code, detail string) (Sellability, error) {
		result.Sellable = false
		result.ReasonCode = code
		result.Detail = detail
		return result, nil
	}

	// 1-2. Business and branch must be operating at all.
	if graph.businessStatus != "ACTIVE" {
		return deny(ReasonBusinessInactive, "The vendor business is not active.")
	}
	if graph.branchStatus != "ACTIVE" {
		return deny(ReasonBranchInactive, "The branch is not active.")
	}
	// 3-4. The branch must hold this menu's food capability, and hold it live.
	if graph.capabilityStatus == nil {
		return deny(ReasonCapabilityInactive, "The branch does not have the "+graph.capability+" capability.")
	}
	if *graph.capabilityStatus != "ACTIVE" {
		return deny(ReasonCapabilityInactive, "The branch capability is not active.")
	}
	// 5. Administrative menu state. A schedule cannot reopen a paused menu.
	if graph.menuStatus != StatusActive {
		return deny(ReasonMenuNotActive, "The menu is not active.")
	}
	// 6. Schedule, evaluated in the branch's own timezone.
	menu, err := loadMenu(ctx, s.db, item.BusinessID, item.MenuID, false)
	if err != nil {
		return Sellability{}, err
	}
	decision, err := s.evaluateSchedules(ctx, s.db, menu, at)
	if err != nil {
		return Sellability{}, err
	}
	result.Timezone = decision.Timezone
	result.MatchedScheduleRef = decision.MatchRef
	if !decision.Allowed {
		return deny(ReasonOutsideSchedule, "The menu is outside its configured availability windows.")
	}
	// 7-8. Section and item administrative state.
	if graph.sectionStatus == ChildArchived {
		return deny(ReasonSectionInactive, "The menu section has been archived.")
	}
	if item.Status != StatusActive {
		return deny(ReasonItemNotActive, "The item is not active.")
	}
	// 9. Operational override â€” the only layer the kitchen controls, and the
	// only one that can deny an otherwise-sellable item.
	switch availability.Effective(at) {
	case OperationalSoldOut:
		return deny(ReasonItemSoldOut, availabilityDetail(availability, "The item is sold out."))
	case OperationalPaused:
		return deny(ReasonItemPaused, availabilityDetail(availability, "The item is temporarily paused."))
	}
	// 10. Inventory seam.
	//
	// MANUAL is deliberately independent of inventory.
	//
	// INVENTORY_LINKED delegates to the generic Inventory authority. If the
	// future recipe authority is absent, unresolved or unsupported, the answer
	// remains fail-closed.
	if source.Mode == SourceInventoryLinked {
		decision := s.resolveInventoryAvailability(
			ctx,
			item,
			source,
		)

		result.InventoryAvailabilitySupport =
			decision.Support

		if !decision.Sellable {
			switch decision.Support {
			case InventorySupportNotConfigured,
				InventorySupportNotSupported,
				InventorySupportUnknown:

				return deny(
					ReasonInventorySourceGone,
					"Inventory-linked availability cannot currently be resolved authoritatively.",
				)

			default:
				return deny(
					ReasonInventoryEmpty,
					"Inventory reports this item as unavailable.",
				)
			}
		}
	}

	result.Sellable = true
	result.ReasonCode = ReasonSellable
	return result, nil
}

func availabilityDetail(a ItemAvailability, fallback string) string {
	if a.Reason != "" {
		return a.Reason
	}
	return fallback
}

// ResolveOperationalProfile bundles the operational answer a management client
// needs about one item at one instant.
//
// It deliberately stops at preparation minutes and handling metadata. There is
// no combined delivery estimate anywhere in this type, because travel and
// dispatch are outside this service's authority and a number that mixed them
// would be a promise Vendor Service cannot keep.
type OperationalProfile struct {
	Sellability Sellability           `json:"sellability"`
	Prep        ResolvedPrepProfile   `json:"prep"`
	Logistics   *LogisticsProfileView `json:"logistics,omitempty"`
}

// ResolveOperationalProfile answers sellability, preparation and handling in
// one read.
func (s *Service) ResolveOperationalProfile(ctx context.Context, actor Actor, scope Scope, itemID string, at time.Time) (OperationalProfile, error) {
	sellability, err := s.ResolveSellability(ctx, actor, scope, itemID, at)
	if err != nil {
		return OperationalProfile{}, err
	}
	item, err := s.scopedItem(ctx, actor, scope, itemID)
	if err != nil {
		return OperationalProfile{}, err
	}
	prep, err := s.ResolvePrepProfile(ctx, s.db, item)
	if err != nil {
		return OperationalProfile{}, err
	}
	profile := OperationalProfile{Sellability: sellability, Prep: prep}
	logistics, found, err := loadLogistics(ctx, s.db, item, false)
	if err != nil {
		return OperationalProfile{}, err
	}
	if found {
		view := LogisticsProfileViewOf(logistics)
		profile.Logistics = &view
	}
	return profile, nil
}
