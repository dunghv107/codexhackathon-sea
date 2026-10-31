#!/usr/bin/env bash
# Usage: bash scripts/bootstrap-demo.sh [docker compose global options]
set -euo pipefail

if [[ -z ${BOOTSTRAP_BUYER_PASSWORD:-} ]]; then
  read -r -s -p 'Password for buyer_demo (8–72 bytes): ' BOOTSTRAP_BUYER_PASSWORD
  printf '\n'
fi
if [[ -z ${BOOTSTRAP_SELLER_PASSWORD:-} ]]; then
  read -r -s -p 'Password for seller_demo (8–72 bytes): ' BOOTSTRAP_SELLER_PASSWORD
  printf '\n'
fi
export BOOTSTRAP_BUYER_PASSWORD BOOTSTRAP_SELLER_PASSWORD
trap 'unset BOOTSTRAP_BUYER_PASSWORD BOOTSTRAP_SELLER_PASSWORD' EXIT

# Pass secrets by environment name, never as command-line values or shell-expanded SQL.
docker compose "$@" exec -T \
  -e BOOTSTRAP_BUYER_PASSWORD -e BOOTSTRAP_SELLER_PASSWORD \
  postgres sh -ec 'export PGPASSWORD="$POSTGRES_PASSWORD"; exec psql -X -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
\getenv buyer_password BOOTSTRAP_BUYER_PASSWORD
\getenv seller_password BOOTSTRAP_SELLER_PASSWORD
BEGIN;
SET LOCAL lock_timeout = '10s';
SET LOCAL search_path = sfd, public;
DO $$ BEGIN PERFORM pg_advisory_xact_lock(20261031, 1); END $$;
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE TEMP TABLE bootstrap_credentials (username text, password text) ON COMMIT DROP;
INSERT INTO bootstrap_credentials VALUES ('buyer_demo', :'buyer_password'), ('seller_demo', :'seller_password');
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM bootstrap_credentials WHERE octet_length(password) NOT BETWEEN 8 AND 72) THEN
    RAISE EXCEPTION 'Passwords must contain 8–72 bytes; nothing was created';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM data_batches WHERE batch_id='olist_vn_sim' AND is_synthetic) THEN
    RAISE EXCEPTION 'Apply migrations through 000005 before bootstrap';
  END IF;
  -- An existing username is reusable only when already linked to this bootstrap's profile.
  IF EXISTS (
    SELECT 1 FROM public.users u JOIN bootstrap_credentials c USING (username)
    WHERE u.deleted_at IS NOT NULL OR u.role <> 'USER' OR
      (u.username='buyer_demo' AND NOT EXISTS (
        SELECT 1 FROM buyers WHERE buyer_id='live:bootstrap-buyer' AND auth_subject='local:' || u.id)) OR
      (u.username='seller_demo' AND NOT EXISTS (
        SELECT 1 FROM sellers WHERE seller_id='live:bootstrap-seller' AND auth_subject='local:' || u.id))
  ) THEN
    RAISE EXCEPTION 'Demo username already belongs to another account; no account or password was changed';
  END IF;
END $$;
INSERT INTO public.users (username,email,password,role,status,created_at,updated_at)
SELECT c.username,c.username || '@demo.invalid',public.crypt(c.password,public.gen_salt('bf',10)),
       'USER','ACTIVE',now(),now()
