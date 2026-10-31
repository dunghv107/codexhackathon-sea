#!/usr/bin/env bash
# Seed 300 synthetic orders for one demo shop in a 4m59s window.
# Buyers 001-005 build verified failure history; their last five orders also
# exceed the adaptive quantity/velocity baseline. Assessments remain KBS-owned.
# Usage: bash scripts/seed-sale-burst-demo.sh [docker compose global options]
set -euo pipefail

if [[ ${DEMO_MODE:-false} != true || ${ALLOW_DEMO_SEED:-false} != true ]]; then
  echo 'Refusing demo seed: set DEMO_MODE=true and ALLOW_DEMO_SEED=true explicitly.' >&2
  exit 1
fi

COMPOSE_ARGS=("$@")
docker compose "${COMPOSE_ARGS[@]}" exec -T postgres sh -ec '
  export PGPASSWORD="$POSTGRES_PASSWORD"
  exec psql -X -qAt -v ON_ERROR_STOP=1 -v VERBOSITY=terse \
    -U "$POSTGRES_USER" -d "$POSTGRES_DB"
' <<'SQL'
BEGIN;
SET LOCAL search_path = sfd, public;
SET LOCAL lock_timeout = '10s';
SELECT pg_advisory_xact_lock(20261031, 2);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM data_batches
    WHERE batch_id = 'olist_vn_sim' AND is_synthetic
  ) THEN
    RAISE EXCEPTION 'Synthetic batch olist_vn_sim is missing; run the demo bootstrap first';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM public.users u
    JOIN sellers seller ON seller.auth_subject = 'local:' || u.id
    JOIN shops s ON s.owner_seller_id = seller.seller_id AND s.batch_id = seller.batch_id
    JOIN listings l ON l.shop_id = s.shop_id AND l.batch_id = s.batch_id
    WHERE u.username = 'seller_demo'
      AND u.deleted_at IS NULL
      AND u.status = 'ACTIVE'
      AND seller.status = 'active'
      AND s.shop_id = 'live:bootstrap-shop'
      AND s.batch_id = 'olist_vn_sim'
      AND l.listing_id = 'live:bootstrap-listing-1'
      AND l.current_price IS NOT NULL
      AND l.current_price > 0
  ) THEN
    RAISE EXCEPTION 'Bootstrap shop/listing is missing or conflicts with the sale scenario';
  END IF;
END $$;

CREATE TEMP TABLE sale_burst_state ON COMMIT DROP AS
SELECT EXISTS (
         SELECT 1 FROM checkouts
         WHERE batch_id = 'olist_vn_sim'
           AND source_record_id LIKE 'sale-burst:v1:checkout:%'
       ) AS existing,
       COALESCE(
         (SELECT min(placed_at) FROM checkouts
          WHERE batch_id = 'olist_vn_sim'
            AND source_record_id LIKE 'sale-burst:v1:checkout:%'),
         statement_timestamp() - interval '5 minutes'
       ) AS base_at;

CREATE TEMP TABLE sale_burst_rows ON COMMIT DROP AS
SELECT n,
       ((n - 1) % 20) + 1 AS buyer_number,
       ((n - 1) / 20) + 1 AS buyer_order_number,
       CASE
         WHEN ((n - 1) % 20) + 1 <= 5 AND ((n - 1) / 20) + 1 >= 11 THEN 20
         ELSE (((n - 1) / 20) % 2) + 1
       END AS quantity,
       s.base_at + (n - 1) * interval '1 second' AS placed_at
FROM generate_series(1, 300) AS n
CROSS JOIN sale_burst_state s;

INSERT INTO buyers
  (buyer_id, auth_subject, status, display_name, account_created_at, batch_id, source_record_id)
SELECT format('live:sale-burst:buyer:%s', to_char(n, 'FM000')),
       NULL,
       'active',
       format('Người mua sale demo %s', to_char(n, 'FM000')),
       s.base_at - interval '30 days',
       'olist_vn_sim',
       format('sale-burst:v1:buyer:%s', to_char(n, 'FM000'))
