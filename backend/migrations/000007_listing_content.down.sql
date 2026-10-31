BEGIN;
ALTER TABLE sfd.listings DROP COLUMN image_url, DROP COLUMN description, DROP COLUMN title;
COMMIT;
