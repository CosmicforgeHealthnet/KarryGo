-- Audit source binding preparation; no owner allocation is issued here.
CREATE TABLE IF NOT EXISTS admin_owner_context_preparation_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 workload_principal text NOT NULL,purpose_code text NOT NULL,binding_fingerprint text NOT NULL,source_contract text NOT NULL,
 known_source_dimensions jsonb NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS admin_owner_context_preparation_immutable ON admin_owner_context_preparation_audit;
CREATE TRIGGER admin_owner_context_preparation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_preparation_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();
