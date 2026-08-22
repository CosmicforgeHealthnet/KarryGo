package searchprojection

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresFoodSearchBackend struct {
	pool *pgxpool.Pool
}

func NewPostgresFoodSearchBackend(
	pool *pgxpool.Pool,
) *PostgresFoodSearchBackend {
	return &PostgresFoodSearchBackend{
		pool: pool,
	}
}

type PostgresShopSearchBackend struct {
	pool *pgxpool.Pool
}

func NewPostgresShopSearchBackend(
	pool *pgxpool.Pool,
) *PostgresShopSearchBackend {
	return &PostgresShopSearchBackend{
		pool: pool,
	}
}

func InspectPostgresSearchCapabilities(
	ctx context.Context,
	pool *pgxpool.Pool,
) (
	PostgresSearchCapabilities,
	error,
) {
	if pool == nil {
		return PostgresSearchCapabilities{},
			ErrInvalidArgument
	}

	var capabilities PostgresSearchCapabilities

	err := pool.QueryRow(
		ctx,
		`
        SELECT
            to_regprocedure(
                'pg_catalog.to_tsvector(regconfig,text)'
            ) IS NOT NULL,

            EXISTS (
                SELECT 1
                  FROM pg_catalog.pg_am
                 WHERE amname='gin'
            ),

            EXISTS (
                SELECT 1
                  FROM pg_catalog.pg_available_extensions
                 WHERE name='pg_trgm'
            ),

            EXISTS (
                SELECT 1
                  FROM pg_catalog.pg_extension
                 WHERE extname='pg_trgm'
            )
        `,
	).Scan(
		&capabilities.BuiltInTextSearch,
		&capabilities.GINAvailable,
		&capabilities.PGTrgmAvailable,
		&capabilities.PGTrgmInstalled,
	)
	if err != nil {
		return PostgresSearchCapabilities{},
			err
	}

	return capabilities, nil
}

