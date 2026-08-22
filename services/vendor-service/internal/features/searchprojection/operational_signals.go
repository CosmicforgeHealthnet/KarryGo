package searchprojection

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

type FoodAvailabilityStatus string

const (
	FoodAvailabilityAvailable FoodAvailabilityStatus = "AVAILABLE"

	FoodAvailabilityUnavailable FoodAvailabilityStatus = "UNAVAILABLE"

	FoodAvailabilityScheduled FoodAvailabilityStatus = "SCHEDULED"

	FoodAvailabilityCapacityLimited FoodAvailabilityStatus = "CAPACITY_LIMITED"

	FoodAvailabilityUnknown FoodAvailabilityStatus = "UNKNOWN"
)

type ShopInventoryStatus string

const (
	ShopInventoryInStock ShopInventoryStatus = "IN_STOCK"

	ShopInventoryLowStock ShopInventoryStatus = "LOW_STOCK"

	ShopInventoryPreorder ShopInventoryStatus = "PREORDER"

	ShopInventoryBackorder ShopInventoryStatus = "BACKORDER"

	ShopInventoryOutOfStock ShopInventoryStatus = "OUT_OF_STOCK"

	ShopInventoryUnknown ShopInventoryStatus = "UNKNOWN"
)

type ServiceabilityStatus string

const (
	ServiceabilityServiceable ServiceabilityStatus = "SERVICEABLE"

	ServiceabilityNotServiceable ServiceabilityStatus = "NOT_SERVICEABLE"

	ServiceabilityUnknown ServiceabilityStatus = "UNKNOWN"
)

type SearchTrustStatus string

const (
	SearchTrustVerified SearchTrustStatus = "VERIFIED"

	SearchTrustNotVerified SearchTrustStatus = "NOT_VERIFIED"

	SearchTrustSuspended SearchTrustStatus = "SUSPENDED"

	SearchTrustUnknown SearchTrustStatus = "UNKNOWN"
)

type SearchReliabilityStatus string

const (
	SearchReliabilityKnown SearchReliabilityStatus = "KNOWN"

	SearchReliabilityUnknown SearchReliabilityStatus = "UNKNOWN"
)

type SearchSourceEvidence struct {
	CatalogueRevision int64 `json:"catalogue_revision"`

	AvailabilityRevision int64 `json:"availability_revision,omitempty"`
	InventoryRevision    int64 `json:"inventory_revision,omitempty"`

	ServiceabilityRevision int64 `json:"serviceability_revision,omitempty"`
	TrustRevision          int64 `json:"trust_revision,omitempty"`
	ReliabilityRevision    int64 `json:"reliability_revision,omitempty"`
	CommercialRevision     int64 `json:"commercial_revision,omitempty"`

	ProjectionRevision int64 `json:"projection_revision"`

	AvailabilitySource string `json:"availability_source,omitempty"`
	InventorySource    string `json:"inventory_source,omitempty"`

	ServiceabilitySource string `json:"serviceability_source,omitempty"`
	TrustSource          string `json:"trust_source,omitempty"`
	ReliabilitySource    string `json:"reliability_source,omitempty"`
	CommercialSource     string `json:"commercial_source,omitempty"`
}

type ReliabilitySignal struct {
	Status SearchReliabilityStatus

	Source   string
	Revision int64

	CompletionBasisPoints   *int
	CancellationBasisPoints *int
	SLABasisPoints          *int
}

type CommercialSignal struct {
	Source   string
	Revision int64

	ConversionBasisPoints *int
	QualityBasisPoints    *int

	PromotionIndicator bool
}

type FoodOperationalSignals struct {
	Generation int64
	DocumentID string

	AvailabilityStatus   FoodAvailabilityStatus
	AvailabilitySource   string
	AvailabilityRevision int64

	ServiceabilityStatus   ServiceabilityStatus
	ServiceabilitySource   string
	ServiceabilityRevision int64

	TrustStatus   SearchTrustStatus
	TrustSource   string
	TrustRevision int64

	Reliability ReliabilitySignal

	Commercial CommercialSignal
}

