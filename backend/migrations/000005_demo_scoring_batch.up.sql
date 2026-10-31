-- New demo writes only. Existing app/raw records retain their provenance.
INSERT INTO sfd.data_batches(batch_id, source_system, is_synthetic, source_reference, loaded_at)
VALUES ('olist_vn_sim', 'marketplace_demo', true, 'Live demo records; historical dataset not loaded by this migration', now())
ON CONFLICT (batch_id) DO NOTHING;
