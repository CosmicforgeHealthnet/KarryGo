-- Security audit history is evidence, not mutable application state. Keep the
-- database invariant aligned with the append-only repository contract so an
-- ordinary application connection cannot rewrite or erase an actor's trail.

CREATE OR REPLACE FUNCTION vendor_audit_log_append_only_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'vendor_audit_log is append-only: % is not permitted', TG_OP
        USING ERRCODE = '55000';

    RETURN NULL;
END;
$$;

CREATE TRIGGER vendor_audit_log_append_only_trg
BEFORE UPDATE OR DELETE
ON vendor_audit_log
FOR EACH ROW
EXECUTE FUNCTION vendor_audit_log_append_only_guard();