type ShopOperationalSignals struct {
	Generation int64
	DocumentID string

	InventoryStatus   ShopInventoryStatus
	InventorySource   string
	InventoryRevision int64

	ServiceabilityStatus   ServiceabilityStatus
	ServiceabilitySource   string
	ServiceabilityRevision int64

	TrustStatus   SearchTrustStatus
	TrustSource   string
	TrustRevision int64

	Reliability ReliabilitySignal

	Commercial CommercialSignal
}

type OperationalSignalApplyResult struct {
	Applied bool

	ProjectionRevision int64
}

func validFoodAvailabilityStatus(
	status FoodAvailabilityStatus,
) bool {
	switch status {
	case FoodAvailabilityAvailable,
		FoodAvailabilityUnavailable,
		FoodAvailabilityScheduled,
		FoodAvailabilityCapacityLimited,
		FoodAvailabilityUnknown:
		return true

	default:
		return false
	}
}

func validShopInventoryStatus(
	status ShopInventoryStatus,
) bool {
	switch status {
	case ShopInventoryInStock,
		ShopInventoryLowStock,
		ShopInventoryPreorder,
		ShopInventoryBackorder,
		ShopInventoryOutOfStock,
		ShopInventoryUnknown:
		return true

	default:
		return false
	}
}

func validServiceabilityStatus(
	status ServiceabilityStatus,
) bool {
	switch status {
	case ServiceabilityServiceable,
		ServiceabilityNotServiceable,
		ServiceabilityUnknown:
		return true

	default:
		return false
	}
}

func validTrustStatus(
	status SearchTrustStatus,
) bool {
	switch status {
	case SearchTrustVerified,
		SearchTrustNotVerified,
		SearchTrustSuspended,
		SearchTrustUnknown:
		return true

	default:
		return false
	}
}

func validReliabilityStatus(
	status SearchReliabilityStatus,
) bool {
	switch status {
	case SearchReliabilityKnown,
		SearchReliabilityUnknown:
		return true

	default:
		return false
	}
}

func validBasisPoints(
	value *int,
) bool {
	return value == nil ||
		(*value >= 0 &&
			*value <= 10000)
}

func foodAvailableBool(
	status FoodAvailabilityStatus,
) any {
	switch status {
	case FoodAvailabilityAvailable,
		FoodAvailabilityCapacityLimited:
		return true

	case FoodAvailabilityUnavailable:
		return false

	default:
		return nil
	}
}

func shopAvailableBool(
	status ShopInventoryStatus,
) any {
	switch status {
	case ShopInventoryInStock,
		ShopInventoryLowStock:
		return true

	case ShopInventoryOutOfStock:
		return false

	default:
		return nil
	}
}

func serviceableBool(
	status ServiceabilityStatus,
) any {
	switch status {
	case ServiceabilityServiceable:
		return true

	case ServiceabilityNotServiceable:
		return false

	default:
		return nil
	}
}

func trustedBool(
	status SearchTrustStatus,
) any {
	switch status {
	case SearchTrustVerified:
		return true

	case SearchTrustNotVerified,
		SearchTrustSuspended:
		return false

	default:
		return nil
	}
}

// The five basis-point parameters below are nullable, and every @name
// reference to them is cast explicitly.
//
// Without the cast, PostgreSQL cannot infer a parameter's type when its only
// appearances are inside jsonb_build_object and COALESCE and the value is
// NULL, and the statement fails with "could not determine data type". That is
// not an edge case here: Section 13F requires an unknown reliability signal to
// stay unknown, so NULL basis points are the NORMAL shape for any document
// whose reliability the platform has never measured - which, today, is all of
// them. The casts are what let "we do not know" be stored at all.

