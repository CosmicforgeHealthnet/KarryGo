package shop

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Branch operating models
// ---------------------------------------------------------------------------
//
// SUPERMARKET, GROCERY_SHOP, FOODSTUFF_DEALER, MARKET_TRADER and WHOLESALER
// describe how a branch trades. They refine Shop behaviour; they are not new
// capabilities, not new services and not new catalogues.
//
// A branch may run several — a foodstuff dealer that also wholesales is an
// ordinary Nigerian business, not an edge case — so enabled models are a set
// with an optional primary. Every model must be admitted by a capability THIS
// branch holds: owning a market stall elsewhere does not make a supermarket
// branch a market trader.

const operatingConfigCols = `id,business_id,branch_id,COALESCE(primary_model,''),status,
	created_at,updated_at,archived_at,version`

func scanOperatingConfig(row rowScanner) (BranchOperatingModel, error) {
	var c BranchOperatingModel
	err := row.Scan(&c.ID, &c.BusinessID, &c.BranchID, &c.PrimaryModel, &c.Status,
		&c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.Version)
	return c, err
}

func hydrateOperatingConfig(ctx context.Context, q querier, config *BranchOperatingModel) error {
	rows, err := q.Query(ctx, `SELECT operating_model, admitted_capability
		FROM shop_branch_operating_model_modes WHERE config_id=$1 ORDER BY operating_model`, config.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	config.Models = []BranchOperatingMode{}
	for rows.Next() {
		var mode BranchOperatingMode
		if err = rows.Scan(&mode.Model, &mode.AdmittedCapability); err != nil {
			return err
		}
		config.Models = append(config.Models, mode)
	}
	return rows.Err()
}

// SetOperatingModelsInput configures a branch's Shop operating models.
type SetOperatingModelsInput struct {
	Models          []string
	PrimaryModel    string
	ExpectedVersion int64
}

// SetOperatingModels enables a branch's operating models, or replaces the set.
func (s *Service) SetOperatingModels(ctx context.Context, actor Actor, scope Scope, branchID string, in SetOperatingModelsInput) (BranchOperatingModel, error) {
	if err := s.requireWrites(); err != nil {
		return BranchOperatingModel{}, err
	}
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return BranchOperatingModel{}, err
	}
	branch := strings.TrimSpace(branchID)
	models, err := normalizeOperatingModels(in.Models)
	if err != nil {
		return BranchOperatingModel{}, err
	}
	if len(models) == 0 {
		return BranchOperatingModel{}, fieldError("models", "Enable at least one operating model.")
	}
	primary := strings.ToUpper(strings.TrimSpace(in.PrimaryModel))
	if primary != "" && !contains(models, primary) {
		return BranchOperatingModel{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "primary_model", Message: "The primary model must be one of the enabled models."}}),
			CodeOperatingModelInvalid)
	}

	var configured BranchOperatingModel
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		capabilities, err := s.requireShopBranch(ctx, tx, actor.VendorID, branch)
		if err != nil {
			return err
		}
		admitted := make(map[string]string, len(models))
		for _, model := range models {
			capability, admitErr := admitOperatingModel(model, capabilities)
			if admitErr != nil {
				return admitErr
			}
			admitted[model] = capability
		}

		existing, err := scanOperatingConfig(tx.QueryRow(ctx, `SELECT `+operatingConfigCols+`
			FROM shop_branch_operating_models WHERE business_id=$1 AND branch_id=$2 FOR UPDATE`,
			actor.VendorID, branch))
		found := err == nil
		if err != nil && !isNoRows(err) {
			return mapDB(err, "Operating model configuration", CodeOperatingModelInvalid)
		}

		var before any
		if found {
			if err = expectedVersionMatches(existing.Version, in.ExpectedVersion); err != nil {
				return err
			}
			if err = hydrateOperatingConfig(ctx, tx, &existing); err != nil {
				return err
			}
			before = OperatingModelViewOf(existing)
			row, updateErr := scanOperatingConfig(tx.QueryRow(ctx, `
				UPDATE shop_branch_operating_models
				SET primary_model=NULLIF($3,''), status='ACTIVE', archived_at=NULL,
				    updated_at=now(), version=version+1
				WHERE business_id=$1 AND id=$2 RETURNING `+operatingConfigCols,
				actor.VendorID, existing.ID, primary))
			if updateErr != nil {
				return mapDB(updateErr, "Operating model configuration", CodeOperatingModelInvalid)
			}
			configured = row
		} else {
			row, insertErr := scanOperatingConfig(tx.QueryRow(ctx, `
				INSERT INTO shop_branch_operating_models (id,business_id,branch_id,primary_model,status)
				VALUES ($1,$2,$3,NULLIF($4,''),'ACTIVE') RETURNING `+operatingConfigCols,
				newID("sbo_"), actor.VendorID, branch, primary))
			if insertErr != nil {
				return mapDB(insertErr, "Operating model configuration", CodeOperatingModelInvalid)
			}
			configured = row
		}

		// A model being removed must not leave listings claiming it.
		if _, err = tx.Exec(ctx, `
			UPDATE shop_branch_listings SET operating_model=NULL, updated_at=now(), version=version+1
			WHERE business_id=$1 AND branch_id=$2 AND operating_model IS NOT NULL
			  AND NOT (operating_model = ANY($3))`, actor.VendorID, branch, models); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM shop_branch_operating_model_modes WHERE config_id=$1`,
			configured.ID); err != nil {
			return err
		}
		for _, model := range models {
			if _, err = tx.Exec(ctx, `
				INSERT INTO shop_branch_operating_model_modes
					(id,business_id,config_id,branch_id,operating_model,admitted_capability)
				VALUES ($1,$2,$3,$4,$5,$6)`,
				newID("sbm_"), actor.VendorID, configured.ID, branch, model, admitted[model]); err != nil {
				return mapDB(err, "Operating model", CodeOperatingModelInvalid)
			}
		}
		if err = hydrateOperatingConfig(ctx, tx, &configured); err != nil {
			return err
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, branch); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.operating_models.configured", eventType: "shop.operating_model.changed",
			aggregateType: "shop_branch_operating_model", aggregateID: configured.ID, branchID: branch,
			version: configured.Version, before: before, after: OperatingModelViewOf(configured),
			payload: OperatingModelViewOf(configured),
		})
	})
	return configured, err
}

// GetOperatingModels reads a branch's Shop operating configuration together
// with the capabilities that could admit further models.
func (s *Service) GetOperatingModels(ctx context.Context, actor Actor, scope Scope, branchID string) (BranchOperatingModel, []string, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return BranchOperatingModel{}, nil, err
	}
	branch := strings.TrimSpace(branchID)
	capabilities, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branch)
	if err != nil {
		return BranchOperatingModel{}, nil, err
	}
	eligible := []string{}
	for _, model := range OperatingModels {
		if _, admitErr := admitOperatingModel(model, capabilities); admitErr == nil {
			eligible = append(eligible, model)
		}
	}
	config, err := scanOperatingConfig(s.db.QueryRow(ctx, `SELECT `+operatingConfigCols+`
		FROM shop_branch_operating_models WHERE business_id=$1 AND branch_id=$2`, actor.VendorID, branch))
	if err != nil {
		if isNoRows(err) {
			return BranchOperatingModel{BusinessID: actor.VendorID, BranchID: branch,
				Models: []BranchOperatingMode{}}, eligible, nil
		}
		return BranchOperatingModel{}, nil, mapDB(err, "Operating model configuration", CodeOperatingModelInvalid)
	}
	return config, eligible, hydrateOperatingConfig(ctx, s.db, &config)
}

// ArchiveOperatingModels retires a branch's Shop operating configuration,
// deactivating the listings it governed rather than leaving them sellable
// under a configuration that no longer exists.
func (s *Service) ArchiveOperatingModels(ctx context.Context, actor Actor, scope Scope, branchID string, expectedVersion int64) (BranchOperatingModel, error) {
	if err := s.requireWrites(); err != nil {
		return BranchOperatingModel{}, err
	}
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return BranchOperatingModel{}, err
	}
	branch := strings.TrimSpace(branchID)
	var archived BranchOperatingModel
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := scanOperatingConfig(tx.QueryRow(ctx, `SELECT `+operatingConfigCols+`
			FROM shop_branch_operating_models WHERE business_id=$1 AND branch_id=$2 FOR UPDATE`,
			actor.VendorID, branch))
		if err != nil {
			return mapDB(err, "Operating model configuration", CodeOperatingModelInvalid)
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if err = hydrateOperatingConfig(ctx, tx, &current); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		row, err := scanOperatingConfig(tx.QueryRow(ctx, `
			UPDATE shop_branch_operating_models
			SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+operatingConfigCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Operating model configuration", CodeOperatingModelInvalid)
		}
		archived = row
		archived.Models = current.Models
		if _, err = tx.Exec(ctx, `
			UPDATE shop_branch_listings SET status='INACTIVE', updated_at=now(), version=version+1
			WHERE business_id=$1 AND branch_id=$2 AND status='ACTIVE'`, actor.VendorID, branch); err != nil {
			return err
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, branch); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.operating_models.archived", eventType: "shop.operating_model.changed",
			aggregateType: "shop_branch_operating_model", aggregateID: archived.ID, branchID: branch,
			version: archived.Version, before: OperatingModelViewOf(current), after: OperatingModelViewOf(archived),
			payload: OperatingModelViewOf(archived),
		})
	})
	return archived, err
}
