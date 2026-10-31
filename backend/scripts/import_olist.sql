-- Import the Olist dataset (Kaggle olistbr/brazilian-ecommerce) into sfd.
-- Run via `make import-olist`: psql with the 9 CSVs mounted at /data/olist.
-- Raw CSVs are kept as text in schema olist_raw; the transform follows docs/API_CONTRACT.md "Dữ liệu Olist".
-- Single transaction: any error rolls back everything. Refuses to run twice.
\set ON_ERROR_STOP on

BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM sfd.data_batches WHERE batch_id = 'olist') THEN
        RAISE EXCEPTION 'batch olist already imported; reset the database to re-import';
    END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 1. Raw load (client-side \copy, every column text)
-- ---------------------------------------------------------------------------
CREATE SCHEMA olist_raw;

CREATE TABLE olist_raw.customers (
    customer_id text, customer_unique_id text, customer_zip_code_prefix text,
    customer_city text, customer_state text
);
CREATE TABLE olist_raw.geolocation (
    geolocation_zip_code_prefix text, geolocation_lat text, geolocation_lng text,
    geolocation_city text, geolocation_state text
);
CREATE TABLE olist_raw.order_items (
    order_id text, order_item_id text, product_id text, seller_id text,
    shipping_limit_date text, price text, freight_value text
);
CREATE TABLE olist_raw.order_payments (
    order_id text, payment_sequential text, payment_type text,
    payment_installments text, payment_value text
);
CREATE TABLE olist_raw.order_reviews (
    review_id text, order_id text, review_score text, review_comment_title text,
    review_comment_message text, review_creation_date text, review_answer_timestamp text
);
CREATE TABLE olist_raw.orders (
    order_id text, customer_id text, order_status text, order_purchase_timestamp text,
    order_approved_at text, order_delivered_carrier_date text,
    order_delivered_customer_date text, order_estimated_delivery_date text
);
CREATE TABLE olist_raw.products (
    product_id text, product_category_name text, product_name_lenght text,
    product_description_lenght text, product_photos_qty text, product_weight_g text,
    product_length_cm text, product_height_cm text, product_width_cm text
);
CREATE TABLE olist_raw.sellers (
    seller_id text, seller_zip_code_prefix text, seller_city text, seller_state text
);
CREATE TABLE olist_raw.category_translation (
    product_category_name text, product_category_name_english text
);

\copy olist_raw.customers FROM '/data/olist/olist_customers_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.geolocation FROM '/data/olist/olist_geolocation_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.order_items FROM '/data/olist/olist_order_items_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.order_payments FROM '/data/olist/olist_order_payments_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.order_reviews FROM '/data/olist/olist_order_reviews_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.orders FROM '/data/olist/olist_orders_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.products FROM '/data/olist/olist_products_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.sellers FROM '/data/olist/olist_sellers_dataset.csv' WITH (FORMAT csv, HEADER true)
\copy olist_raw.category_translation FROM '/data/olist/product_category_name_translation.csv' WITH (FORMAT csv, HEADER true)

-- ---------------------------------------------------------------------------
-- 2. Transform into sfd
-- ---------------------------------------------------------------------------
SET LOCAL search_path TO sfd, public;

INSERT INTO data_batches (batch_id, source_system, is_synthetic, source_reference, loaded_at) VALUES
    ('olist', 'olist', false, 'Kaggle olistbr/brazilian-ecommerce', now()),
    ('olist-demo-overlay', 'demo', true, 'demo seller per Olist shop; Olist has no seller accounts', now());

-- Source orders without items stay in olist_raw only.
CREATE TEMP TABLE imported_orders ON COMMIT DROP AS
SELECT o.*
FROM olist_raw.orders o
WHERE EXISTS (SELECT 1 FROM olist_raw.order_items i WHERE i.order_id = o.order_id);

-- One demo seller per Olist shop (Olist has no seller accounts).
INSERT INTO sellers (seller_id, display_name, batch_id, source_record_id)
SELECT 'demo:' || seller_id, 'Seller ' || upper(left(seller_id, 6)), 'olist-demo-overlay', seller_id
FROM olist_raw.sellers;

INSERT INTO shops (shop_id, display_name, owner_seller_id, batch_id, source_record_id)
SELECT 'olist:' || seller_id, 'Shop ' || upper(left(seller_id, 6)), 'demo:' || seller_id, 'olist', seller_id
FROM olist_raw.sellers;

INSERT INTO products (product_id, category_code, category_name_en, batch_id, source_record_id)
SELECT 'olist:' || p.product_id,
       nullif(p.product_category_name, ''),
       nullif(t.product_category_name_english, ''),
       'olist', p.product_id
FROM olist_raw.products p
LEFT JOIN olist_raw.category_translation t ON t.product_category_name = p.product_category_name;