func (b *PostgresFoodSearchBackend) Upsert(
	ctx context.Context,
	document FoodSearchDocument,
) (
	ProjectionWriteResult,
	error,
) {
	if b == nil ||
		b.pool == nil ||
		document.DocumentID == "" ||
		document.Generation <= 0 ||
		document.BusinessRef == "" ||
		document.BranchRef == "" ||
		document.FoodItemRef == "" ||
		document.CatalogueRevision <= 0 ||
		document.NormalizedName == "" ||
		document.SearchText == "" {
		return ProjectionWriteResult{},
			ErrInvalidArgument
	}

	tag, err := b.pool.Exec(
		ctx,
		`
        INSERT INTO food_search_documents (
            generation,
            document_id,
            business_ref,
            branch_ref,
            food_item_ref,
            menu_refs,
            name,
            normalized_name,
            search_tokens,
            aliases,
            synonyms,
            canonical_taxonomy,
            provider_types,
            cuisine_terms,
            meal_periods,
            dish_types,
            food_groups,
            occasions,
            dietary_claims,
            preference_claims,
            active,
            availability_summary,
            effective_price_summary,
            serviceability_hint,
            trust_hint,
            reliability_hint,
            promotion_indicator,
            catalogue_revision,
            projection_revision,
            search_text,
            updated_at
        )
        VALUES (
            @generation,
            @document_id,
            @business_ref,
            @branch_ref,
            @food_item_ref,
            @menu_refs,
            @name,
            @normalized_name,
            @search_tokens,
            @aliases,
            @synonyms,
            @canonical_taxonomy,
            @provider_types,
            @cuisine_terms,
            @meal_periods,
            @dish_types,
            @food_groups,
            @occasions,
            @dietary_claims,
            @preference_claims,
            @active,
            @availability_summary,
            @effective_price_summary,
            @serviceability_hint,
            @trust_hint,
            @reliability_hint,
            @promotion_indicator,
            @catalogue_revision,
            1,
            @search_text,
            now()
        )
        ON CONFLICT (
            generation,
            document_id
        )
        DO UPDATE SET
            business_ref=EXCLUDED.business_ref,
            branch_ref=EXCLUDED.branch_ref,
            food_item_ref=EXCLUDED.food_item_ref,
            menu_refs=EXCLUDED.menu_refs,
            name=EXCLUDED.name,
            normalized_name=EXCLUDED.normalized_name,
            search_tokens=EXCLUDED.search_tokens,
            aliases=EXCLUDED.aliases,
            synonyms=EXCLUDED.synonyms,
            canonical_taxonomy=EXCLUDED.canonical_taxonomy,
            provider_types=EXCLUDED.provider_types,
            cuisine_terms=EXCLUDED.cuisine_terms,
            meal_periods=EXCLUDED.meal_periods,
            dish_types=EXCLUDED.dish_types,
            food_groups=EXCLUDED.food_groups,
            occasions=EXCLUDED.occasions,
            dietary_claims=EXCLUDED.dietary_claims,
            preference_claims=EXCLUDED.preference_claims,
            active=EXCLUDED.active,
            availability_summary=EXCLUDED.availability_summary,
            effective_price_summary=EXCLUDED.effective_price_summary,
            serviceability_hint=EXCLUDED.serviceability_hint,
            trust_hint=EXCLUDED.trust_hint,
            reliability_hint=EXCLUDED.reliability_hint,
            promotion_indicator=EXCLUDED.promotion_indicator,
            catalogue_revision=EXCLUDED.catalogue_revision,
            projection_revision=
                food_search_documents.projection_revision + 1,
            search_text=EXCLUDED.search_text,
            updated_at=now()
        WHERE
            EXCLUDED.catalogue_revision >
            food_search_documents.catalogue_revision
        `,
		pgx.NamedArgs{
			"generation": document.Generation,

			"document_id": document.DocumentID,

			"business_ref": document.BusinessRef,

			"branch_ref": document.BranchRef,

			"food_item_ref": document.FoodItemRef,

			"menu_refs": document.MenuRefs,

			"name": document.Name,

			"normalized_name": document.NormalizedName,

			"search_tokens": document.SearchTokens,

			"aliases": document.Aliases,

			"synonyms": document.Synonyms,

			"canonical_taxonomy": document.CanonicalTaxonomy,

			"provider_types": document.ProviderTypes,

			"cuisine_terms": document.CuisineTerms,

			"meal_periods": document.MealPeriods,

			"dish_types": document.DishTypes,

			"food_groups": document.FoodGroups,

			"occasions": document.Occasions,

			"dietary_claims": document.DietaryClaims,

			"preference_claims": document.PreferenceClaims,

			"active": document.Active,

			"availability_summary": document.AvailabilitySummary,

			"effective_price_summary": document.EffectivePriceSummary,

			"serviceability_hint": document.ServiceabilityHint,

			"trust_hint": document.TrustHint,

			"reliability_hint": document.ReliabilityHint,

			"promotion_indicator": document.PromotionIndicator,

			"catalogue_revision": document.CatalogueRevision,

			"search_text": document.SearchText,
		},
	)
	if err != nil {
		return ProjectionWriteResult{},
			err
	}

	var projectionRevision int64
	var catalogueRevision int64

	err = b.pool.QueryRow(
		ctx,
		`
        SELECT
            projection_revision,
            catalogue_revision
          FROM food_search_documents
         WHERE generation=$1
           AND document_id=$2
        `,
		document.Generation,
		document.DocumentID,
	).Scan(
		&projectionRevision,
		&catalogueRevision,
	)
	if err != nil {
		return ProjectionWriteResult{},
			err
	}

	return ProjectionWriteResult{
		Applied: tag.RowsAffected() > 0,

		ProjectionRevision: projectionRevision,

		CatalogueRevision: catalogueRevision,
	}, nil
}

