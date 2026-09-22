-- Catering and Manufacturer become configurable business capabilities.
--
-- Section 11 specifies both categories and their readiness substates, and the
-- readiness engine has carried them since 000117. What it could not do was
-- start: a business had no way to hold the capability, so a readiness row of
-- that type described an order nobody could place.
--
-- This widens the capability vocabulary and nothing else. Every existing value
-- is preserved, no row is rewritten, and no vendor's capabilities change. The
-- narrower boundaries stay narrow on purpose:
--
--   food_menus / food_catalogue_revisions   RESTAURANT and FAST_FOOD only
--   production_* / capacity_reservations    the four kitchen categories
--   shop_* operating models                 the three shop categories
--
-- A caterer is not a restaurant menu and a manufacturer is not a shop shelf;
-- admitting them to those modules would be a different decision from being
-- able to onboard them, and this migration does not make it.

ALTER TABLE vendor_capabilities
    DROP CONSTRAINT IF EXISTS vendor_capabilities_capability_type_check;
ALTER TABLE vendor_capabilities
    ADD CONSTRAINT vendor_capabilities_capability_type_check
    CHECK (capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE','CATERING','MANUFACTURER'));

-- Compliance requirements are seeded per capability. Catering and
-- Manufacturer get none here, exactly like PASTRY_BAKERY, SHOP_GROCERY,
-- MARKET and WHOLESALE: only the three kitchen categories carry a
-- BRANCH_CAPABILITY requirement today. Widening the CHECK is what lets a
-- later requirement be recorded for them at all.
ALTER TABLE vendor_compliance_requirements
    DROP CONSTRAINT IF EXISTS vendor_compliance_requirements_capability_type_check;
ALTER TABLE vendor_compliance_requirements
    ADD CONSTRAINT vendor_compliance_requirements_capability_type_check
    CHECK (capability_type IS NULL OR capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE','CATERING','MANUFACTURER'));

-- CRM projections record which capability an order was fulfilled with. They
-- must accept the new values or a caterer's first order would fail to project.
ALTER TABLE crm_customer_branch_relations
    DROP CONSTRAINT IF EXISTS crm_customer_branch_relations_last_capability_type_check;
ALTER TABLE crm_customer_branch_relations
    ADD CONSTRAINT crm_customer_branch_relations_last_capability_type_check
    CHECK (last_capability_type IS NULL OR last_capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE','CATERING','MANUFACTURER'));

ALTER TABLE crm_customer_order_facts
    DROP CONSTRAINT IF EXISTS crm_customer_order_facts_capability_type_check;
ALTER TABLE crm_customer_order_facts
    ADD CONSTRAINT crm_customer_order_facts_capability_type_check
    CHECK (capability_type IS NULL OR capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE','CATERING','MANUFACTURER'));

ALTER TABLE vendor_customer_profiles
    DROP CONSTRAINT IF EXISTS vendor_customer_profiles_last_capability_type_check;
ALTER TABLE vendor_customer_profiles
    ADD CONSTRAINT vendor_customer_profiles_last_capability_type_check
    CHECK (last_capability_type IS NULL OR last_capability_type IN (
        'RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY',
        'SHOP_GROCERY','MARKET','WHOLESALE','CATERING','MANUFACTURER'));
