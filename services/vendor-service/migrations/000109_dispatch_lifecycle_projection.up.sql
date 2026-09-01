-- Additional opaque projection in the existing reserved mission authority.
ALTER TABLE commerce_platform_mission_refs ADD COLUMN dispatch_domain_projection jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE commerce_platform_operational_events DROP CONSTRAINT commerce_platform_operational_events_event_type_check;
ALTER TABLE commerce_platform_operational_events ADD CONSTRAINT commerce_platform_operational_events_event_type_check CHECK(event_type IN (
 'commerce.evidence.recorded.v1','commerce.exception.reported.v1','commerce.return.started.v1','commerce.return.completed.v1','commerce.provider_transfer.started.v1','commerce.provider_transfer.completed.v1',
 'dispatch.delivery_plan.created.v1','dispatch.delivery_plan.superseded.v1','dispatch.mission.assigned.v1','dispatch.pickup.verified.v1','dispatch.receiver.verified.v1','dispatch.return.completed.v1','dispatch.return.started.v1','dispatch.delivery_recovery.selected.v1','dispatch.provider_replacement.started.v1'));