func (b *PostgresShopSearchBackend) Upsert(
	ctx context.Context,
	document ShopSearchDocument,
) (
	ProjectionWriteResult,
	error,
) {
	if b == nil ||
		b.pool == nil ||
		document.DocumentID == "" ||
		document.Generation <= 0 ||
		document.BusinessRef == "" ||
		document.BranchRef == "" ||
		document.ProductRef == "" ||
		document.CatalogueRevision <= 0 ||
		document.InventoryRevision <= 0 ||
		document.NormalizedName == "" ||
		document.SearchText == "" {
		return ProjectionWriteResult{},
			ErrInvalidArgument
	}

	tag, err := b.pool.Exec(
		ctx,
		`
        INSERT INTO shop_search_documents (
            generation,
            document_id,
            business_ref,
            branch_ref,
            product_ref,
            variant_ref,
            listing_ref,
            name,
            brand,
            normalized_name,
            search_tokens,
            aliases,
            synonyms,
            canonical_taxonomy,
            sell_units,
            measure_names,
            local_measure_aliases,
            packaging_terms,
            sku_terms,
            barcode_terms,
            retail_eligible,
            wholesale_eligible,
            active,
            effective_price_summary,
            inventory_availability_summary,
            preorder_hint,
            backorder_hint,
            fresh_attributes,
            temperature_zone,
            handling_metadata,
            catalogue_revision,
            inventory_revision,
            projection_revision,
            search_text,
            updated_at
        )
        VALUES (
            @generation,
            @document_id,
            @business_ref,
            @branch_ref,
            @product_ref,
            NULLIF(@variant_ref, ''),
            NULLIF(@listing_ref, ''),
            @name,
            NULLIF(@brand, ''),
            @normalized_name,
            @search_tokens,
            @aliases,
            @synonyms,
            @canonical_taxonomy,
            @sell_units,
            @measure_names,
            @local_measure_aliases,
            @packaging_terms,
            @sku_terms,
            @barcode_terms,
            @retail_eligible,
            @wholesale_eligible,
            @active,
            @effective_price_summary,
            @inventory_availability_summary,
            @preorder_hint,
            @backorder_hint,
            @fresh_attributes,
            NULLIF(@temperature_zone, ''),
            @handling_metadata,
            @catalogue_revision,
            @inventory_revision,
            1,
            @search_text,
            now()
        )
        ON CONFLICT (
            generation,
            document_id
        )
        DO UPDATE SET
            business_ref=EXCLUDED.business_ref,
            branch_ref=EXCLUDED.branch_ref,
            product_ref=EXCLUDED.product_ref,
            variant_ref=EXCLUDED.variant_ref,
            listing_ref=EXCLUDED.listing_ref,
            name=EXCLUDED.name,
            brand=EXCLUDED.brand,
            normalized_name=EXCLUDED.normalized_name,
            search_tokens=EXCLUDED.search_tokens,
            aliases=EXCLUDED.aliases,
            synonyms=EXCLUDED.synonyms,
            canonical_taxonomy=EXCLUDED.canonical_taxonomy,
            sell_units=EXCLUDED.sell_units,
            measure_names=EXCLUDED.measure_names,
            local_measure_aliases=EXCLUDED.local_measure_aliases,
            packaging_terms=EXCLUDED.packaging_terms,
            sku_terms=EXCLUDED.sku_terms,
            barcode_terms=EXCLUDED.barcode_terms,
            retail_eligible=EXCLUDED.retail_eligible,
            wholesale_eligible=EXCLUDED.wholesale_eligible,
            active=EXCLUDED.active,
            effective_price_summary=EXCLUDED.effective_price_summary,
            inventory_availability_summary=
                EXCLUDED.inventory_availability_summary,
            preorder_hint=EXCLUDED.preorder_hint,
            backorder_hint=EXCLUDED.backorder_hint,
            fresh_attributes=EXCLUDED.fresh_attributes,
            temperature_zone=EXCLUDED.temperature_zone,
            handling_metadata=EXCLUDED.handling_metadata,
            catalogue_revision=EXCLUDED.catalogue_revision,
            inventory_revision=EXCLUDED.inventory_revision,
            projection_revision=
                shop_search_documents.projection_revision + 1,
            search_text=EXCLUDED.search_text,
            updated_at=now()
        WHERE
            EXCLUDED.catalogue_revision >=
                shop_search_documents.catalogue_revision
            AND
            EXCLUDED.inventory_revision >=
                shop_search_documents.inventory_revision
            AND (
                EXCLUDED.catalogue_revision >
                    shop_search_documents.catalogue_revision
                OR
                EXCLUDED.inventory_revision >
                    shop_search_documents.inventory_revision
            )
        `,
		pgx.NamedArgs{
			"generation": document.Generation,

			"document_id": document.DocumentID,

			"business_ref": document.BusinessRef,

			"branch_ref": document.BranchRef,

			"product_ref": document.ProductRef,

			"variant_ref": document.VariantRef,

			"listing_ref": document.ListingRef,

			"name": document.Name,

			"brand": document.Brand,

			"normalized_name": document.NormalizedName,

			"search_tokens": document.SearchTokens,

			"aliases": document.Aliases,

			"synonyms": document.Synonyms,

			"canonical_taxonomy": document.CanonicalTaxonomy,

			"sell_units": document.SellUnits,

			"measure_names": document.MeasureNames,

			"local_measure_aliases": document.LocalMeasureAliases,

			"packaging_terms": document.PackagingTerms,

			"sku_terms": document.SKUTerms,

			"barcode_terms": document.BarcodeTerms,

			"retail_eligible": document.RetailEligible,

			"wholesale_eligible": document.WholesaleEligible,

			"active": document.Active,

			"effective_price_summary": document.EffectivePriceSummary,

			"inventory_availability_summary": document.InventoryAvailabilitySummary,

			"preorder_hint": document.PreorderHint,

			"backorder_hint": document.BackorderHint,

			"fresh_attributes": document.FreshAttributes,

			"temperature_zone": document.TemperatureZone,

			"handling_metadata": document.HandlingMetadata,

			"catalogue_revision": document.CatalogueRevision,

			"inventory_revision": document.InventoryRevision,

			"search_text": document.SearchText,
		},
	)
	if err != nil {
		return ProjectionWriteResult{},
			err
	}

	var projectionRevision int64
	var catalogueRevision int64
	var inventoryRevision int64

	err = b.pool.QueryRow(
		ctx,
		`
        SELECT
            projection_revision,
            catalogue_revision,
            inventory_revision
          FROM shop_search_documents
         WHERE generation=$1
           AND document_id=$2
        `,
		document.Generation,
		document.DocumentID,
	).Scan(
		&projectionRevision,
		&catalogueRevision,
		&inventoryRevision,
	)
	if err != nil {
		return ProjectionWriteResult{},
			err
	}

	return ProjectionWriteResult{
		Applied: tag.RowsAffected() > 0,

		ProjectionRevision: projectionRevision,

		CatalogueRevision: catalogueRevision,

		InventoryRevision: inventoryRevision,
	}, nil
}

