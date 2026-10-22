-- Intervention records are immutable evidence; roll back only while empty.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM dispatch_admin_intervention_records) OR EXISTS(SELECT 1 FROM dispatch_admin_intervention_heads) THEN
  RAISE EXCEPTION 'Dispatch intervention evidence requires an approved forward retention migration';
 END IF;
END $$;
DROP TABLE dispatch_admin_intervention_records;
DROP TABLE dispatch_admin_intervention_heads;
DROP FUNCTION reject_dispatch_admin_intervention_record_mutation();
DROP FUNCTION reject_dispatch_admin_intervention_truncate();
DROP FUNCTION dispatch_admin_intervention_head_fence();
