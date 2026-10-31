BEGIN;
ALTER TABLE sfd.order_items DROP COLUMN product_name;
COMMIT;