func validateFoodOperationalSignals(
	input FoodOperationalSignals,
) error {
	if input.Generation <= 0 ||
		strings.TrimSpace(input.DocumentID) == "" ||
		!validFoodAvailabilityStatus(
			input.AvailabilityStatus,
		) ||
		!validServiceabilityStatus(
			input.ServiceabilityStatus,
		) ||
		!validTrustStatus(
			input.TrustStatus,
		) ||
		!validReliabilityStatus(
			input.Reliability.Status,
		) ||
		input.AvailabilityRevision < 0 ||
		input.ServiceabilityRevision < 0 ||
		input.TrustRevision < 0 ||
		input.Reliability.Revision < 0 ||
		input.Commercial.Revision < 0 ||
		!validBasisPoints(
			input.Reliability.CompletionBasisPoints,
		) ||
		!validBasisPoints(
			input.Reliability.CancellationBasisPoints,
		) ||
		!validBasisPoints(
			input.Reliability.SLABasisPoints,
		) ||
		!validBasisPoints(
			input.Commercial.ConversionBasisPoints,
		) ||
		!validBasisPoints(
			input.Commercial.QualityBasisPoints,
		) {
		return ErrInvalidArgument
	}

	return nil
}

func validateShopOperationalSignals(
	input ShopOperationalSignals,
) error {
	if input.Generation <= 0 ||
		strings.TrimSpace(input.DocumentID) == "" ||
		!validShopInventoryStatus(
			input.InventoryStatus,
		) ||
		!validServiceabilityStatus(
			input.ServiceabilityStatus,
		) ||
		!validTrustStatus(
			input.TrustStatus,
		) ||
		!validReliabilityStatus(
			input.Reliability.Status,
		) ||
		input.InventoryRevision <= 0 ||
		input.ServiceabilityRevision < 0 ||
		input.TrustRevision < 0 ||
		input.Reliability.Revision < 0 ||
		input.Commercial.Revision < 0 ||
		!validBasisPoints(
			input.Reliability.CompletionBasisPoints,
		) ||
		!validBasisPoints(
			input.Reliability.CancellationBasisPoints,
		) ||
		!validBasisPoints(
			input.Reliability.SLABasisPoints,
		) ||
		!validBasisPoints(
			input.Commercial.ConversionBasisPoints,
		) ||
		!validBasisPoints(
			input.Commercial.QualityBasisPoints,
		) {
		return ErrInvalidArgument
	}

	return nil
}

