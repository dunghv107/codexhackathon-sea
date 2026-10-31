-- Demo accounts on top of the Olist import. Run via `make seed-demo` after `make import-olist`.
-- seller_demo / demo1234 -> seller of the shop with the most orders
-- buyer_demo  / demo1234 -> buyer with the most checkouts, plus one default saved address
-- Single transaction; refuses to run twice.
\set ON_ERROR_STOP on

BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM sfd.data_batches WHERE batch_id = 'olist') THEN
        RAISE EXCEPTION 'batch olist not found; run make import-olist first';
    END IF;
    IF EXISTS (SELECT 1 FROM sfd.data_batches WHERE batch_id = 'demo-seed') THEN
        RAISE EXCEPTION 'batch demo-seed already exists; demo accounts are already seeded';
    END IF;
END $$;

-- crypt(..., gen_salt('bf')) produces $2a$ bcrypt hashes that Go's bcrypt accepts.
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;

INSERT INTO sfd.data_batches (batch_id, source_system, is_synthetic, source_reference, loaded_at)
VALUES ('demo-seed', 'demo', true, 'scripts/seed_demo.sql demo accounts', now());

INSERT INTO public.users (username, email, password, role, status, created_at, updated_at) VALUES
    ('seller_demo', 'seller_demo@demo.local', public.crypt('demo1234', public.gen_salt('bf', 10)), 'USER', 'ACTIVE', now(), now()),
    ('buyer_demo', 'buyer_demo@demo.local', public.crypt('demo1234', public.gen_salt('bf', 10)), 'USER', 'ACTIVE', now(), now());

UPDATE sfd.sellers s
SET auth_subject = 'local:' || u.id
FROM public.users u
WHERE u.username = 'seller_demo'
  AND s.seller_id = (
      SELECT sh.owner_seller_id
      FROM sfd.shops sh
      JOIN sfd.orders o ON o.shop_id = sh.shop_id
      GROUP BY sh.shop_id, sh.owner_seller_id
      ORDER BY count(*) DESC, sh.shop_id
      LIMIT 1
  );

UPDATE sfd.buyers b
SET auth_subject = 'local:' || u.id
FROM public.users u
WHERE u.username = 'buyer_demo'
  AND b.buyer_id = (
      SELECT c.buyer_id
      FROM sfd.checkouts c
      GROUP BY c.buyer_id
      ORDER BY count(*) DESC, c.buyer_id
      LIMIT 1
  );

INSERT INTO sfd.saved_addresses (saved_address_id, buyer_id, label, address_raw, address_key,
                                 region, country_code, is_default, created_at, batch_id)
SELECT 'demo:' || gen_random_uuid(), b.buyer_id, 'Nhà',
       a.raw, lower(regexp_replace(btrim(a.raw), '\s+', ' ', 'g')),
       'TP. Hồ Chí Minh', 'VN', true, now(), 'demo-seed'
FROM sfd.buyers b
CROSS JOIN (VALUES ('12 Nguyễn Huệ, Phường Bến Nghé, Quận 1, TP. Hồ Chí Minh')) AS a(raw)
JOIN public.users u ON b.auth_subject = 'local:' || u.id
WHERE u.username = 'buyer_demo';

-- Self-check: both accounts must be linked.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM sfd.sellers s JOIN public.users u ON s.auth_subject = 'local:' || u.id
                   WHERE u.username = 'seller_demo')
       OR NOT EXISTS (SELECT 1 FROM sfd.buyers b JOIN public.users u ON b.auth_subject = 'local:' || u.id
                      WHERE u.username = 'buyer_demo')
       OR (SELECT count(*) FROM sfd.saved_addresses WHERE batch_id = 'demo-seed') <> 1 THEN
        RAISE EXCEPTION 'demo accounts were not linked as expected';
    END IF;
END $$;

COMMIT;
