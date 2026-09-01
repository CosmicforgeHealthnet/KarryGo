DO $$ BEGIN IF EXISTS(SELECT 1 FROM commerce_platform_operational_events WHERE event_type LIKE 'dispatch.%') THEN RAISE EXCEPTION 'retain consumed Dispatch lifecycle history; rollback requires maintenance'; END IF; END $$;
ALTER TABLE commerce_platform_operational_events DROP CONSTRAINT commerce_platform_operational_events_event_type_check;
ALTER TABLE commerce_platform_operational_events ADD CONSTRAINT commerce_platform_operational_events_event_type_check CHECK(event_type IN ('commerce.evidence.recorded.v1','commerce.exception.reported.v1','commerce.return.started.v1','commerce.return.completed.v1','commerce.provider_transfer.started.v1','commerce.provider_transfer.completed.v1'));
ALTER TABLE commerce_platform_mission_refs DROP COLUMN dispatch_domain_projection;