func (
	backend *PostgresFoodSearchBackend,
) ApplyOperationalSignals(
	ctx context.Context,
	input FoodOperationalSignals,
) (
	OperationalSignalApplyResult,
	error,
) {
	if backend == nil ||
		backend.pool == nil {
		return OperationalSignalApplyResult{},
			ErrInvalidArgument
	}

	if err :=
		validateFoodOperationalSignals(
			input,
		); err != nil {
		return OperationalSignalApplyResult{},
			err
	}

	row := backend.pool.QueryRow(
		ctx,
		`
        UPDATE food_search_documents
           SET
               availability_status =
                   CASE
                       WHEN @availability_revision::bigint > availability_revision
                           THEN @availability_status::text
                       ELSE availability_status
                   END,

               availability_summary =
                   CASE
                       WHEN @availability_revision::bigint > availability_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'status', @availability_status::text,
                                   'available', @availability_bool::boolean,
                                   'source', NULLIF(@availability_source::text, ''),
                                   'source_revision', @availability_revision::bigint
                               )
                           )
                       ELSE availability_summary
                   END,

               availability_revision =
                   greatest(
                       availability_revision,
                       @availability_revision::bigint
                   ),

               serviceability_status =
                   CASE
                       WHEN @serviceability_revision::bigint > serviceability_revision
                           THEN @serviceability_status::text
                       ELSE serviceability_status
                   END,

               serviceability_hint =
                   CASE
                       WHEN @serviceability_revision::bigint > serviceability_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'status', @serviceability_status::text,
                                   'serviceable', @serviceable_bool::boolean,
                                   'source', NULLIF(@serviceability_source::text, ''),
                                   'source_revision', @serviceability_revision::bigint
                               )
                           )
                       ELSE serviceability_hint
                   END,

               serviceability_revision =
                   greatest(
                       serviceability_revision,
                       @serviceability_revision::bigint
                   ),

               trust_status =
                   CASE
                       WHEN @trust_revision::bigint > trust_revision
                           THEN @trust_status::text
                       ELSE trust_status
                   END,

               trust_hint =
                   CASE
                       WHEN @trust_revision::bigint > trust_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'status', @trust_status::text,
                                   'trusted', @trusted_bool::boolean,
                                   'source', NULLIF(@trust_source::text, ''),
                                   'source_revision', @trust_revision::bigint
                               )
                           )
                       ELSE trust_hint
                   END,

               trust_revision =
                   greatest(
                       trust_revision,
                       @trust_revision::bigint
                   ),

               reliability_status =
                   CASE
                       WHEN @reliability_revision::bigint > reliability_revision
                           THEN @reliability_status::text
                       ELSE reliability_status
                   END,

               reliability_hint =
                   reliability_hint

                   ||

                   CASE
                       WHEN @reliability_revision::bigint > reliability_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'status', @reliability_status::text,
                                   'source', NULLIF(@reliability_source::text, ''),
                                   'source_revision', @reliability_revision::bigint,
                                   'completion_reliability_bps', @completion_bps::integer,
                                   'cancellation_reliability_bps', @cancellation_bps::integer,
                                   'sla_reliability_bps', @sla_bps::integer,
                                   'reliability_bps',
                                       CASE
                                           WHEN @reliability_status::text='KNOWN'
                                               THEN greatest(
                                                   0,
                                                   least(
                                                       10000,
                                                       (
                                                           COALESCE(@completion_bps::integer, 0)
                                                           + COALESCE(@cancellation_bps::integer, 0)
                                                           + COALESCE(@sla_bps::integer, 0)
                                                       )
                                                       /
                                                       greatest(
                                                           1,
                                                           (
                                                               CASE WHEN @completion_bps::integer IS NULL THEN 0 ELSE 1 END
                                                               + CASE WHEN @cancellation_bps::integer IS NULL THEN 0 ELSE 1 END
                                                               + CASE WHEN @sla_bps::integer IS NULL THEN 0 ELSE 1 END
                                                           )
                                                       )
                                                   )
                                               )
                                           ELSE NULL
                                       END
                               )
                           )
                       ELSE '{}'::jsonb
                   END

                   ||

                   CASE
                       WHEN @commercial_revision::bigint > commercial_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'commercial_source', NULLIF(@commercial_source::text, ''),
                                   'commercial_source_revision', @commercial_revision::bigint,
                                   'commercial_score_bps', @conversion_bps::integer,
                                   'quality_score_bps', @quality_bps::integer
                               )
                           )
                       ELSE '{}'::jsonb
                   END,

               reliability_revision =
                   greatest(
                       reliability_revision,
                       @reliability_revision::bigint
                   ),

               commercial_signal =
                   CASE
                       WHEN @commercial_revision::bigint > commercial_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'source', NULLIF(@commercial_source::text, ''),
                                   'source_revision', @commercial_revision::bigint,
                                   'conversion_bps', @conversion_bps::integer,
                                   'quality_bps', @quality_bps::integer,
                                   'promotion_indicator', @promotion_indicator::boolean
                               )
                           )
                       ELSE commercial_signal
                   END,

               promotion_indicator =
                   CASE
                       WHEN @commercial_revision::bigint > commercial_revision
                           THEN @promotion_indicator::boolean
                       ELSE promotion_indicator
                   END,

               commercial_revision =
                   greatest(
                       commercial_revision,
                       @commercial_revision::bigint
                   ),

               projection_revision =
                   projection_revision + 1,

               updated_at = now()

         WHERE generation=@generation::bigint
           AND document_id=@document_id::text
           AND (
               @availability_revision::bigint > availability_revision
               OR @serviceability_revision::bigint > serviceability_revision
               OR @trust_revision::bigint > trust_revision
               OR @reliability_revision::bigint > reliability_revision
               OR @commercial_revision::bigint > commercial_revision
           )

        RETURNING projection_revision
        `,
		pgx.NamedArgs{
			"generation": input.Generation,

			"document_id": input.DocumentID,

			"availability_status": string(input.AvailabilityStatus),

			"availability_source": strings.TrimSpace(
				input.AvailabilitySource,
			),

			"availability_revision": input.AvailabilityRevision,

			"availability_bool": foodAvailableBool(
				input.AvailabilityStatus,
			),

			"serviceability_status": string(input.ServiceabilityStatus),

			"serviceability_source": strings.TrimSpace(
				input.ServiceabilitySource,
			),

			"serviceability_revision": input.ServiceabilityRevision,

			"serviceable_bool": serviceableBool(
				input.ServiceabilityStatus,
			),

			"trust_status": string(input.TrustStatus),

			"trust_source": strings.TrimSpace(
				input.TrustSource,
			),

			"trust_revision": input.TrustRevision,

			"trusted_bool": trustedBool(
				input.TrustStatus,
			),

			"reliability_status": string(input.Reliability.Status),

			"reliability_source": strings.TrimSpace(
				input.Reliability.Source,
			),

			"reliability_revision": input.Reliability.Revision,

			"completion_bps": input.Reliability.CompletionBasisPoints,

			"cancellation_bps": input.Reliability.CancellationBasisPoints,

			"sla_bps": input.Reliability.SLABasisPoints,

			"commercial_source": strings.TrimSpace(
				input.Commercial.Source,
			),

			"commercial_revision": input.Commercial.Revision,

			"conversion_bps": input.Commercial.ConversionBasisPoints,

			"quality_bps": input.Commercial.QualityBasisPoints,

			"promotion_indicator": input.Commercial.PromotionIndicator,
		},
	)

	var projectionRevision int64

	if err := row.Scan(
		&projectionRevision,
	); err == nil {
		return OperationalSignalApplyResult{
			Applied: true,

			ProjectionRevision: projectionRevision,
		}, nil
	} else if err != pgx.ErrNoRows {
		return OperationalSignalApplyResult{},
			err
	}

	err := backend.pool.QueryRow(
		ctx,
		`
        SELECT projection_revision
          FROM food_search_documents
         WHERE generation=$1
           AND document_id=$2
        `,
		input.Generation,
		input.DocumentID,
	).Scan(
		&projectionRevision,
	)
	if err != nil {
		return OperationalSignalApplyResult{},
			err
	}

	return OperationalSignalApplyResult{
		Applied: false,

		ProjectionRevision: projectionRevision,
	}, nil
}

