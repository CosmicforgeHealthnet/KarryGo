-- Reverses 000121 by restoring the seven-value capability vocabulary.
--
-- It refuses rather than deletes. 000120 could remove its own event rows to
-- restore a narrower CHECK, because an event is a record of something the
-- system did. A capability row is a business's livelihood: a caterer onboarded
-- while this migration was applied would lose its capability, its branch
-- capabilities and its CRM history to make a rollback pass quietly.
--
-- So if any Catering or Manufacturer data exists, this fails with a message
-- naming what to do about it, and the operator decides. With no such rows the
-- rollback is exact.

DO $$
DECLARE
    offending bigint;
BEGIN
    SELECT count(*) INTO offending FROM vendor_capabilities
     WHERE capability_type IN ('CATERING','MANUFACTURER');
    IF offending > 0 THEN
        RAISE EXCEPTION
            'cannot roll back 000121: % vendor_capabilities rows are CATERING or MANUFACTURER. '
            'Retire those capabilities through the Vendor API first; this migration will not delete them.',
            offending;
    END IF;

    SELECT count(*) INTO offending FROM vendor_compliance_requirements
     WHERE capability_type IN ('CATERING','MANUFACTURER');
    IF offending > 0 THEN
        RAISE EXCEPTION
            'cannot roll back 000121: % vendor_compliance_requirements rows reference CATERING or MANUFACTURER.',
            offending;
    END IF;

    SELECT count(*) INTO offending FROM crm_customer_branch_relations
     WHERE last_capability_type IN ('CATERING','MANUFACTURER');
    IF offending > 0 THEN
        RAISE EXCEPTION
            'cannot roll back 000121: % crm_customer_branch_relations rows record a CATERING or MANUFACTURER order.',
            offending;
    END IF;

    SELECT count(*) INTO offending FROM crm_customer_order_facts
     WHERE capability_type IN ('CATERING','MANUFACTURER');
    IF offending > 0 THEN
        RAISE EXCEPTION
            'cannot roll back 000121: % crm_customer_order_facts rows record a CATERING or MANUFACTURER order.',
            offending;
    END IF;

    SELECT count(*) INTO offending FROM vendor_customer_profiles
     WHERE last_capability_type IN ('CATERING','MANUFACTURER');
    IF offending > 0 THEN
        RAISE EXCEPTION
            'cannot roll back 000121: % vendor_customer_profiles rows record a CATERING or MANUFACTURER order.',
            offending;
    END IF;
END $$;

ALTER TABLE vendor_capabilities
    DROP CONSTRAINT IF EXISTS vendor_capabilities_capability_type_check;
ALTER TABLE vendor_capabilities
    ADD CONSTRAINT vendor_capabilities_capability_type_check
    CHECK (capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE'));

ALTER TABLE vendor_compliance_requirements
    DROP CONSTRAINT IF EXISTS vendor_compliance_requirements_capability_type_check;
ALTER TABLE vendor_compliance_requirements
    ADD CONSTRAINT vendor_compliance_requirements_capability_type_check
    CHECK (capability_type IS NULL OR capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE'));

ALTER TABLE crm_customer_branch_relations
    DROP CONSTRAINT IF EXISTS crm_customer_branch_relations_last_capability_type_check;
ALTER TABLE crm_customer_branch_relations
    ADD CONSTRAINT crm_customer_branch_relations_last_capability_type_check
    CHECK (last_capability_type IS NULL OR last_capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE'));

ALTER TABLE crm_customer_order_facts
    DROP CONSTRAINT IF EXISTS crm_customer_order_facts_capability_type_check;
ALTER TABLE crm_customer_order_facts
    ADD CONSTRAINT crm_customer_order_facts_capability_type_check
    CHECK (capability_type IS NULL OR capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE'));

ALTER TABLE vendor_customer_profiles
    DROP CONSTRAINT IF EXISTS vendor_customer_profiles_last_capability_type_check;
ALTER TABLE vendor_customer_profiles
    ADD CONSTRAINT vendor_customer_profiles_last_capability_type_check
    CHECK (last_capability_type IS NULL OR last_capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE'));
