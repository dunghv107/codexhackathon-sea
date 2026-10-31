BEGIN;
ALTER TABLE sfd.listings ADD COLUMN IF NOT EXISTS title text CHECK (length(title) <= 200);
ALTER TABLE sfd.listings ADD COLUMN IF NOT EXISTS description text CHECK (length(description) <= 5000);
ALTER TABLE sfd.listings ADD COLUMN IF NOT EXISTS image_url text CHECK (length(image_url) <= 2048);
COMMIT;