-- Demo-only current price: the price of the most recent sale of that (seller, product).
INSERT INTO listings (listing_id, shop_id, product_id, current_price, currency, batch_id, source_record_id)
SELECT DISTINCT ON (i.seller_id, i.product_id)
       'olist:' || i.seller_id || ':' || i.product_id,
       'olist:' || i.seller_id,
       'olist:' || i.product_id,
       i.price::numeric(18,2),
       'BRL', 'olist', i.seller_id || ':' || i.product_id
FROM olist_raw.order_items i
JOIN imported_orders o ON o.order_id = i.order_id
ORDER BY i.seller_id, i.product_id,
         o.order_purchase_timestamp::timestamp DESC, i.order_id DESC, i.order_item_id::int DESC;

INSERT INTO buyers (buyer_id, batch_id, source_record_id)
SELECT DISTINCT 'olist:' || c.customer_unique_id, 'olist', c.customer_unique_id
FROM imported_orders o
JOIN olist_raw.customers c ON c.customer_id = o.customer_id;

INSERT INTO checkouts (checkout_id, buyer_id, placed_at, payment_method, currency, total_amount, batch_id, source_record_id)
SELECT 'olist:' || o.order_id,
       'olist:' || c.customer_unique_id,
       o.order_purchase_timestamp::timestamp AT TIME ZONE 'America/Sao_Paulo',
       CASE
           WHEN pm.type_count = 1 THEN pm.single_type
           WHEN pm.type_count > 1 THEN 'mixed'
           ELSE 'not_defined' -- no payment rows
       END,
       'BRL',
       it.total,
       'olist', o.order_id
FROM imported_orders o
JOIN olist_raw.customers c ON c.customer_id = o.customer_id
JOIN (
    SELECT order_id, sum(price::numeric(18,2) + freight_value::numeric(18,2)) AS total
    FROM olist_raw.order_items GROUP BY order_id
) it ON it.order_id = o.order_id
LEFT JOIN (
    SELECT order_id, count(DISTINCT payment_type) AS type_count, min(payment_type) AS single_type
    FROM olist_raw.order_payments GROUP BY order_id
) pm ON pm.order_id = o.order_id;

INSERT INTO checkout_addresses (checkout_id, postal_prefix, city, region, country_code, batch_id, source_record_id)
SELECT 'olist:' || o.order_id, c.customer_zip_code_prefix, c.customer_city, c.customer_state, 'BR', 'olist', c.customer_id
FROM imported_orders o
JOIN olist_raw.customers c ON c.customer_id = o.customer_id;

-- One order per (source order, seller); defaults unfulfilled/pending.
INSERT INTO orders (order_id, checkout_id, shop_id, batch_id, source_record_id)
SELECT DISTINCT 'olist:' || order_id || ':' || seller_id, 'olist:' || order_id, 'olist:' || seller_id,
       'olist', order_id || ':' || seller_id
FROM olist_raw.order_items;

INSERT INTO order_items (item_id, order_id, shop_id, listing_id, quantity, unit_price, freight_amount, batch_id, source_record_id)
SELECT 'olist:' || order_id || ':' || order_item_id,
       'olist:' || order_id || ':' || seller_id,
       'olist:' || seller_id,
       'olist:' || seller_id || ':' || product_id,
       1, price::numeric(18,2), freight_value::numeric(18,2),
       'olist', order_id || ':' || order_item_id
FROM olist_raw.order_items;

-- Olist has no payment outcome, so status stays unknown.
INSERT INTO payments (payment_id, checkout_id, method, amount, status, batch_id, source_record_id)
SELECT 'olist:' || p.order_id || ':' || p.payment_sequential, 'olist:' || p.order_id,
       p.payment_type, p.payment_value::numeric(18,2), 'unknown',
       'olist', p.order_id || ':' || p.payment_sequential
FROM olist_raw.order_payments p
JOIN imported_orders o ON o.order_id = p.order_id;

INSERT INTO reviews (review_record_id, checkout_id, rating, title, message,
                     source_created_at_raw, source_answered_at_raw, batch_id, source_record_id)
SELECT 'olist:' || r.review_id || ':' || r.order_id, 'olist:' || r.order_id,
       r.review_score::int, nullif(btrim(r.review_comment_title), ''), nullif(btrim(r.review_comment_message), ''),
       r.review_creation_date, r.review_answer_timestamp,
       'olist', r.review_id || ':' || r.order_id
FROM olist_raw.order_reviews r
JOIN imported_orders o ON o.order_id = r.order_id;

-- Self-check: checkout total must equal the sum of its order items.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM sfd.checkouts c
        LEFT JOIN sfd.orders o ON o.checkout_id = c.checkout_id
        LEFT JOIN sfd.order_items i ON i.order_id = o.order_id
        WHERE c.batch_id = 'olist'
        GROUP BY c.checkout_id, c.total_amount
        HAVING c.total_amount <> coalesce(sum(i.quantity * i.unit_price + i.freight_amount), 0)
    ) THEN
        RAISE EXCEPTION 'checkout total does not match its items';
    END IF;
END $$;

COMMIT;
