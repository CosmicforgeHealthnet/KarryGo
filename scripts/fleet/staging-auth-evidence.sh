#!/usr/bin/env bash
# Read-only staging evidence. Never select codes, digests, tokens or email bodies.
set -euo pipefail
test_email="${1:?test email required}"
[[ "$test_email" =~ ^[A-Za-z0-9._+@-]+$ ]] || exit 1
fleet_db=karrygo-fleet-partner-db
notification_db=karrygo-notification-db
for container in karrygo-fleet-partner-api karrygo-fleet-partner-worker; do
  image=$(docker inspect -f '{{.Image}}' "$container")
  revision=$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image")
  echo "FLEET_RUNTIME_EVIDENCE container=$container image=$image revision=$revision"
done
echo 'FLEET_DATABASE_EVIDENCE'
docker exec -i -e TEST_EMAIL="$test_email" "$fleet_db" sh -c 'exec psql -X -v ON_ERROR_STOP=1 -v test_email="$TEST_EMAIL" -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
BEGIN READ ONLY;
SELECT version,applied_at FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1;
SELECT indexname,indexdef FROM pg_indexes WHERE tablename='fleet_partner_corporate_identities' AND indexname LIKE 'uq_%' ORDER BY indexname;
SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='fleet_partner_auth_otp_challenges'::regclass AND conname LIKE '%purpose%';
SELECT count(*) AS legacy_case_collision_groups FROM (SELECT lower(email_normalized) FROM fleet_partner_corporate_identities WHERE status<>'ARCHIVED' GROUP BY 1 HAVING count(*)>1) collisions;
SELECT count(*) AS legacy_malformed_phone_count FROM fleet_partner_corporate_identities WHERE status<>'ARCHIVED' AND phone_e164 !~ '^\+[1-9][0-9]{7,14}$';
SELECT count(*) AS test_identity_count FROM fleet_partner_corporate_identities WHERE lower(email_normalized)=lower(:'test_email');
SELECT purpose,status,delivery_status,count(*) FROM fleet_partner_auth_otp_challenges WHERE lower(email_normalized)=lower(:'test_email') GROUP BY 1,2,3 ORDER BY 1,2,3;
SELECT count(*) AS test_active_sessions FROM fleet_partner_corporate_sessions WHERE identity_id IN (SELECT id FROM fleet_partner_corporate_identities WHERE lower(email_normalized)=lower(:'test_email')) AND status='ACTIVE';
COMMIT;
SQL
echo 'FLEET_NOTIFICATION_EVIDENCE'
docker exec -i -e TEST_EMAIL="$test_email" "$notification_db" sh -c 'exec psql -X -v ON_ERROR_STOP=1 -v test_email="$TEST_EMAIL" -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
BEGIN READ ONLY;
SELECT m.event_type,m.channels,m.status,count(*) FROM notification_messages m WHERE m.source_service='fleet-partner-service' AND lower(m.recipient_email)=lower(:'test_email') GROUP BY 1,2,3;
SELECT d.channel,d.status,d.attempts,count(*) FROM notification_deliveries d JOIN notification_messages m ON m.id=d.message_id WHERE m.source_service='fleet-partner-service' AND lower(m.recipient_email)=lower(:'test_email') GROUP BY 1,2,3;
COMMIT;
SQL
