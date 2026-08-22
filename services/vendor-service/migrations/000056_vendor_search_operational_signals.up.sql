-- Section 13F
-- Operational discovery signals.
--
-- Search remains non-authoritative.
--
-- SEARCH RESULT != PRICE GUARANTEE
-- SEARCH RESULT != STOCK RESERVATION
-- SEARCH RESULT != CHECKOUT AUTHORIZATION
--
-- Checkout/order flows must revalidate authoritative domain state.

ALTER TABLE food_search_documents
    ADD COLUMN availability_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            availability_status IN (
                'AVAILABLE',
                'UNAVAILABLE',
                'SCHEDULED',
                'CAPACITY_LIMITED',
                'UNKNOWN'
            )
        ),

    ADD COLUMN availability_revision BIGINT NOT NULL DEFAULT 0
        CHECK (availability_revision >= 0),

    ADD COLUMN serviceability_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            serviceability_status IN (
                'SERVICEABLE',
                'NOT_SERVICEABLE',
                'UNKNOWN'
            )
        ),

    ADD COLUMN serviceability_revision BIGINT NOT NULL DEFAULT 0
        CHECK (serviceability_revision >= 0),

    ADD COLUMN trust_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            trust_status IN (
                'VERIFIED',
                'NOT_VERIFIED',
                'SUSPENDED',
                'UNKNOWN'
            )
        ),

    ADD COLUMN trust_revision BIGINT NOT NULL DEFAULT 0
        CHECK (trust_revision >= 0),

    ADD COLUMN reliability_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            reliability_status IN (
                'KNOWN',
                'UNKNOWN'
            )
        ),

    ADD COLUMN reliability_revision BIGINT NOT NULL DEFAULT 0
        CHECK (reliability_revision >= 0),

    ADD COLUMN commercial_revision BIGINT NOT NULL DEFAULT 0
        CHECK (commercial_revision >= 0),

    ADD COLUMN commercial_signal JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(commercial_signal)='object');


ALTER TABLE shop_search_documents
    ADD COLUMN inventory_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            inventory_status IN (
                'IN_STOCK',
                'LOW_STOCK',
                'PREORDER',
                'BACKORDER',
                'OUT_OF_STOCK',
                'UNKNOWN'
            )
        ),

    ADD COLUMN serviceability_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            serviceability_status IN (
                'SERVICEABLE',
                'NOT_SERVICEABLE',
                'UNKNOWN'
            )
        ),

    ADD COLUMN serviceability_revision BIGINT NOT NULL DEFAULT 0
        CHECK (serviceability_revision >= 0),

    ADD COLUMN trust_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            trust_status IN (
                'VERIFIED',
                'NOT_VERIFIED',
                'SUSPENDED',
                'UNKNOWN'
            )
        ),

    ADD COLUMN trust_revision BIGINT NOT NULL DEFAULT 0
        CHECK (trust_revision >= 0),

    ADD COLUMN reliability_status TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (
            reliability_status IN (
                'KNOWN',
                'UNKNOWN'
            )
        ),

    ADD COLUMN reliability_revision BIGINT NOT NULL DEFAULT 0
        CHECK (reliability_revision >= 0),

    ADD COLUMN commercial_revision BIGINT NOT NULL DEFAULT 0
        CHECK (commercial_revision >= 0),

    ADD COLUMN commercial_signal JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(commercial_signal)='object'),

    ADD COLUMN promotion_indicator BOOLEAN NOT NULL DEFAULT FALSE;


CREATE INDEX food_search_documents_operational_idx
    ON food_search_documents (
        generation,
        availability_status,
        serviceability_status,
        active
    );


CREATE INDEX shop_search_documents_operational_idx
    ON shop_search_documents (
        generation,
        inventory_status,
        serviceability_status,
        active
    );


COMMENT ON COLUMN food_search_documents.availability_status IS
    'Derived Food availability signal only. Food domain remains sellability authority.';

COMMENT ON COLUMN food_search_documents.availability_revision IS
    'Revision evidence from authoritative Food availability source.';

COMMENT ON COLUMN shop_search_documents.inventory_status IS
    'Derived inventory visibility only. Section 12 inventory remains reservation authority.';

COMMENT ON COLUMN shop_search_documents.inventory_revision IS
    'Authoritative Section 12 inventory revision represented by this search projection.';

COMMENT ON COLUMN food_search_documents.serviceability_status IS
    'Derived delivery/serviceability signal. UNKNOWN means no legitimate source evidence is available.';

COMMENT ON COLUMN shop_search_documents.serviceability_status IS
    'Derived delivery/serviceability signal. UNKNOWN means no legitimate source evidence is available.';

COMMENT ON COLUMN food_search_documents.trust_status IS
    'Derived Vendor/Verification trust signal. Search is not KYB or identity authority.';

COMMENT ON COLUMN shop_search_documents.trust_status IS
    'Derived Vendor/Verification trust signal. Search is not KYB or identity authority.';

COMMENT ON COLUMN food_search_documents.reliability_status IS
    'Derived reliability signal. New/unknown vendors are never assigned a synthetic perfect score.';

COMMENT ON COLUMN shop_search_documents.reliability_status IS
    'Derived reliability signal. New/unknown vendors are never assigned a synthetic perfect score.';

COMMENT ON COLUMN food_search_documents.commercial_signal IS
    'Derived legitimate commercial/quality signal. Advertising/campaign authority is outside Section 13.';

COMMENT ON COLUMN shop_search_documents.commercial_signal IS
    'Derived legitimate commercial/quality signal. Advertising/campaign authority is outside Section 13.';