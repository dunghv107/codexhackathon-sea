BEGIN;

-- Only rename imported demo placeholders; keep custom names and all shop identities.
-- shortcut: imports after this migration need a rerun; move naming into import when that workflow changes.
WITH last_number AS (
    SELECT COALESCE(max(substring(display_name FROM '^Cửa hàng demo ([0-9]{1,9})$')::bigint), 0) AS n
    FROM sfd.shops
), placeholders AS (
    SELECT shop_id, row_number() OVER (ORDER BY shop_id) AS n
    FROM sfd.shops
    WHERE batch_id = 'olist_vn_sim'
      AND shop_id ~ '^sim:[0-9a-f]{32}$'
      AND display_name = 'Demo shop ' || substring(shop_id FROM 5)
)
UPDATE sfd.shops s
SET display_name = 'Cửa hàng demo ' || (last_number.n + placeholders.n)::text
FROM placeholders CROSS JOIN last_number
WHERE s.shop_id = placeholders.shop_id;

COMMIT;
