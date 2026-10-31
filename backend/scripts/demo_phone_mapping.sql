-- Synthetic demo numbers only. Requires psql -v phone=... -v phone_key=...
-- -v scenario=... -v source=...; application uses these only with DEMO_MODE=true.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO sfd.demo_phone_mappings(batch_id,phone_e164,phone_key,scenario_label,source_reference)
VALUES ('olist_vn_sim', :'phone',
    (SELECT phone_key FROM sfd.checkout_identity_snapshots
     WHERE batch_id='olist_vn_sim' AND phone_key=:'phone_key' LIMIT 1),
    :'scenario', :'source');
-- Missing history fails NOT NULL; a duplicate mapping fails rather than silently remapping identity.
COMMIT;