func (
	backend *PostgresShopSearchBackend,
) ApplyOperationalSignals(
	ctx context.Context,
	input ShopOperationalSignals,
) (
	OperationalSignalApplyResult,
	error,
) {
	if backend == nil ||
		backend.pool == nil {
		return OperationalSignalApplyResult{},
			ErrInvalidArgument
	}

	if err :=
		validateShopOperationalSignals(
			input,
		); err != nil {
		return OperationalSignalApplyResult{},
			err
	}

	row := backend.pool.QueryRow(
		ctx,
		`
        UPDATE shop_search_documents
           SET
               inventory_status =
                   CASE
                       WHEN @inventory_revision::bigint > inventory_revision
                           THEN @inventory_status::text
                       ELSE inventory_status
                   END,

               inventory_availability_summary =
                   CASE
                       WHEN @inventory_revision::bigint > inventory_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'status', @inventory_status::text,
                                   'available', @inventory_available::boolean,
                                   'source', NULLIF(@inventory_source::text, ''),
                                   'source_revision', @inventory_revision::bigint
                               )
                           )
                       ELSE inventory_availability_summary
                   END,

               inventory_revision =
                   greatest(
                       inventory_revision,
                       @inventory_revision::bigint
                   ),

               serviceability_status =
                   CASE
                       WHEN @serviceability_revision::bigint > serviceability_revision
                           THEN @serviceability_status::text
                       ELSE serviceability_status
                   END,

               handling_metadata =
                   handling_metadata

                   ||

                   CASE
                       WHEN @serviceability_revision::bigint > serviceability_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'serviceability_status', @serviceability_status::text,
                                   'serviceable', @serviceable_bool::boolean,
                                   'serviceability_source', NULLIF(@serviceability_source::text, ''),
                                   'serviceability_source_revision', @serviceability_revision::bigint
                               )
                           )
                       ELSE '{}'::jsonb
                   END

                   ||

                   CASE
                       WHEN @trust_revision::bigint > trust_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'trust_status', @trust_status::text,
                                   'trusted', @trusted_bool::boolean,
                                   'trust_source', NULLIF(@trust_source::text, ''),
                                   'trust_source_revision', @trust_revision::bigint
                               )
                           )
                       ELSE '{}'::jsonb
                   END

                   ||

                   CASE
                       WHEN @reliability_revision::bigint > reliability_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'reliability_status', @reliability_status::text,
                                   'reliability_source', NULLIF(@reliability_source::text, ''),
                                   'reliability_source_revision', @reliability_revision::bigint,
                                   'completion_reliability_bps', @completion_bps::integer,
                                   'cancellation_reliability_bps', @cancellation_bps::integer,
                                   'sla_reliability_bps', @sla_bps::integer,
                                   'reliability_bps',
                                       CASE
                                           WHEN @reliability_status::text='KNOWN'
                                               THEN greatest(
                                                   0,
                                                   least(
                                                       10000,
                                                       (
                                                           COALESCE(@completion_bps::integer, 0)
                                                           + COALESCE(@cancellation_bps::integer, 0)
                                                           + COALESCE(@sla_bps::integer, 0)
                                                       )
                                                       /
                                                       greatest(
                                                           1,
                                                           (
                                                               CASE WHEN @completion_bps::integer IS NULL THEN 0 ELSE 1 END
                                                               + CASE WHEN @cancellation_bps::integer IS NULL THEN 0 ELSE 1 END
                                                               + CASE WHEN @sla_bps::integer IS NULL THEN 0 ELSE 1 END
                                                           )
                                                       )
                                                   )
                                               )
                                           ELSE NULL
                                       END
                               )
                           )
                       ELSE '{}'::jsonb
                   END

                   ||

                   CASE
                       WHEN @commercial_revision::bigint > commercial_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'commercial_source', NULLIF(@commercial_source::text, ''),
                                   'commercial_source_revision', @commercial_revision::bigint,
                                   'commercial_score_bps', @conversion_bps::integer,
                                   'quality_score_bps', @quality_bps::integer
                               )
                           )
                       ELSE '{}'::jsonb
                   END,

               serviceability_revision =
                   greatest(
                       serviceability_revision,
                       @serviceability_revision::bigint
                   ),

               trust_status =
                   CASE
                       WHEN @trust_revision::bigint > trust_revision
                           THEN @trust_status::text
                       ELSE trust_status
                   END,

               trust_revision =
                   greatest(
                       trust_revision,
                       @trust_revision::bigint
                   ),

               reliability_status =
                   CASE
                       WHEN @reliability_revision::bigint > reliability_revision
                           THEN @reliability_status::text
                       ELSE reliability_status
                   END,

               reliability_revision =
                   greatest(
                       reliability_revision,
                       @reliability_revision::bigint
                   ),

               commercial_signal =
                   CASE
                       WHEN @commercial_revision::bigint > commercial_revision
                           THEN jsonb_strip_nulls(
                               jsonb_build_object(
                                   'source', NULLIF(@commercial_source::text, ''),
                                   'source_revision', @commercial_revision::bigint,
                                   'conversion_bps', @conversion_bps::integer,
                                   'quality_bps', @quality_bps::integer,
                                   'promotion_indicator', @promotion_indicator::boolean
                               )
                           )
                       ELSE commercial_signal
                   END,

               promotion_indicator =
                   CASE
                       WHEN @commercial_revision::bigint > commercial_revision
                           THEN @promotion_indicator::boolean
                       ELSE promotion_indicator
                   END,

               commercial_revision =
                   greatest(
                       commercial_revision,
                       @commercial_revision::bigint
                   ),

               projection_revision =
                   projection_revision + 1,

               updated_at = now()

         WHERE generation=@generation::bigint
           AND document_id=@document_id::text
           AND (
               @inventory_revision::bigint > inventory_revision
               OR @serviceability_revision::bigint > serviceability_revision
               OR @trust_revision::bigint > trust_revision
               OR @reliability_revision::bigint > reliability_revision
               OR @commercial_revision::bigint > commercial_revision
           )

        RETURNING projection_revision
        `,
		pgx.NamedArgs{
			"generation": input.Generation,

			"document_id": input.DocumentID,

			"inventory_status": string(input.InventoryStatus),

			"inventory_source": strings.TrimSpace(
				input.InventorySource,
			),

			"inventory_revision": input.InventoryRevision,

			"inventory_available": shopAvailableBool(
				input.InventoryStatus,
			),

			"serviceability_status": string(input.ServiceabilityStatus),

			"serviceability_source": strings.TrimSpace(
				input.ServiceabilitySource,
			),

			"serviceability_revision": input.ServiceabilityRevision,

			"serviceable_bool": serviceableBool(
				input.ServiceabilityStatus,
			),

			"trust_status": string(input.TrustStatus),

			"trust_source": strings.TrimSpace(
				input.TrustSource,
			),

			"trust_revision": input.TrustRevision,

			"trusted_bool": trustedBool(
				input.TrustStatus,
			),

			"reliability_status": string(input.Reliability.Status),

			"reliability_source": strings.TrimSpace(
				input.Reliability.Source,
			),

			"reliability_revision": input.Reliability.Revision,

			"completion_bps": input.Reliability.CompletionBasisPoints,

			"cancellation_bps": input.Reliability.CancellationBasisPoints,

			"sla_bps": input.Reliability.SLABasisPoints,

			"commercial_source": strings.TrimSpace(
				input.Commercial.Source,
			),

			"commercial_revision": input.Commercial.Revision,

			"conversion_bps": input.Commercial.ConversionBasisPoints,

			"quality_bps": input.Commercial.QualityBasisPoints,

			"promotion_indicator": input.Commercial.PromotionIndicator,
		},
	)

	var projectionRevision int64

	if err := row.Scan(
		&projectionRevision,
	); err == nil {
		return OperationalSignalApplyResult{
			Applied: true,

			ProjectionRevision: projectionRevision,
		}, nil
	} else if err != pgx.ErrNoRows {
		return OperationalSignalApplyResult{},
			err
	}

	err := backend.pool.QueryRow(
		ctx,
		`
        SELECT projection_revision
          FROM shop_search_documents
         WHERE generation=$1
           AND document_id=$2
        `,
		input.Generation,
		input.DocumentID,
	).Scan(
		&projectionRevision,
	)
	if err != nil {
		return OperationalSignalApplyResult{},
			err
	}

	return OperationalSignalApplyResult{
		Applied: false,

		ProjectionRevision: projectionRevision,
	}, nil
}

