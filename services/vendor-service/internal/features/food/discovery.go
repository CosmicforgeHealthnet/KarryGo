package food

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	DiscoveryMaxPage       = 100
	DiscoveryMaxSnapshot   = 200
	DiscoveryMaxSearchText = 120
)

const discoveryCatalogueCols = `m.id,m.business_id,m.branch_id,m.capability_type,m.name,m.description,m.status,m.created_at,m.updated_at,m.archived_at,m.version,
	sec.id,sec.business_id,sec.menu_id,sec.name,sec.description,sec.position,sec.status,sec.created_at,sec.updated_at,sec.archived_at,sec.version,
	i.id,i.business_id,i.branch_id,i.menu_id,i.section_id,i.name,i.description,i.base_price_minor,i.dietary_codes,i.allergen_codes,i.status,i.position,i.created_at,i.updated_at,i.archived_at,i.version`

type DiscoveryOptions struct {
	BranchID        string
	Capability      string
	MenuID          string
	SectionID       string
	Text            string
	DietaryCode     string
	ExcludeAllergen string
	MinPriceMinor   *int64
	MaxPriceMinor   *int64
	PrepMaxMinutes  *int
	SellableOnly    bool
	Limit           int
	Offset          int
	At              time.Time
}

type DiscoverySnapshot struct {
	BusinessRef       string          `json:"business_ref"`
	BranchRef         string          `json:"branch_ref"`
	Capability        string          `json:"capability"`
	CatalogueRevision int64           `json:"catalogue_revision"`
	GeneratedAt       time.Time       `json:"generated_at"`
	Menus             []DiscoveryMenu `json:"menus"`
	Pagination        DiscoveryPage   `json:"pagination"`
	Unchanged         bool            `json:"unchanged,omitempty"`
}

