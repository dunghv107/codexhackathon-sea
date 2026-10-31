BEGIN;
ALTER TABLE sfd.order_items ADD COLUMN IF NOT EXISTS product_name text;
-- Historical titles cannot be reconstructed; freeze the best currently available name.
-- Imported canonical rows without a name retain NULL and the immutable product fallback.
UPDATE sfd.order_items i
SET product_name = COALESCE(l.title, p.display_name)
FROM sfd.listings l JOIN sfd.products p ON p.product_id = l.product_id
WHERE i.listing_id = l.listing_id AND i.product_name IS NULL
  AND COALESCE(l.title, p.display_name) IS NOT NULL;
COMMIT;
