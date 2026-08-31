-- Deploy reference-writing application binaries first. Drain legacy active
-- credentials before applying; never silently invalidate a still-active code.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM trips WHERE
   (pickup_code ~ '^[0-9]+$' AND pickup_code_expires_at > now() AND pickup_code_verified_at IS NULL AND status NOT IN ('completed','cancelled','failed')) OR
   (delivery_code ~ '^[0-9]+$' AND delivery_code_expires_at > now() AND delivery_code_verified_at IS NULL AND status NOT IN ('completed','cancelled','failed')))
 THEN RAISE EXCEPTION 'legacy handover credentials must drain before secret isolation migration'; END IF;
END $$;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM commerce_mission_stops WHERE handover_code ~ '^[0-9]+$' AND handover_code_expires_at>now() AND handover_status='ISSUED')
 OR EXISTS (SELECT 1 FROM commerce_mission_operations o JOIN commerce_provider_transfers t ON t.transfer_ref=o.result_snapshot->>'transfer_ref' WHERE o.operation_type='START_PROVIDER_TRANSFER' AND o.result_snapshot->>'challenge_code' ~ '^[0-9]+$' AND t.challenge_expires_at>now() AND t.state='CHALLENGE_ISSUED')
 THEN RAISE EXCEPTION 'legacy Commerce handover credentials must drain before secret isolation migration'; END IF;
END $$;
UPDATE trips SET pickup_code='' WHERE pickup_code<>'' AND pickup_code !~ '^handover_[A-Za-z0-9_-]{24}$';
UPDATE trips SET delivery_code='' WHERE delivery_code<>'' AND delivery_code !~ '^handover_[A-Za-z0-9_-]{24}$';
ALTER TABLE trips ADD CONSTRAINT trips_pickup_reference_only CHECK (pickup_code='' OR pickup_code ~ '^handover_[A-Za-z0-9_-]{24}$');
ALTER TABLE trips ADD CONSTRAINT trips_delivery_reference_only CHECK (delivery_code='' OR delivery_code ~ '^handover_[A-Za-z0-9_-]{24}$');
UPDATE commerce_mission_stops SET handover_code='' WHERE handover_code<>'' AND handover_code !~ '^handover_[A-Za-z0-9_-]{24}$';
UPDATE commerce_mission_operations SET result_snapshot=result_snapshot-'code' WHERE operation_type='ARRIVE_PICKUP' AND result_snapshot->>'code' !~ '^handover_[A-Za-z0-9_-]{24}$';
UPDATE commerce_mission_operations SET result_snapshot=result_snapshot-'challenge_code' WHERE operation_type='START_PROVIDER_TRANSFER' AND result_snapshot->>'challenge_code' !~ '^handover_[A-Za-z0-9_-]{24}$';
ALTER TABLE commerce_mission_stops ADD CONSTRAINT commerce_stop_reference_only CHECK(handover_code='' OR handover_code ~ '^handover_[A-Za-z0-9_-]{24}$');
ALTER TABLE commerce_mission_operations ADD CONSTRAINT commerce_operation_reference_only CHECK (
 (operation_type<>'ARRIVE_PICKUP' OR NOT result_snapshot ? 'code' OR result_snapshot->>'code' ~ '^handover_[A-Za-z0-9_-]{24}$') AND
 (operation_type<>'START_PROVIDER_TRANSFER' OR NOT result_snapshot ? 'challenge_code' OR result_snapshot->>'challenge_code' ~ '^handover_[A-Za-z0-9_-]{24}$'));