func rawSignalString(
	raw json.RawMessage,
	keys ...string,
) (
	string,
	bool,
) {
	object := jsonObject(raw)

	for _, key := range keys {
		value, exists :=
			object[key]

		if !exists {
			continue
		}

		text, ok :=
			value.(string)

		if ok &&
			strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text),
				true
		}
	}

	return "", false
}

func rawSignalRevision(
	raw json.RawMessage,
	keys ...string,
) int64 {
	value, ok :=
		jsonInt64(
			raw,
			keys...,
		)

	if !ok ||
		value < 0 {
		return 0
	}

	return value
}

func foodAvailabilityStatusFromDocument(
	document FoodEngineDocument,
) FoodAvailabilityStatus {
	value, ok :=
		rawSignalString(
			document.AvailabilitySummary,
			"status",
		)

	if !ok {
		return FoodAvailabilityUnknown
	}

	status :=
		FoodAvailabilityStatus(
			strings.ToUpper(value),
		)

	if !validFoodAvailabilityStatus(status) {
		return FoodAvailabilityUnknown
	}

	return status
}

func shopInventoryStatusFromDocument(
	document ShopEngineDocument,
) ShopInventoryStatus {
	value, ok :=
		rawSignalString(
			document.InventoryAvailabilitySummary,
			"status",
		)

	if !ok {
		return ShopInventoryUnknown
	}

	status :=
		ShopInventoryStatus(
			strings.ToUpper(value),
		)

	if !validShopInventoryStatus(status) {
		return ShopInventoryUnknown
	}

	return status
}

