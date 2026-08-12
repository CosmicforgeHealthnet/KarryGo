DROP TABLE IF EXISTS hauling_private_execution_mappings;
-- Opaque identifiers cannot be safely converted back to UUID.  The down
-- migration intentionally retains the corrected TEXT authority columns.
