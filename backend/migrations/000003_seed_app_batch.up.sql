-- Batch for records created through the platform API (IDs 'app:<uuid>').
INSERT INTO sfd.data_batches (batch_id, source_system, is_synthetic, source_reference, loaded_at)
VALUES ('app', 'app', false, 'created via platform API', now());