func foodServiceabilityStatusFromDocument(
	document FoodEngineDocument,
) ServiceabilityStatus {
	value, ok :=
		rawSignalString(
			document.ServiceabilityHint,
			"status",
		)

	if !ok {
		return ServiceabilityUnknown
	}

	status :=
		ServiceabilityStatus(
			strings.ToUpper(value),
		)

	if !validServiceabilityStatus(status) {
		return ServiceabilityUnknown
	}

	return status
}

func shopServiceabilityStatusFromDocument(
	document ShopEngineDocument,
) ServiceabilityStatus {
	value, ok :=
		rawSignalString(
			document.HandlingMetadata,
			"serviceability_status",
		)

	if !ok {
		return ServiceabilityUnknown
	}

	status :=
		ServiceabilityStatus(
			strings.ToUpper(value),
		)

	if !validServiceabilityStatus(status) {
		return ServiceabilityUnknown
	}

	return status
}

func buildFoodSearchSourceEvidence(
	document FoodEngineDocument,
) SearchSourceEvidence {
	availabilitySource, _ :=
		rawSignalString(
			document.AvailabilitySummary,
			"source",
		)

	serviceabilitySource, _ :=
		rawSignalString(
			document.ServiceabilityHint,
			"source",
		)

	trustSource, _ :=
		rawSignalString(
			document.TrustHint,
			"source",
		)

	reliabilitySource, _ :=
		rawSignalString(
			document.ReliabilityHint,
			"source",
		)

	commercialSource, _ :=
		rawSignalString(
			document.ReliabilityHint,
			"commercial_source",
		)

	return SearchSourceEvidence{
		CatalogueRevision: document.CatalogueRevision,

		AvailabilityRevision: rawSignalRevision(
			document.AvailabilitySummary,
			"source_revision",
		),

		ServiceabilityRevision: rawSignalRevision(
			document.ServiceabilityHint,
			"source_revision",
		),

		TrustRevision: rawSignalRevision(
			document.TrustHint,
			"source_revision",
		),

		ReliabilityRevision: rawSignalRevision(
			document.ReliabilityHint,
			"source_revision",
		),

		CommercialRevision: rawSignalRevision(
			document.ReliabilityHint,
			"commercial_source_revision",
		),

		ProjectionRevision: document.ProjectionRevision,

		AvailabilitySource: availabilitySource,

		ServiceabilitySource: serviceabilitySource,

		TrustSource: trustSource,

		ReliabilitySource: reliabilitySource,

		CommercialSource: commercialSource,
	}
}

