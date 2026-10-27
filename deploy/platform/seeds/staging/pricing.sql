-- Staging pricing geography and operators.
-- Safe to re-run: fixed ids, ON CONFLICT DO NOTHING.
--
-- Geography: Nigeria > Lagos State > Lagos Metropolitan market (code NG-LAG,
-- the same code taxi's markets use) > one 60 km circle around central Lagos.
-- A pickup inside the circle is priced with Lagos's rates.
--
-- Operators: two STAGING-ONLY names, so the V1 rate books can go through the
-- real four-eye approval (the database refuses a change approved by the person
-- who proposed it). Production uses real people's names.
--
-- Rate books are NOT inserted here: every rate must be proposed, approved and
-- activated through the signed admin API (cmd/pricingctl), never by SQL.

INSERT INTO pricing_countries (code, name, currency) VALUES
  ('NG', 'Nigeria', 'NGN')
ON CONFLICT DO NOTHING;

INSERT INTO pricing_states (id, country_code, code, name) VALUES
  ('st_5eed0000-0000-4000-8000-000000000001', 'NG', 'LA', 'Lagos')
ON CONFLICT DO NOTHING;

INSERT INTO pricing_markets (id, country_code, state_id, code, name, timezone) VALUES
  ('mkt_5eed0000-0000-4000-8000-000000000001', 'NG', 'st_5eed0000-0000-4000-8000-000000000001',
   'NG-LAG', 'Lagos Metropolitan', 'Africa/Lagos')
ON CONFLICT DO NOTHING;

INSERT INTO pricing_zones (id, market_id, code, name) VALUES
  ('zn_5eed0000-0000-4000-8000-000000000001', 'mkt_5eed0000-0000-4000-8000-000000000001',
   'LAGOS-METRO', 'Lagos Metro')
ON CONFLICT DO NOTHING;

-- Centre 6.5244 N, 3.3792 E; radius 60 km. shape_hash is the SHA-256 of the
-- canonical (sorted-key, compact) JSON of the shape.
INSERT INTO pricing_zone_versions (id, zone_id, version_no, shape, shape_hash, created_by) VALUES
  ('znv_5eed0000-0000-4000-8000-000000000001', 'zn_5eed0000-0000-4000-8000-000000000001', 1,
   '{"center_lat_e6":6524400,"center_lng_e6":3379200,"radius_m":60000,"type":"CIRCLE"}',
   '3214310daf727d38008958f32af8c08fb1378363871e4ab58b24480a71e1b65b',
   'staging-seed')
ON CONFLICT DO NOTHING;

INSERT INTO pricing_operators (operator_id, display_name, roles) VALUES
  ('staging-editor@karryam.com',   'Staging editor (staging only)',   '{EDITOR}'),
  ('staging-approver@karryam.com', 'Staging approver (staging only)', '{APPROVER,ROLLOUT_APPROVER}')
ON CONFLICT DO NOTHING;
