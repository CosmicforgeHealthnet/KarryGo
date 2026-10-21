DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM fleet_partner_provider_offer_decision_receipts)
 THEN RAISE EXCEPTION 'Provider decision evidence requires an approved forward retention migration'; END IF;
END $$;
DROP TABLE fleet_partner_provider_offer_decision_receipts;
DROP FUNCTION reject_provider_offer_receipt_mutation();