func buildShopSearchSourceEvidence(
	document ShopEngineDocument,
) SearchSourceEvidence {
	inventorySource, _ :=
		rawSignalString(
			document.InventoryAvailabilitySummary,
			"source",
		)

	serviceabilitySource, _ :=
		rawSignalString(
			document.HandlingMetadata,
			"serviceability_source",
		)

	trustSource, _ :=
		rawSignalString(
			document.HandlingMetadata,
			"trust_source",
		)

	reliabilitySource, _ :=
		rawSignalString(
			document.HandlingMetadata,
			"reliability_source",
		)

	commercialSource, _ :=
		rawSignalString(
			document.HandlingMetadata,
			"commercial_source",
		)

	return SearchSourceEvidence{
		CatalogueRevision: document.CatalogueRevision,

		InventoryRevision: document.InventoryRevision,

		ServiceabilityRevision: rawSignalRevision(
			document.HandlingMetadata,
			"serviceability_source_revision",
		),

		TrustRevision: rawSignalRevision(
			document.HandlingMetadata,
			"trust_source_revision",
		),

		ReliabilityRevision: rawSignalRevision(
			document.HandlingMetadata,
			"reliability_source_revision",
		),

		CommercialRevision: rawSignalRevision(
			document.HandlingMetadata,
			"commercial_source_revision",
		),

		ProjectionRevision: document.ProjectionRevision,

		InventorySource: inventorySource,

		ServiceabilitySource: serviceabilitySource,

		TrustSource: trustSource,

		ReliabilitySource: reliabilitySource,

		CommercialSource: commercialSource,
	}
}