FROM generate_series(1, 20) AS n
CROSS JOIN sale_burst_state s
WHERE NOT s.existing;

INSERT INTO checkouts
  (checkout_id, buyer_id, placed_at, payment_method, currency, total_amount, batch_id, source_record_id)
SELECT format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:buyer:%s', to_char(r.buyer_number, 'FM000')),
       r.placed_at,
       'credit_card',
       l.currency,
       l.current_price * r.quantity,
       'olist_vn_sim',
       format('sale-burst:v1:checkout:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
JOIN listings l ON l.listing_id = 'live:bootstrap-listing-1'
WHERE NOT s.existing;

INSERT INTO orders
  (order_id, checkout_id, shop_id, fulfillment_status, review_status, batch_id, source_record_id)
SELECT format('live:sale-burst:order:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000')),
       'live:bootstrap-shop',
       'unfulfilled',
       'pending',
       'olist_vn_sim',
       format('sale-burst:v1:order:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
WHERE NOT s.existing;

INSERT INTO order_items
  (item_id, order_id, shop_id, listing_id, quantity, unit_price, freight_amount, batch_id, source_record_id)
SELECT format('live:sale-burst:item:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:order:%s', to_char(r.n, 'FM000')),
       'live:bootstrap-shop',
       'live:bootstrap-listing-1',
       r.quantity,
       l.current_price,
       0,
       'olist_vn_sim',
       format('sale-burst:v1:item:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
JOIN listings l ON l.listing_id = 'live:bootstrap-listing-1'
WHERE NOT s.existing;

INSERT INTO payments
  (payment_id, checkout_id, method, amount, status, paid_at, batch_id, source_record_id)
SELECT format('live:sale-burst:payment:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000')),
       'credit_card',
       l.current_price * r.quantity,
       'succeeded',
       r.placed_at,
       'olist_vn_sim',
       format('sale-burst:v1:payment:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
JOIN listings l ON l.listing_id = 'live:bootstrap-listing-1'
WHERE NOT s.existing;

INSERT INTO order_events
  (event_id, order_id, event_type, actor_type, reason, occurred_at, available_at, batch_id, source_record_id)
SELECT format('live:sale-burst:event:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:order:%s', to_char(r.n, 'FM000')),
       'created',
       'buyer',
       'Synthetic five-minute sale burst',
       r.placed_at,
       r.placed_at,
       'olist_vn_sim',
       format('sale-burst:v1:event:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
WHERE NOT s.existing;

INSERT INTO checkout_addresses
  (checkout_id, address_raw, address_key, postal_prefix, city, region, country_code,
   batch_id, source_record_id)
SELECT format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000')),
       NULL,
       format('demo:sale-burst:address:%s', to_char(r.buyer_number, 'FM000')),
       NULL,
       NULL,
       NULL,
       'VN',
       'olist_vn_sim',
       format('sale-burst:v1:address:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
WHERE NOT s.existing;

-- Terminal evidence is available before the same buyer's next checkout (20s later).
-- This compressed timing is synthetic and exists only to exercise point-in-time rules.
INSERT INTO order_events
  (event_id, order_id, event_type, actor_type, reason, occurred_at, available_at,
   batch_id, source_record_id)
SELECT format('live:sale-burst:terminal:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:order:%s', to_char(r.n, 'FM000')),
       CASE WHEN r.buyer_number <= 5 AND r.buyer_order_number <= 3
            THEN 'delivery_failed' ELSE 'delivery_succeeded' END,
       'carrier',
       CASE WHEN r.buyer_number <= 5 AND r.buyer_order_number <= 3
            THEN 'Synthetic verified buyer delivery failure'
            ELSE 'Synthetic successful delivery baseline' END,
       r.placed_at + interval '5 seconds',
       r.placed_at + interval '5 seconds',
       'olist_vn_sim',
       format('sale-burst:v1:terminal:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
WHERE NOT s.existing
  AND r.buyer_order_number <= 10;

INSERT INTO cases
  (case_id, order_id, case_type, opened_at, available_at, batch_id, source_record_id)
SELECT format('live:sale-burst:case:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:order:%s', to_char(r.n, 'FM000')),
       'delivery',
       r.placed_at + interval '6 seconds',
       r.placed_at + interval '6 seconds',
       'olist_vn_sim',
       format('sale-burst:v1:case:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
WHERE NOT s.existing
  AND r.buyer_number <= 5
  AND r.buyer_order_number <= 3;

INSERT INTO case_resolutions
  (resolution_id, case_id, verification_status, responsible_party, rationale,
   decided_by, decided_at, available_at, batch_id, source_record_id)
SELECT format('live:sale-burst:resolution:%s', to_char(r.n, 'FM000')),
       format('live:sale-burst:case:%s', to_char(r.n, 'FM000')),
       'verified',
       'buyer',
       'Synthetic ground truth for the fraud-rule demo',
       'demo-seed',
       r.placed_at + interval '7 seconds',
       r.placed_at + interval '7 seconds',
       'olist_vn_sim',
       format('sale-burst:v1:resolution:%s', to_char(r.n, 'FM000'))
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
WHERE NOT s.existing
  AND r.buyer_number <= 5
  AND r.buyer_order_number <= 3;

-- Synthetic tokens model repeated identities without storing raw phone/device/address data.
INSERT INTO checkout_identity_snapshots
  (checkout_id, phone_key, device_key, address_cluster_id, available_at, batch_id)
SELECT format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000')),
       format('demo:sale-burst:phone:%s', to_char(r.buyer_number, 'FM000')),
       format('demo:sale-burst:device:%s', to_char(r.buyer_number, 'FM000')),
       format('demo:sale-burst:address:%s', to_char(r.buyer_number, 'FM000')),
       r.placed_at,
       'olist_vn_sim'
FROM sale_burst_rows r
CROSS JOIN sale_burst_state s
WHERE NOT s.existing;

DO $$
DECLARE
  bad_buyers integer;
  assessment_count integer;
BEGIN
  SELECT count(*) INTO bad_buyers
  FROM (
    SELECT c.buyer_id
    FROM checkouts c
    JOIN orders o ON o.checkout_id = c.checkout_id AND o.batch_id = c.batch_id
    WHERE c.batch_id = 'olist_vn_sim'
      AND c.source_record_id LIKE 'sale-burst:v1:checkout:%'
    GROUP BY c.buyer_id
    HAVING count(*) <> 15
  ) AS invalid;

  IF (SELECT count(*) FROM buyers WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:buyer:%') <> 20
  OR (SELECT count(*)
      FROM generate_series(1, 20) AS n
      JOIN buyers b
        ON b.buyer_id = format('live:sale-burst:buyer:%s', to_char(n, 'FM000'))
       AND b.source_record_id = format('sale-burst:v1:buyer:%s', to_char(n, 'FM000'))
       AND b.display_name = format('Người mua sale demo %s', to_char(n, 'FM000'))
       AND b.auth_subject IS NULL
       AND b.status = 'active'
       AND b.batch_id = 'olist_vn_sim') <> 20
  OR (SELECT count(*) FROM checkouts WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:checkout:%') <> 300
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN checkouts c
        ON c.checkout_id = format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000'))
       AND c.buyer_id = format('live:sale-burst:buyer:%s', to_char(r.buyer_number, 'FM000'))
       AND c.source_record_id = format('sale-burst:v1:checkout:%s', to_char(r.n, 'FM000'))
       AND c.placed_at = r.placed_at
       AND c.total_amount = (SELECT sum(i.quantity * i.unit_price + i.freight_amount)
                             FROM order_items i
                             WHERE i.order_id = format('live:sale-burst:order:%s', to_char(r.n, 'FM000')))
       AND c.batch_id = 'olist_vn_sim') <> 300
  OR (SELECT count(*) FROM orders WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:order:%') <> 300
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN orders o
        ON o.order_id = format('live:sale-burst:order:%s', to_char(r.n, 'FM000'))
       AND o.checkout_id = format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000'))
       AND o.shop_id = 'live:bootstrap-shop'
       AND o.source_record_id = format('sale-burst:v1:order:%s', to_char(r.n, 'FM000'))
       AND o.batch_id = 'olist_vn_sim') <> 300
  OR (SELECT count(*) FROM order_items WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:item:%') <> 300
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN order_items i
        ON i.item_id = format('live:sale-burst:item:%s', to_char(r.n, 'FM000'))
       AND i.order_id = format('live:sale-burst:order:%s', to_char(r.n, 'FM000'))
       AND i.shop_id = 'live:bootstrap-shop'
       AND i.listing_id = 'live:bootstrap-listing-1'
       AND i.quantity = r.quantity
       AND i.unit_price > 0
       AND i.source_record_id = format('sale-burst:v1:item:%s', to_char(r.n, 'FM000'))
       AND i.batch_id = 'olist_vn_sim') <> 300
  OR (SELECT count(*) FROM payments WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:payment:%') <> 300
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN payments p
        ON p.payment_id = format('live:sale-burst:payment:%s', to_char(r.n, 'FM000'))
       AND p.checkout_id = format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000'))
       AND p.status = 'succeeded'
       AND p.amount = (SELECT c.total_amount FROM checkouts c
                       WHERE c.checkout_id = format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000')))
       AND p.paid_at = r.placed_at
       AND p.source_record_id = format('sale-burst:v1:payment:%s', to_char(r.n, 'FM000'))
       AND p.batch_id = 'olist_vn_sim') <> 300
  OR (SELECT count(*) FROM order_events WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:event:%') <> 300
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN order_events e
        ON e.event_id = format('live:sale-burst:event:%s', to_char(r.n, 'FM000'))
       AND e.order_id = format('live:sale-burst:order:%s', to_char(r.n, 'FM000'))
       AND e.event_type = 'created'
       AND e.occurred_at = r.placed_at
       AND e.available_at = r.placed_at
       AND e.source_record_id = format('sale-burst:v1:event:%s', to_char(r.n, 'FM000'))
       AND e.batch_id = 'olist_vn_sim') <> 300
  OR (SELECT count(*) FROM order_events WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:terminal:%') <> 200
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN order_events e
        ON e.event_id = format('live:sale-burst:terminal:%s', to_char(r.n, 'FM000'))
       AND e.order_id = format('live:sale-burst:order:%s', to_char(r.n, 'FM000'))
       AND e.event_type = CASE WHEN r.buyer_number <= 5 AND r.buyer_order_number <= 3
                               THEN 'delivery_failed' ELSE 'delivery_succeeded' END
       AND e.occurred_at = r.placed_at + interval '5 seconds'
       AND e.available_at = r.placed_at + interval '5 seconds'
       AND e.source_record_id = format('sale-burst:v1:terminal:%s', to_char(r.n, 'FM000'))
       AND e.batch_id = 'olist_vn_sim'
      WHERE r.buyer_order_number <= 10) <> 200
  OR (SELECT count(*) FROM checkout_addresses WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:address:%') <> 300
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN checkout_addresses a
        ON a.checkout_id = format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000'))
       AND a.address_key = format('demo:sale-burst:address:%s', to_char(r.buyer_number, 'FM000'))
       AND a.source_record_id = format('sale-burst:v1:address:%s', to_char(r.n, 'FM000'))
       AND a.batch_id = 'olist_vn_sim') <> 300
  OR (SELECT count(*) FROM cases WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:case:%') <> 15
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN cases c
        ON c.case_id = format('live:sale-burst:case:%s', to_char(r.n, 'FM000'))
       AND c.order_id = format('live:sale-burst:order:%s', to_char(r.n, 'FM000'))
       AND c.case_type = 'delivery'
       AND c.opened_at = r.placed_at + interval '6 seconds'
       AND c.available_at = r.placed_at + interval '6 seconds'
       AND c.source_record_id = format('sale-burst:v1:case:%s', to_char(r.n, 'FM000'))
       AND c.batch_id = 'olist_vn_sim'
      WHERE r.buyer_number <= 5 AND r.buyer_order_number <= 3) <> 15
  OR (SELECT count(*) FROM case_resolutions WHERE batch_id = 'olist_vn_sim'
      AND source_record_id LIKE 'sale-burst:v1:resolution:%') <> 15
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN case_resolutions cr
        ON cr.resolution_id = format('live:sale-burst:resolution:%s', to_char(r.n, 'FM000'))
       AND cr.case_id = format('live:sale-burst:case:%s', to_char(r.n, 'FM000'))
       AND cr.verification_status = 'verified'
       AND cr.responsible_party = 'buyer'
       AND cr.decided_at = r.placed_at + interval '7 seconds'
       AND cr.available_at = r.placed_at + interval '7 seconds'
       AND cr.source_record_id = format('sale-burst:v1:resolution:%s', to_char(r.n, 'FM000'))
       AND cr.batch_id = 'olist_vn_sim'
      WHERE r.buyer_number <= 5 AND r.buyer_order_number <= 3) <> 15
  OR (SELECT count(*) FROM checkout_identity_snapshots
      WHERE batch_id = 'olist_vn_sim'
        AND checkout_id LIKE 'live:sale-burst:checkout:%') <> 300
  OR (SELECT count(*)
      FROM sale_burst_rows r
      JOIN checkout_identity_snapshots i
        ON i.checkout_id = format('live:sale-burst:checkout:%s', to_char(r.n, 'FM000'))
       AND i.phone_key = format('demo:sale-burst:phone:%s', to_char(r.buyer_number, 'FM000'))
       AND i.device_key = format('demo:sale-burst:device:%s', to_char(r.buyer_number, 'FM000'))
       AND i.address_cluster_id = format('demo:sale-burst:address:%s', to_char(r.buyer_number, 'FM000'))
       AND i.available_at = r.placed_at
       AND i.batch_id = 'olist_vn_sim') <> 300
  OR (SELECT count(DISTINCT o.shop_id) FROM orders o
      WHERE o.batch_id = 'olist_vn_sim'
        AND o.source_record_id LIKE 'sale-burst:v1:order:%') <> 1
  OR (SELECT max(c.placed_at) - min(c.placed_at) FROM checkouts c
      WHERE c.batch_id = 'olist_vn_sim'
        AND c.source_record_id LIKE 'sale-burst:v1:checkout:%') <> interval '299 seconds'
  OR bad_buyers <> 0 THEN
    RAISE EXCEPTION 'Existing sale burst scenario is incomplete or does not match v1; nothing was changed';
  END IF;

  SELECT count(*) INTO assessment_count
  FROM risk_assessments a
  JOIN orders o ON o.order_id = a.order_id AND o.batch_id = a.batch_id
  WHERE o.source_record_id LIKE 'sale-burst:v1:order:%';
  RAISE NOTICE 'Sale burst ready: 20 buyers, 300 orders, 1 shop, 299 seconds, % existing assessments',
    assessment_count;
END $$;
COMMIT;
SQL

printf '%s\n' 'Seed complete: 20 synthetic buyers x 15 orders = 300 orders for live:bootstrap-shop in 4m59s.'
printf '%s\n' 'Expected after KBS scoring: buyers 001-005 become risky; orders 11-15 also carry A08/A04 shadow evidence.'
printf '%s\n' 'No assessment was fabricated. Restart KBS to load the active adaptive version, then score these orders.'