func (b *PostgresFoodSearchBackend) Search(
	ctx context.Context,
	request SearchRequest,
) (
	[]FoodSearchHit,
	error,
) {
	if b == nil ||
		b.pool == nil {
		return nil, ErrInvalidArgument
	}

	request, query, err :=
		normalizeSearchRequest(
			request,
		)
	if err != nil {
		return nil, err
	}

	rows, err := b.pool.Query(
		ctx,
		`
        WITH search_query AS (
            SELECT plainto_tsquery(
                'simple'::regconfig,
                $1
            ) AS query
        )
        SELECT
            d.document_id,
            d.business_ref,
            d.branch_ref,
            d.food_item_ref,
            d.name,
            d.generation,
            d.projection_revision,
            ts_rank_cd(
                d.search_vector,
                search_query.query
            ) AS rank
          FROM food_search_documents d
          CROSS JOIN search_query
         WHERE d.generation=$2
           AND d.active=TRUE
           AND (
               $3=''
               OR d.branch_ref=$3
           )
           AND d.search_vector @@ search_query.query
         ORDER BY
            rank DESC,
            d.normalized_name ASC,
            d.document_id ASC
         LIMIT $4
        `,
		query,
		request.Generation,
		request.BranchRef,
		request.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hits := make(
		[]FoodSearchHit,
		0,
		request.Limit,
	)

	for rows.Next() {
		var hit FoodSearchHit

		if err = rows.Scan(
			&hit.DocumentID,
			&hit.BusinessRef,
			&hit.BranchRef,
			&hit.FoodItemRef,
			&hit.Name,
			&hit.Generation,
			&hit.ProjectionRevision,
			&hit.Rank,
		); err != nil {
			return nil, err
		}

		hits = append(
			hits,
			hit,
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return hits, nil
}

func (b *PostgresShopSearchBackend) Search(
	ctx context.Context,
	request SearchRequest,
) (
	[]ShopSearchHit,
	error,
) {
	if b == nil ||
		b.pool == nil {
		return nil, ErrInvalidArgument
	}

	request, query, err :=
		normalizeSearchRequest(
			request,
		)
	if err != nil {
		return nil, err
	}

	rows, err := b.pool.Query(
		ctx,
		`
        WITH search_query AS (
            SELECT plainto_tsquery(
                'simple'::regconfig,
                $1
            ) AS query
        )
        SELECT
            d.document_id,
            d.business_ref,
            d.branch_ref,
            d.product_ref,
            COALESCE(d.variant_ref, ''),
            COALESCE(d.listing_ref, ''),
            d.name,
            COALESCE(d.brand, ''),
            d.generation,
            d.projection_revision,
            ts_rank_cd(
                d.search_vector,
                search_query.query
            ) AS rank
          FROM shop_search_documents d
          CROSS JOIN search_query
         WHERE d.generation=$2
           AND d.active=TRUE
           AND (
               $3=''
               OR d.branch_ref=$3
           )
           AND d.search_vector @@ search_query.query
         ORDER BY
            rank DESC,
            d.normalized_name ASC,
            d.document_id ASC
         LIMIT $4
        `,
		query,
		request.Generation,
		request.BranchRef,
		request.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hits := make(
		[]ShopSearchHit,
		0,
		request.Limit,
	)

	for rows.Next() {
		var hit ShopSearchHit

		if err = rows.Scan(
			&hit.DocumentID,
			&hit.BusinessRef,
			&hit.BranchRef,
			&hit.ProductRef,
			&hit.VariantRef,
			&hit.ListingRef,
			&hit.Name,
			&hit.Brand,
			&hit.Generation,
			&hit.ProjectionRevision,
			&hit.Rank,
		); err != nil {
			return nil, err
		}

		hits = append(
			hits,
			hit,
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return hits, nil
}

var (
	_ FoodSearchBackend = (*PostgresFoodSearchBackend)(nil)

	_ ShopSearchBackend = (*PostgresShopSearchBackend)(nil)
)
