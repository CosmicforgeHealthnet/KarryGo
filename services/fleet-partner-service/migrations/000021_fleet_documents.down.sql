-- Reverses 000021_fleet_documents.
--
-- Dropped in dependency order. The seeded permissions and reason codes are
-- removed too, because leaving a permission code behind that nothing grants
-- and no route checks is worse than removing it: an operator reading the
-- catalogue would believe a capability exists.

DROP TRIGGER IF EXISTS trg_fleet_partner_document_verifications_append_only
    ON fleet_partner_document_verifications;
DROP TRIGGER IF EXISTS trg_fleet_partner_document_history_append_only
    ON fleet_partner_document_history;

DROP TABLE IF EXISTS fleet_partner_document_deletion_requests;
DROP TABLE IF EXISTS fleet_partner_document_reconciliation_cases;
DROP TABLE IF EXISTS fleet_partner_document_media_versions;
DROP TABLE IF EXISTS fleet_partner_document_verifications;
DROP TABLE IF EXISTS fleet_partner_document_history;
DROP TABLE IF EXISTS fleet_partner_documents;

DELETE FROM fleet_partner_role_permissions
WHERE permission_code IN ('documents.read', 'documents.manage', 'documents.download',
                          'documents.verify', 'documents.delete');

DELETE FROM fleet_partner_permissions
WHERE code IN ('documents.read', 'documents.manage', 'documents.download',
               'documents.verify', 'documents.delete');

-- Reason codes are referenced by decision rows; remove those references first
-- so the delete cannot leave a decision pointing at a code that is gone.
DELETE FROM fleet_partner_eligibility_decision_reasons
WHERE reason_code IN ('document_missing', 'document_not_verified', 'document_not_safe',
                      'document_expired', 'document_revoked', 'document_replaced',
                      'document_deleted', 'document_projection_stale', 'document_reconciling');

DELETE FROM fleet_partner_eligibility_reason_codes
WHERE code IN ('document_missing', 'document_not_verified', 'document_not_safe',
               'document_expired', 'document_revoked', 'document_replaced',
               'document_deleted', 'document_projection_stale', 'document_reconciling');
