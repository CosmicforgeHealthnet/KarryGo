package shop

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Shop taxonomy
// ---------------------------------------------------------------------------
//
// A bounded tree, deterministically ordered. Every structural rule — no self
// parent, no cycle, no cross-tenant parent, no unbounded depth — is enforced
// here AND in the schema, because a tree that can be corrupted by one bad
// write is a tree nobody can render.

const categoryCols = `id,business_id,parent_id,name,slug,description,position,depth,status,
	created_at,updated_at,archived_at,version`

func scanCategory(row rowScanner) (Category, error) {
	var c Category
	var parent, description *string
	err := row.Scan(&c.ID, &c.BusinessID, &parent, &c.Name, &c.Slug, &description, &c.Position,
		&c.Depth, &c.Status, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.Version)
	c.ParentID = derefString(parent)
	c.Description = derefString(description)
	return c, err
}

func loadCategory(ctx context.Context, q querier, businessID, categoryID string, lock bool) (Category, error) {
	sql := `SELECT ` + categoryCols + ` FROM shop_categories WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	category, err := scanCategory(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(categoryID)))
	if err != nil {
		return Category{}, mapDB(err, "Category", CodeCategoryNotFound)
	}
	return category, nil
}

// CreateCategoryInput is the create command.
type CreateCategoryInput struct {
	ParentID    string
	Name        string
	Slug        string
	Description string
	Position    *int
}

// CreateCategory adds a node to the taxonomy.
func (s *Service) CreateCategory(ctx context.Context, actor Actor, scope Scope, in CreateCategoryInput) (Category, error) {
	if err := s.requireWrites(); err != nil {
		return Category{}, err
	}
	if err := requireBusinessWide(scope, "Shop categories"); err != nil {
		return Category{}, err
	}
	name, violation := validateName("name", in.Name, 2, 120)
	if violation != nil {
		return Category{}, validation(*violation)
	}
	description, violation := validateOptionalText("description", in.Description, 2000)
	if violation != nil {
		return Category{}, validation(*violation)
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if slug == "" {
		slug = slugify(name)
	}
	if !slugPattern.MatchString(slug) {
		return Category{}, fieldError("slug", "Use lowercase letters, digits and hyphens only.")
	}
	parentID := strings.TrimSpace(in.ParentID)

	var created Category
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		depth := 0
		if parentID != "" {
			parent, err := loadCategory(ctx, tx, actor.VendorID, parentID, true)
			if err != nil {
				return err
			}
			if parent.Status == StatusArchived {
				return errArchived("The parent category")
			}
			depth = parent.Depth + 1
			if depth > MaxCategoryDepth {
				return coded(apperrors.Conflict("The category tree may not be deeper than "+itoa(MaxCategoryDepth+1)+" levels.", nil),
					CodeCategoryTooDeep)
			}
		}
		position := 0
		if in.Position != nil && *in.Position >= 0 {
			position = *in.Position
		} else {
			if err := tx.QueryRow(ctx, `
				SELECT count(*) FROM shop_categories
				WHERE business_id=$1 AND COALESCE(parent_id,'')=$2`,
				actor.VendorID, parentID).Scan(&position); err != nil {
				return err
			}
		}
		row, err := scanCategory(tx.QueryRow(ctx, `
			INSERT INTO shop_categories (id,business_id,parent_id,name,slug,description,position,depth,status)
			VALUES ($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),$7,$8,'ACTIVE') RETURNING `+categoryCols,
			newID("scg_"), actor.VendorID, parentID, name, slug, description, position, depth))
		if err != nil {
			return mapDB(err, "Category", CodeCategoryNotFound)
		}
		created = row
		return s.record(ctx, tx, actor, evidence{
			action: "shop.category.created", eventType: "shop.category.changed",
			aggregateType: "shop_category", aggregateID: created.ID, version: created.Version,
			after: CategoryViewOf(created), payload: CategoryViewOf(created),
		})
	})
	return created, err
}

// CategoryListOptions bounds a taxonomy read.
type CategoryListOptions struct {
	ParentID        string
	IncludeArchived bool
	Limit, Offset   int
}

// ListCategories reads the taxonomy in deterministic order.
func (s *Service) ListCategories(ctx context.Context, actor Actor, scope Scope, in CategoryListOptions) ([]Category, bool, error) {
	if scope.Empty() {
		return nil, false, errForbidden()
	}
	limit, offset := boundedPage(in.Limit, in.Offset)
	sql := `SELECT ` + categoryCols + ` FROM shop_categories WHERE business_id=$1`
	args := []any{actor.VendorID}
	if parent := strings.TrimSpace(in.ParentID); parent != "" {
		args = append(args, parent)
		sql += ` AND parent_id=$2`
	}
	if !in.IncludeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	args = append(args, limit+1, offset)
	sql += ` ORDER BY depth, COALESCE(parent_id,''), position, id LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var categories []Category
	for rows.Next() {
		category, scanErr := scanCategory(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		categories = append(categories, category)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(categories) > limit
	if hasMore {
		categories = categories[:limit]
	}
	return categories, hasMore, nil
}

// GetCategory reads one node.
func (s *Service) GetCategory(ctx context.Context, actor Actor, scope Scope, categoryID string) (Category, error) {
	if scope.Empty() {
		return Category{}, errForbidden()
	}
	return loadCategory(ctx, s.db, actor.VendorID, categoryID, false)
}

// UpdateCategoryInput is the update command. A nil field is left alone; the
// parent is moved only when ParentID is non-nil, and an empty string in it
// means "make this a root".
type UpdateCategoryInput struct {
	Name            *string
	Slug            *string
	Description     *string
	Position        *int
	ParentID        *string
	ExpectedVersion int64
}

// UpdateCategory edits a node, including moving it in the tree.
func (s *Service) UpdateCategory(ctx context.Context, actor Actor, scope Scope, categoryID string, in UpdateCategoryInput) (Category, error) {
	if err := s.requireWrites(); err != nil {
		return Category{}, err
	}
	if err := requireBusinessWide(scope, "Shop categories"); err != nil {
		return Category{}, err
	}
	var updated Category
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadCategory(ctx, tx, actor.VendorID, categoryID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This category")
		}

		name, slug, description := current.Name, current.Slug, current.Description
		position, parentID, depth := current.Position, current.ParentID, current.Depth

		if in.Name != nil {
			value, violation := validateName("name", *in.Name, 2, 120)
			if violation != nil {
				return validation(*violation)
			}
			name = value
		}
		if in.Slug != nil {
			value := strings.ToLower(strings.TrimSpace(*in.Slug))
			if !slugPattern.MatchString(value) {
				return fieldError("slug", "Use lowercase letters, digits and hyphens only.")
			}
			slug = value
		}
		if in.Description != nil {
			value, violation := validateOptionalText("description", *in.Description, 2000)
			if violation != nil {
				return validation(*violation)
			}
			description = value
		}
		if in.Position != nil {
			if *in.Position < 0 {
				return fieldError("position", "Position must be zero or greater.")
			}
			position = *in.Position
		}
		if in.ParentID != nil {
			candidate := strings.TrimSpace(*in.ParentID)
			if candidate == current.ID {
				return coded(apperrors.Conflict("A category cannot be its own parent.", nil), CodeCategoryCycle)
			}
			depth = 0
			if candidate != "" {
				parent, parentErr := loadCategory(ctx, tx, actor.VendorID, candidate, false)
				if parentErr != nil {
					return parentErr
				}
				if parent.Status == StatusArchived {
					return errArchived("The parent category")
				}
				// Walking up from the proposed parent is the whole cycle
				// check: if this category is anywhere above it, the move would
				// close a loop.
				cycle, cycleErr := ancestorContains(ctx, tx, actor.VendorID, candidate, current.ID)
				if cycleErr != nil {
					return cycleErr
				}
				if cycle {
					return coded(apperrors.Conflict("That move would place the category inside its own subtree.", nil),
						CodeCategoryCycle)
				}
				depth = parent.Depth + 1
			}
			parentID = candidate
			// The subtree moves with the node, so the deepest descendant is
			// what the bound has to hold for.
			subtreeDepth, depthErr := subtreeHeight(ctx, tx, actor.VendorID, current.ID)
			if depthErr != nil {
				return depthErr
			}
			if depth+subtreeDepth > MaxCategoryDepth {
				return coded(apperrors.Conflict("That move would exceed the maximum category depth.", nil),
					CodeCategoryTooDeep)
			}
		}

		row, err := scanCategory(tx.QueryRow(ctx, `
			UPDATE shop_categories
			SET parent_id=NULLIF($3,''), name=$4, slug=$5, description=NULLIF($6,''), position=$7,
			    depth=$8, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+categoryCols,
			actor.VendorID, current.ID, parentID, name, slug, description, position, depth))
		if err != nil {
			return mapDB(err, "Category", CodeCategoryNotFound)
		}
		updated = row
		if depth != current.Depth {
			if err = reindexSubtreeDepth(ctx, tx, actor.VendorID, current.ID, depth); err != nil {
				return err
			}
		}
		if _, err = bumpRevisionsForCategory(ctx, tx, actor.VendorID, current.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.category.updated", eventType: "shop.category.changed",
			aggregateType: "shop_category", aggregateID: updated.ID, version: updated.Version,
			before: CategoryViewOf(current), after: CategoryViewOf(updated), payload: CategoryViewOf(updated),
		})
	})
	return updated, err
}

// ancestorContains reports whether needle appears at or above start.
func ancestorContains(ctx context.Context, q querier, businessID, start, needle string) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `
		WITH RECURSIVE up AS (
			SELECT id, parent_id, 1 AS level FROM shop_categories WHERE business_id=$1 AND id=$2
			UNION ALL
			SELECT c.id, c.parent_id, up.level+1 FROM shop_categories c
			JOIN up ON c.id = up.parent_id AND c.business_id=$1
			WHERE up.level <= $4
		)
		SELECT EXISTS (SELECT 1 FROM up WHERE id=$3)`,
		businessID, start, needle, MaxCategoryDepth+2).Scan(&found)
	return found, err
}

// subtreeHeight is how many levels hang below a node (0 for a leaf).
func subtreeHeight(ctx context.Context, q querier, businessID, rootID string) (int, error) {
	var height int
	err := q.QueryRow(ctx, `
		WITH RECURSIVE down AS (
			SELECT id, 0 AS level FROM shop_categories WHERE business_id=$1 AND id=$2
			UNION ALL
			SELECT c.id, down.level+1 FROM shop_categories c
			JOIN down ON c.parent_id = down.id AND c.business_id=$1
			WHERE down.level <= $3
		)
		SELECT COALESCE(MAX(level),0) FROM down`,
		businessID, rootID, MaxCategoryDepth+2).Scan(&height)
	return height, err
}

// reindexSubtreeDepth rewrites stored depth for a moved subtree, so the depth
// column stays a fact rather than a stale copy.
func reindexSubtreeDepth(ctx context.Context, tx pgx.Tx, businessID, rootID string, rootDepth int) error {
	_, err := tx.Exec(ctx, `
		WITH RECURSIVE down AS (
			SELECT id, $3::int AS new_depth FROM shop_categories WHERE business_id=$1 AND id=$2
			UNION ALL
			SELECT c.id, down.new_depth+1 FROM shop_categories c
			JOIN down ON c.parent_id = down.id AND c.business_id=$1
			WHERE down.new_depth <= $4
		)
		UPDATE shop_categories t SET depth = down.new_depth, updated_at = now()
		FROM down WHERE t.id = down.id AND t.business_id=$1 AND t.depth <> down.new_depth`,
		businessID, rootID, rootDepth, MaxCategoryDepth+1)
	return err
}

// ArchiveCategory retires a node. Archiving is refused while live children or
// live products still hang off it, because silently orphaning a catalogue is
// worse than making the vendor tidy up first.
func (s *Service) ArchiveCategory(ctx context.Context, actor Actor, scope Scope, categoryID string, expectedVersion int64) (Category, error) {
	if err := s.requireWrites(); err != nil {
		return Category{}, err
	}
	if err := requireBusinessWide(scope, "Shop categories"); err != nil {
		return Category{}, err
	}
	var archived Category
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadCategory(ctx, tx, actor.VendorID, categoryID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		var children, products int
		if err = tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM shop_categories
			        WHERE business_id=$1 AND parent_id=$2 AND status <> 'ARCHIVED'),
			       (SELECT count(*) FROM shop_products
			        WHERE business_id=$1 AND category_id=$2 AND status <> 'ARCHIVED')`,
			actor.VendorID, current.ID).Scan(&children, &products); err != nil {
			return err
		}
		if children > 0 || products > 0 {
			return apperrors.Conflict("Archive or move the categories and products inside this category first.", nil)
		}
		row, err := scanCategory(tx.QueryRow(ctx, `
			UPDATE shop_categories SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+categoryCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Category", CodeCategoryNotFound)
		}
		archived = row
		if _, err = bumpRevisionsForCategory(ctx, tx, actor.VendorID, current.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.category.archived", eventType: "shop.category.changed",
			aggregateType: "shop_category", aggregateID: archived.ID, version: archived.Version,
			before: CategoryViewOf(current), after: CategoryViewOf(archived), payload: CategoryViewOf(archived),
		})
	})
	return archived, err
}

// RestoreCategory reactivates an archived node, provided its parent is live.
func (s *Service) RestoreCategory(ctx context.Context, actor Actor, scope Scope, categoryID string, expectedVersion int64) (Category, error) {
	if err := s.requireWrites(); err != nil {
		return Category{}, err
	}
	if err := requireBusinessWide(scope, "Shop categories"); err != nil {
		return Category{}, err
	}
	var restored Category
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadCategory(ctx, tx, actor.VendorID, categoryID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status != StatusArchived {
			restored = current
			return nil
		}
		if current.ParentID != "" {
			parent, parentErr := loadCategory(ctx, tx, actor.VendorID, current.ParentID, false)
			if parentErr != nil {
				return parentErr
			}
			if parent.Status == StatusArchived {
				return errArchived("The parent category")
			}
		}
		row, err := scanCategory(tx.QueryRow(ctx, `
			UPDATE shop_categories SET status='ACTIVE', archived_at=NULL, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+categoryCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Category", CodeCategoryNotFound)
		}
		restored = row
		if _, err = bumpRevisionsForCategory(ctx, tx, actor.VendorID, current.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.category.restored", eventType: "shop.category.changed",
			aggregateType: "shop_category", aggregateID: restored.ID, version: restored.Version,
			before: CategoryViewOf(current), after: CategoryViewOf(restored), payload: CategoryViewOf(restored),
		})
	})
	return restored, err
}