type DiscoveryPage struct {
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

type DiscoveryMenu struct {
	Ref      string             `json:"ref"`
	Name     string             `json:"name"`
	Sections []DiscoverySection `json:"sections"`
}

type DiscoverySection struct {
	Ref      string          `json:"ref"`
	Name     string          `json:"name"`
	Position int             `json:"position"`
	Items    []DiscoveryItem `json:"items"`
}

type DiscoveryItem struct {
	Ref            string                   `json:"ref"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	MediaRefs      []string                 `json:"media_refs"`
	DietaryCodes   []string                 `json:"dietary_codes"`
	AllergenCodes  []string                 `json:"allergen_codes"`
	Price          *ResolvedPrice           `json:"price,omitempty"`
	Variants       []VariantView            `json:"variants"`
	ModifierGroups []DiscoveryModifierGroup `json:"modifier_groups"`
	Sellability    Sellability              `json:"sellability"`
	Prep           ResolvedPrepProfile      `json:"prep"`
	Logistics      *LogisticsProfileView    `json:"logistics,omitempty"`
}

type DiscoveryModifierGroup struct {
	Group     ModifierGroupView `json:"group"`
	Modifiers []ModifierView    `json:"modifiers"`
}

type DiscoveryRevision struct {
	BusinessRef       string    `json:"business_ref"`
	BranchRef         string    `json:"branch_ref"`
	Capability        string    `json:"capability"`
	CatalogueRevision int64     `json:"catalogue_revision"`
	UpdatedAt         time.Time `json:"updated_at"`
	Unchanged         bool      `json:"unchanged,omitempty"`
}

type discoveryItemRow struct {
	item    Item
	menu    Menu
	section Section
}

func validateDiscoveryOptions(in DiscoveryOptions) (DiscoveryOptions, error) {
	in.BranchID = strings.TrimSpace(in.BranchID)
	in.Capability = strings.ToUpper(strings.TrimSpace(in.Capability))
	in.MenuID = strings.TrimSpace(in.MenuID)
	in.SectionID = strings.TrimSpace(in.SectionID)
	in.Text = strings.TrimSpace(in.Text)
	in.DietaryCode = strings.ToUpper(strings.TrimSpace(in.DietaryCode))
	in.ExcludeAllergen = strings.ToUpper(strings.TrimSpace(in.ExcludeAllergen))
	if in.BranchID == "" || !IsFoodCapability(in.Capability) {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "capability", Message: "A food capability is required."}})
	}
	if len(in.Text) > DiscoveryMaxSearchText {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "q", Message: "Search text is too long."}})
	}
	if in.Limit <= 0 || in.Limit > DiscoveryMaxPage {
		in.Limit = DiscoveryMaxPage
	}
	if in.Offset < 0 {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "offset", Message: "Offset must be zero or greater."}})
	}
	if in.MinPriceMinor != nil && *in.MinPriceMinor < 0 {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "min_price_minor", Message: "Price must not be negative."}})
	}
	if in.MaxPriceMinor != nil && *in.MaxPriceMinor < 0 {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "max_price_minor", Message: "Price must not be negative."}})
	}
	if in.MinPriceMinor != nil && in.MaxPriceMinor != nil && *in.MinPriceMinor > *in.MaxPriceMinor {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "max_price_minor", Message: "Maximum price must be at least the minimum."}})
	}
	if in.PrepMaxMinutes != nil && (*in.PrepMaxMinutes < 0 || *in.PrepMaxMinutes > MaxPrepMinutes) {
		return DiscoveryOptions{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "prep_max_minutes", Message: "Preparation bound is outside the supported range."}})
	}
	if in.At.IsZero() {
		in.At = time.Now().UTC()
	}
	return in, nil
}

func (s *Service) branchForDiscovery(ctx context.Context, branchID, capability string) (string, error) {
	var businessID, status string
	err := s.db.QueryRow(ctx, `SELECT business_id,status FROM vendor_branches WHERE id=$1`, branchID).Scan(&businessID, &status)
	if err != nil || status != "ACTIVE" {
		return "", errNotFound("Branch", CodeMenuNotFound)
	}
	var capabilityStatus string
	err = s.db.QueryRow(ctx, `SELECT status FROM vendor_branch_capabilities WHERE business_id=$1 AND branch_id=$2 AND capability_type=$3`, businessID, branchID, capability).Scan(&capabilityStatus)
	if err != nil || capabilityStatus != "ACTIVE" {
		return "", errNotFound("Branch", CodeMenuNotFound)
	}
	return businessID, nil
}

func (s *Service) discoveryItems(ctx context.Context, businessID string, in DiscoveryOptions) ([]discoveryItemRow, bool, error) {
	args := []any{businessID, in.BranchID, in.Capability}
	where := `m.business_id=$1 AND m.branch_id=$2 AND m.capability_type=$3 AND m.status='ACTIVE' AND sec.status='ACTIVE' AND i.status='ACTIVE'`
	if in.MenuID != "" {
		args = append(args, in.MenuID)
		where += ` AND m.id=$` + itoa(len(args))
	}
	if in.SectionID != "" {
		args = append(args, in.SectionID)
		where += ` AND sec.id=$` + itoa(len(args))
	}
	if in.Text != "" {
		args = append(args, "%"+strings.ToLower(in.Text)+"%")
		where += ` AND (lower(i.name) LIKE $` + itoa(len(args)) + ` OR lower(i.description) LIKE $` + itoa(len(args)) + `)`
	}
	if in.DietaryCode != "" {
		args = append(args, in.DietaryCode)
		where += ` AND i.dietary_codes @> jsonb_build_array($` + itoa(len(args)) + `::text)`
	}
	if in.ExcludeAllergen != "" {
		args = append(args, in.ExcludeAllergen)
		where += ` AND NOT (i.allergen_codes @> jsonb_build_array($` + itoa(len(args)) + `::text))`
	}
	rows, err := s.db.Query(ctx, `SELECT `+discoveryCatalogueCols+` FROM food_menus m JOIN food_menu_sections sec ON sec.business_id=m.business_id AND sec.menu_id=m.id JOIN food_items i ON i.business_id=sec.business_id AND i.menu_id=sec.menu_id AND i.section_id=sec.id WHERE `+where+` ORDER BY m.id,sec.position,i.position,i.id LIMIT $`+itoa(len(args)+1)+` OFFSET $`+itoa(len(args)+2), append(args, in.Limit+1, in.Offset)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]discoveryItemRow, 0, in.Limit)
	for rows.Next() {
		var menu Menu
		var section Section
		var item Item
		var menuArchived, sectionArchived, itemArchived *time.Time
		var dietary, allergens []byte
		if err = rows.Scan(&menu.ID, &menu.BusinessID, &menu.BranchID, &menu.CapabilityType, &menu.Name, &menu.Description, &menu.Status, &menu.CreatedAt, &menu.UpdatedAt, &menuArchived, &menu.Version,
			&section.ID, &section.BusinessID, &section.MenuID, &section.Name, &section.Description, &section.Position, &section.Status, &section.CreatedAt, &section.UpdatedAt, &sectionArchived, &section.Version,
			&item.ID, &item.BusinessID, &item.BranchID, &item.MenuID, &item.SectionID, &item.Name, &item.Description, &item.BasePriceMinor, &dietary, &allergens, &item.Status, &item.Position, &item.CreatedAt, &item.UpdatedAt, &itemArchived, &item.Version); err != nil {
			return nil, false, err
		}
		if err = json.Unmarshal(dietary, &item.DietaryCodes); err != nil {
			return nil, false, err
		}
		if err = json.Unmarshal(allergens, &item.AllergenCodes); err != nil {
			return nil, false, err
		}
		items = append(items, discoveryItemRow{item: item, menu: menu, section: section})
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > in.Limit
	if hasMore {
		items = items[:in.Limit]
	}
	return items, hasMore, nil
}

func (s *Service) DiscoverCatalogue(ctx context.Context, in DiscoveryOptions) (DiscoverySnapshot, error) {
	in, err := validateDiscoveryOptions(in)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	businessID, err := s.branchForDiscovery(ctx, in.BranchID, in.Capability)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	var revision int64
	if err = s.db.QueryRow(ctx, `SELECT COALESCE(revision,0) FROM food_catalogue_revisions WHERE business_id=$1 AND branch_id=$2 AND capability_type=$3`, businessID, in.BranchID, in.Capability).Scan(&revision); err != nil && err != pgx.ErrNoRows {
		return DiscoverySnapshot{}, err
	}
	items, hasMore, err := s.discoveryItems(ctx, businessID, in)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	itemIDs := make([]string, 0, len(items))
	for _, row := range items {
		itemIDs = append(itemIDs, row.item.ID)
	}
	variantsByItem, err := s.discoveryVariantsBatch(ctx, itemIDs, businessID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	groupsByItem, err := s.discoveryModifierGroupsBatch(ctx, itemIDs, businessID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	mediaByItem, err := s.discoveryMediaBatch(ctx, itemIDs, businessID)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	menus := make([]DiscoveryMenu, 0)
	menuIndex := map[string]int{}
	for _, row := range items {
		idx, ok := menuIndex[row.menu.ID]
		if !ok {
			idx = len(menus)
			menuIndex[row.menu.ID] = idx
			menus = append(menus, DiscoveryMenu{Ref: row.menu.ID, Name: row.menu.Name, Sections: []DiscoverySection{}})
		}
		sectionIndex := -1
		for i := range menus[idx].Sections {
			if menus[idx].Sections[i].Ref == row.section.ID {
				sectionIndex = i
				break
			}
		}
		if sectionIndex < 0 {
			menus[idx].Sections = append(menus[idx].Sections, DiscoverySection{Ref: row.section.ID, Name: row.section.Name, Position: row.section.Position, Items: []DiscoveryItem{}})
			sectionIndex = len(menus[idx].Sections) - 1
		}
		item := row.item
		sellability, sellErr := s.ResolveSellability(ctx, Actor{VendorID: businessID}, Scope{Global: true}, item.ID, in.At)
		if sellErr != nil {
			return DiscoverySnapshot{}, sellErr
		}
		if in.SellableOnly && !sellability.Sellable {
			continue
		}
		price, priceErr := s.ResolvePrice(ctx, Actor{VendorID: businessID}, Scope{Global: true}, item.ID, ResolvePriceInput{})
		if priceErr != nil {
			currency, currencyErr := s.businessCurrency(ctx, s.db, businessID)
			if currencyErr != nil {
				return DiscoverySnapshot{}, currencyErr
			}
			price = ResolvedPrice{Currency: currency, ItemRef: item.ID, ItemVersion: item.Version, BasePriceMinor: item.BasePriceMinor, BaseSource: BaseSourceItem, EffectiveUnitPriceMinor: item.BasePriceMinor, CatalogueRevision: revision, ResolvedAt: s.now(), Modifiers: []PriceComponent{}}
			if override, overrideErr := loadOverride(ctx, s.db, item, false); overrideErr != nil {
				return DiscoverySnapshot{}, overrideErr
			} else if override.BasePriceOverrideMinor != nil {
				price.BaseSource = BaseSourceBranch
				price.BasePriceMinor = *override.BasePriceOverrideMinor
				price.EffectiveUnitPriceMinor = *override.BasePriceOverrideMinor
				price.BranchOverrideRef = override.ID
				price.BranchOverrideVersion = override.Version
			}
		}
		if in.MinPriceMinor != nil && price.EffectiveUnitPriceMinor < *in.MinPriceMinor {
			continue
		}
		if in.MaxPriceMinor != nil && price.EffectiveUnitPriceMinor > *in.MaxPriceMinor {
			continue
		}
		prep, prepErr := s.ResolvePrepProfile(ctx, s.db, item)
		if prepErr != nil {
			return DiscoverySnapshot{}, prepErr
		}
		if in.PrepMaxMinutes != nil && prep.Configured && prep.PrepMinMinutes > *in.PrepMaxMinutes {
			continue
		}
		logistics, found, logisticsErr := loadLogistics(ctx, s.db, item, false)
		if logisticsErr != nil {
			return DiscoverySnapshot{}, logisticsErr
		}
		var logisticsView *LogisticsProfileView
		if found {
			view := LogisticsProfileViewOf(logistics)
			logisticsView = &view
		}
		variants := variantsByItem[item.ID]
		groups := groupsByItem[item.ID]
		media := mediaByItem[item.ID]
		menus[idx].Sections[sectionIndex].Items = append(menus[idx].Sections[sectionIndex].Items, DiscoveryItem{Ref: item.ID, Name: item.Name, Description: item.Description, MediaRefs: media, DietaryCodes: item.DietaryCodes, AllergenCodes: item.AllergenCodes, Price: &price, Variants: variants, ModifierGroups: groups, Sellability: sellability, Prep: prep, Logistics: logisticsView})
	}
	for menuIndex := len(menus) - 1; menuIndex >= 0; menuIndex-- {
		sections := menus[menuIndex].Sections[:0]
		for _, section := range menus[menuIndex].Sections {
			if len(section.Items) != 0 {
				sections = append(sections, section)
			}
		}
		if len(sections) == 0 {
			menus = append(menus[:menuIndex], menus[menuIndex+1:]...)
			continue
		}
		menus[menuIndex].Sections = sections
	}
	return DiscoverySnapshot{BusinessRef: businessID, BranchRef: in.BranchID, Capability: in.Capability, CatalogueRevision: revision, GeneratedAt: s.now(), Menus: menus, Pagination: DiscoveryPage{Limit: in.Limit, Offset: in.Offset, HasMore: hasMore}}, nil
}

func (s *Service) discoveryVariants(ctx context.Context, itemID, businessID string) ([]VariantView, error) {
	rows, err := s.db.Query(ctx, `SELECT `+variantCols+` FROM food_item_variants WHERE business_id=$1 AND item_id=$2 AND status <> 'ARCHIVED' ORDER BY position,id`, businessID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VariantView{}
	for rows.Next() {
		v, scanErr := scanVariant(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, VariantViewOf(v))
	}
	return out, rows.Err()
}

func discoveryItemArgs(businessID string, itemIDs []string) ([]any, string) {
	args := []any{businessID}
	placeholders := make([]string, len(itemIDs))
	for index, itemID := range itemIDs {
		args = append(args, itemID)
		placeholders[index] = "$" + itoa(index+2)
	}
	return args, strings.Join(placeholders, ",")
}

func (s *Service) discoveryVariantsBatch(ctx context.Context, itemIDs []string, businessID string) (map[string][]VariantView, error) {
	out := make(map[string][]VariantView, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}
	args, placeholders := discoveryItemArgs(businessID, itemIDs)
	rows, err := s.db.Query(ctx, `SELECT `+variantCols+` FROM food_item_variants WHERE business_id=$1 AND item_id IN (`+placeholders+`) AND status <> 'ARCHIVED' ORDER BY item_id,position,id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		variant, scanErr := scanVariant(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out[variant.ItemID] = append(out[variant.ItemID], VariantViewOf(variant))
	}
	return out, rows.Err()
}

func (s *Service) discoveryModifierGroups(ctx context.Context, itemID, businessID string) ([]DiscoveryModifierGroup, error) {
	rows, err := s.db.Query(ctx, `SELECT g.id,g.business_id,g.name,g.description,g.min_select,g.max_select,g.status,g.created_at,g.updated_at,g.archived_at,g.version,
		COALESCE(md.id,''),COALESCE(md.business_id,''),COALESCE(md.modifier_group_id,''),COALESCE(md.name,''),COALESCE(md.price_delta_minor,0),COALESCE(md.position,0),COALESCE(md.status,''),COALESCE(md.created_at,'epoch'::timestamptz),COALESCE(md.updated_at,'epoch'::timestamptz),md.archived_at,COALESCE(md.version,0)
		FROM food_item_modifier_groups link JOIN food_modifier_groups g ON g.id=link.modifier_group_id AND g.business_id=link.business_id
		LEFT JOIN food_modifiers md ON md.modifier_group_id=g.id AND md.business_id=g.business_id AND md.status <> 'ARCHIVED'
		WHERE link.business_id=$1 AND link.item_id=$2 AND g.status <> 'ARCHIVED' ORDER BY link.position,md.position,md.id`, businessID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DiscoveryModifierGroup{}
	index := map[string]int{}
	for rows.Next() {
		var group ModifierGroup
		var modifier Modifier
		var groupArchived, modifierArchived *time.Time
		var modifierID, modifierBusiness, modifierGroupID, modifierName, modifierStatus string
		var modifierPrice int64
		var modifierPosition int
		var modifierCreated, modifierUpdated time.Time
		var modifierVersion int64
		if err = rows.Scan(&group.ID, &group.BusinessID, &group.Name, &group.Description, &group.MinSelect, &group.MaxSelect, &group.Status, &group.CreatedAt, &group.UpdatedAt, &groupArchived, &group.Version,
			&modifierID, &modifierBusiness, &modifierGroupID, &modifierName, &modifierPrice, &modifierPosition, &modifierStatus, &modifierCreated, &modifierUpdated, &modifierArchived, &modifierVersion); err != nil {
			return nil, err
		}
		idx, ok := index[group.ID]
		if !ok {
			idx = len(out)
			index[group.ID] = idx
			out = append(out, DiscoveryModifierGroup{Group: ModifierGroupViewOf(group), Modifiers: []ModifierView{}})
		}
		if modifierID != "" {
			modifier = Modifier{ID: modifierID, BusinessID: modifierBusiness, ModifierGroupID: modifierGroupID, Name: modifierName, PriceDeltaMinor: modifierPrice, Position: modifierPosition, Status: modifierStatus, CreatedAt: modifierCreated, UpdatedAt: modifierUpdated, Version: modifierVersion}
			out[idx].Modifiers = append(out[idx].Modifiers, ModifierViewOf(modifier))
		}
	}
	return out, rows.Err()
}

func (s *Service) discoveryModifierGroupsBatch(ctx context.Context, itemIDs []string, businessID string) (map[string][]DiscoveryModifierGroup, error) {
	out := make(map[string][]DiscoveryModifierGroup, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}
	args, placeholders := discoveryItemArgs(businessID, itemIDs)
	rows, err := s.db.Query(ctx, `SELECT link.item_id,g.id,g.business_id,g.name,g.description,g.min_select,g.max_select,g.status,g.created_at,g.updated_at,g.archived_at,g.version,
		COALESCE(md.id,''),COALESCE(md.business_id,''),COALESCE(md.modifier_group_id,''),COALESCE(md.name,''),COALESCE(md.price_delta_minor,0),COALESCE(md.position,0),COALESCE(md.status,''),COALESCE(md.created_at,'epoch'::timestamptz),COALESCE(md.updated_at,'epoch'::timestamptz),md.archived_at,COALESCE(md.version,0)
		FROM food_item_modifier_groups link JOIN food_modifier_groups g ON g.id=link.modifier_group_id AND g.business_id=link.business_id
		LEFT JOIN food_modifiers md ON md.modifier_group_id=g.id AND md.business_id=g.business_id AND md.status <> 'ARCHIVED'
		WHERE link.business_id=$1 AND link.item_id IN (`+placeholders+`) AND g.status <> 'ARCHIVED' ORDER BY link.item_id,link.position,md.position,md.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type groupIndex struct {
		itemID  string
		groupID string
	}
	index := map[groupIndex]int{}
	for rows.Next() {
		var itemID string
		var group ModifierGroup
		var modifier Modifier
		var groupArchived, modifierArchived *time.Time
		var modifierID, modifierBusiness, modifierGroupID, modifierName, modifierStatus string
		var modifierPrice int64
		var modifierPosition int
		var modifierCreated, modifierUpdated time.Time
		var modifierVersion int64
		if err = rows.Scan(&itemID, &group.ID, &group.BusinessID, &group.Name, &group.Description, &group.MinSelect, &group.MaxSelect, &group.Status, &group.CreatedAt, &group.UpdatedAt, &groupArchived, &group.Version,
			&modifierID, &modifierBusiness, &modifierGroupID, &modifierName, &modifierPrice, &modifierPosition, &modifierStatus, &modifierCreated, &modifierUpdated, &modifierArchived, &modifierVersion); err != nil {
			return nil, err
		}
		key := groupIndex{itemID: itemID, groupID: group.ID}
		idx, ok := index[key]
		if !ok {
			idx = len(out[itemID])
			index[key] = idx
			out[itemID] = append(out[itemID], DiscoveryModifierGroup{Group: ModifierGroupViewOf(group), Modifiers: []ModifierView{}})
		}
		if modifierID != "" {
			modifier = Modifier{ID: modifierID, BusinessID: modifierBusiness, ModifierGroupID: modifierGroupID, Name: modifierName, PriceDeltaMinor: modifierPrice, Position: modifierPosition, Status: modifierStatus, CreatedAt: modifierCreated, UpdatedAt: modifierUpdated, Version: modifierVersion}
			out[itemID][idx].Modifiers = append(out[itemID][idx].Modifiers, ModifierViewOf(modifier))
		}
	}
	return out, rows.Err()
}

func (s *Service) discoveryMedia(ctx context.Context, itemID, businessID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT media_ref FROM food_item_media WHERE business_id=$1 AND item_id=$2 ORDER BY position,id`, businessID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := []string{}
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (s *Service) discoveryMediaBatch(ctx context.Context, itemIDs []string, businessID string) (map[string][]string, error) {
	out := make(map[string][]string, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}
	args, placeholders := discoveryItemArgs(businessID, itemIDs)
	rows, err := s.db.Query(ctx, `SELECT item_id,media_ref FROM food_item_media WHERE business_id=$1 AND item_id IN (`+placeholders+`) ORDER BY item_id,position,id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, ref string
		if err = rows.Scan(&itemID, &ref); err != nil {
			return nil, err
		}
		out[itemID] = append(out[itemID], ref)
	}
	return out, rows.Err()
}

func (s *Service) DiscoveryRevision(ctx context.Context, branchID, capability string) (DiscoveryRevision, error) {
	branchID = strings.TrimSpace(branchID)
	capability = strings.ToUpper(strings.TrimSpace(capability))
	businessID, err := s.branchForDiscovery(ctx, branchID, capability)
	if err != nil {
		return DiscoveryRevision{}, err
	}
	var revision int64
	var updated time.Time
	err = s.db.QueryRow(ctx, `SELECT revision,updated_at FROM food_catalogue_revisions WHERE business_id=$1 AND branch_id=$2 AND capability_type=$3`, businessID, branchID, capability).Scan(&revision, &updated)
	if err == pgx.ErrNoRows {
		revision = 0
		updated = time.Time{}
		err = nil
	}
	return DiscoveryRevision{BusinessRef: businessID, BranchRef: branchID, Capability: capability, CatalogueRevision: revision, UpdatedAt: updated}, err
}