FROM bootstrap_credentials c WHERE NOT EXISTS (SELECT 1 FROM public.users u WHERE u.username=c.username);
INSERT INTO buyers (buyer_id,auth_subject,status,display_name,account_created_at,batch_id,source_record_id)
SELECT 'live:bootstrap-buyer','local:' || id,'active','Người mua demo',now(),'olist_vn_sim','bootstrap:buyer'
FROM public.users WHERE username='buyer_demo' ON CONFLICT (buyer_id) DO NOTHING;
INSERT INTO sellers (seller_id,auth_subject,status,display_name,account_created_at,batch_id,source_record_id)
SELECT 'live:bootstrap-seller','local:' || id,'active','Người bán demo',now(),'olist_vn_sim','bootstrap:seller'
FROM public.users WHERE username='seller_demo' ON CONFLICT (seller_id) DO NOTHING;
INSERT INTO shops (shop_id,display_name,owner_seller_id,owner_actor_ref,batch_id,source_record_id)
VALUES ('live:bootstrap-shop','Olist Demo VN','live:bootstrap-seller','live:bootstrap-seller','olist_vn_sim','bootstrap:shop')
ON CONFLICT (shop_id) DO NOTHING;
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM buyers b JOIN public.users u ON b.auth_subject='local:' || u.id
    WHERE b.buyer_id='live:bootstrap-buyer' AND u.username='buyer_demo' AND b.batch_id='olist_vn_sim')
  OR NOT EXISTS (SELECT 1 FROM sellers s JOIN public.users u ON s.auth_subject='local:' || u.id
    WHERE s.seller_id='live:bootstrap-seller' AND u.username='seller_demo' AND s.batch_id='olist_vn_sim')
  OR NOT EXISTS (SELECT 1 FROM shops WHERE shop_id='live:bootstrap-shop'
    AND owner_seller_id='live:bootstrap-seller' AND batch_id='olist_vn_sim') THEN
    RAISE EXCEPTION 'Bootstrap identifiers conflict with existing ownership; nothing was changed';
  END IF;
END $$;
INSERT INTO products (product_id,display_name,batch_id,source_record_id) VALUES
 ('live:bootstrap-product-1','Đèn bàn demo','olist_vn_sim','bootstrap:product-1'),
 ('live:bootstrap-product-2','Bình giữ nhiệt demo','olist_vn_sim','bootstrap:product-2'),
 ('live:bootstrap-product-3','Túi vải demo','olist_vn_sim','bootstrap:product-3')
ON CONFLICT (product_id) DO NOTHING;
INSERT INTO listings (listing_id,shop_id,product_id,current_price,currency,stock_quantity,batch_id,source_record_id) VALUES
 ('live:bootstrap-listing-1','live:bootstrap-shop','live:bootstrap-product-1',150000,'VND',50,'olist_vn_sim','bootstrap:listing-1'),
 ('live:bootstrap-listing-2','live:bootstrap-shop','live:bootstrap-product-2',120000,'VND',50,'olist_vn_sim','bootstrap:listing-2'),
 ('live:bootstrap-listing-3','live:bootstrap-shop','live:bootstrap-product-3',50000,'VND',50,'olist_vn_sim','bootstrap:listing-3')
ON CONFLICT (listing_id) DO NOTHING;
INSERT INTO saved_addresses (saved_address_id,buyer_id,label,address_raw,address_key,region,country_code,is_default,created_at,batch_id)
SELECT 'live:bootstrap-address','live:bootstrap-buyer','Địa chỉ demo','123 Nguyễn Huệ, TP Hồ Chí Minh',
 'bootstrap:address','Hồ Chí Minh','VN',
 NOT EXISTS (SELECT 1 FROM saved_addresses WHERE buyer_id='live:bootstrap-buyer' AND is_default),now(),'olist_vn_sim'
ON CONFLICT (saved_address_id) DO NOTHING;
DO $$
BEGIN
  IF (SELECT count(*) FROM listings l JOIN products p USING (product_id)
      WHERE l.listing_id IN ('live:bootstrap-listing-1','live:bootstrap-listing-2','live:bootstrap-listing-3')
        AND l.shop_id='live:bootstrap-shop' AND l.batch_id='olist_vn_sim' AND p.batch_id='olist_vn_sim'
        AND l.product_id=replace(l.listing_id,'listing','product')) <> 3
  OR NOT EXISTS (SELECT 1 FROM saved_addresses WHERE saved_address_id='live:bootstrap-address'
    AND buyer_id='live:bootstrap-buyer' AND batch_id='olist_vn_sim') THEN
    RAISE EXCEPTION 'Bootstrap catalog/address identifiers conflict with existing data; nothing was changed';
  END IF;
END $$;
COMMIT;
SQL
printf '%s\n' 'Bootstrap complete: buyer_demo / seller_demo, Olist Demo VN shop, 3 VND products and a buyer address.'
printf '%s\n' 'Existing passwords, prices, stock and account statuses were preserved. No orders or risk scores were fabricated.'
