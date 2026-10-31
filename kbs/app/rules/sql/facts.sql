WITH targets AS (
    SELECT o.order_id, o.shop_id, c.checkout_id, c.buyer_id, c.placed_at AS cutoff,
           c.payment_method, c.currency, b.account_created_at,
           s.phone_key, s.device_key, s.address_cluster_id,
           (SELECT sum(i.unit_price * i.quantity + i.freight_amount)
            FROM sfd.order_items i WHERE i.order_id = o.order_id AND i.batch_id = %(batch)s) AS amount
    FROM sfd.orders o
    JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = %(batch)s
    JOIN sfd.buyers b ON b.buyer_id = c.buyer_id AND b.batch_id = %(batch)s
    LEFT JOIN sfd.checkout_identity_snapshots s ON s.checkout_id = c.checkout_id
        AND s.batch_id = %(batch)s AND s.available_at <= c.placed_at
    WHERE o.batch_id = %(batch)s AND o.order_id = ANY(%(order_ids)s::text[])
      AND o.shop_id = %(shop_id)s
)
SELECT t.order_id, t.shop_id, t.cutoff AS input_cutoff_at,
       jsonb_build_object(
         'R1', jsonb_build_object('resolved', CASE WHEN t.phone_key IS NOT NULL THEN h.r1_resolved END,
                                 'failed', CASE WHEN t.phone_key IS NOT NULL THEN h.r1_failed END),
         'R2', jsonb_build_object('resolved', CASE WHEN t.address_cluster_id IS NOT NULL THEN h.r2_resolved END,
                                 'failed', CASE WHEN t.address_cluster_id IS NOT NULL THEN h.r2_failed END),
         'R3', jsonb_build_object('count', CASE WHEN t.phone_key IS NOT NULL THEN h.r3_failed END),
         'R4', jsonb_build_object('checkouts', CASE WHEN t.phone_key IS NOT NULL OR t.device_key IS NOT NULL THEN velocity.checkouts END,
                                 'shops', CASE WHEN t.phone_key IS NOT NULL OR t.device_key IS NOT NULL THEN velocity.shops END),
         'R5', jsonb_build_object('checkouts', CASE WHEN t.phone_key IS NOT NULL OR t.address_cluster_id IS NOT NULL THEN h.r5_checkouts + 1 END),
         'R6', jsonb_build_object('buyers', CASE WHEN t.address_cluster_id IS NOT NULL THEN h.r6_buyers + CASE WHEN h.current_buyer_seen THEN 0 ELSE 1 END END,
                                 'resolved', CASE WHEN t.address_cluster_id IS NOT NULL THEN h.r6_resolved END,
                                 'failed', CASE WHEN t.address_cluster_id IS NOT NULL THEN h.r6_failed END),
         'R7', jsonb_build_object('cod', CASE WHEN t.payment_method = 'cod' THEN 1 ELSE 0 END,
                                 'amount', t.amount::text, 'history_orders', baseline.n, 'median', baseline.median::text),
         'R8', jsonb_build_object('age_days', extract(epoch FROM t.cutoff - t.account_created_at) / 86400),
         'R10', jsonb_build_object('count', CASE WHEN t.phone_key IS NOT NULL THEN h.r10_cancellations END)
       ) AS features
FROM targets t
CROSS JOIN LATERAL (
    WITH history AS MATERIALIZED (
        SELECT c.checkout_id, c.buyer_id, c.placed_at,
               s.phone_key = t.phone_key AS phone_match,
               s.device_key = t.device_key AS device_match,
               s.address_cluster_id = t.address_cluster_id AS cluster_match
        FROM sfd.checkout_identity_snapshots s
        JOIN sfd.checkouts c ON c.checkout_id = s.checkout_id AND c.batch_id = %(batch)s
        WHERE s.batch_id = %(batch)s AND c.placed_at < t.cutoff AND s.available_at < t.cutoff
          AND (s.phone_key = t.phone_key OR s.device_key = t.device_key OR s.address_cluster_id = t.address_cluster_id)
    ), observed AS (
        SELECT history.*, delivery.known_at,
               coalesce(delivery.failed AND resolution.verification_status = 'verified'
                        AND resolution.responsible_party = 'buyer', false) AS buyer_failed,
               cancel.known_at AS canceled_at
        FROM history
        LEFT JOIN LATERAL (
            SELECT max(e.occurred_at) AS known_at, bool_or(e.event_type = 'delivery_failed') AS failed
            FROM sfd.orders o JOIN sfd.order_events e ON e.order_id = o.order_id AND e.batch_id = %(batch)s
            WHERE o.checkout_id = history.checkout_id AND o.batch_id = %(batch)s
              AND e.event_type IN ('delivery_succeeded', 'delivery_failed')
              AND e.available_at < t.cutoff AND e.occurred_at < t.cutoff
        ) delivery ON true
        LEFT JOIN LATERAL (
            SELECT r.verification_status, r.responsible_party
            FROM sfd.orders o
            JOIN sfd.cases c ON c.order_id = o.order_id AND c.batch_id = %(batch)s
            JOIN sfd.case_resolutions r ON r.case_id = c.case_id AND r.batch_id = %(batch)s
            WHERE o.checkout_id = history.checkout_id AND o.batch_id = %(batch)s
              AND r.available_at < t.cutoff AND r.decided_at < t.cutoff
            ORDER BY r.decided_at DESC, r.resolution_id DESC LIMIT 1
        ) resolution ON true
        LEFT JOIN LATERAL (
            SELECT max(e.occurred_at) AS known_at
            FROM sfd.orders o JOIN sfd.order_events e ON e.order_id = o.order_id AND e.batch_id = %(batch)s
            WHERE o.checkout_id = history.checkout_id AND o.batch_id = %(batch)s
              AND e.event_type = 'canceled' AND e.actor_type = 'buyer'
              AND e.available_at < t.cutoff AND e.occurred_at < t.cutoff
              AND EXISTS (
                SELECT 1 FROM sfd.order_events confirmed
                WHERE confirmed.order_id = o.order_id AND confirmed.batch_id = %(batch)s
                  AND confirmed.event_type = 'confirmed' AND confirmed.available_at < t.cutoff
                  AND confirmed.occurred_at < e.occurred_at
              )
        ) cancel ON true
    )
    SELECT count(*) FILTER (WHERE phone_match AND known_at >= t.cutoff - %(r1_days)s * interval '1 day') AS r1_resolved,
           count(*) FILTER (WHERE phone_match AND buyer_failed AND known_at >= t.cutoff - %(r1_days)s * interval '1 day') AS r1_failed,
           count(*) FILTER (WHERE cluster_match AND known_at >= t.cutoff - %(r2_days)s * interval '1 day') AS r2_resolved,
           count(*) FILTER (WHERE cluster_match AND buyer_failed AND known_at >= t.cutoff - %(r2_days)s * interval '1 day') AS r2_failed,
           count(*) FILTER (WHERE phone_match AND buyer_failed AND known_at >= t.cutoff - %(r3_days)s * interval '1 day') AS r3_failed,
           count(*) FILTER (WHERE (phone_match OR cluster_match) AND placed_at >= t.cutoff - %(r5_hours)s * interval '1 hour') AS r5_checkouts,
           count(DISTINCT buyer_id) FILTER (WHERE cluster_match AND placed_at >= t.cutoff - %(r6_days)s * interval '1 day') AS r6_buyers,
           coalesce(bool_or(buyer_id = t.buyer_id) FILTER (WHERE cluster_match AND placed_at >= t.cutoff - %(r6_days)s * interval '1 day'), false) AS current_buyer_seen,
           count(*) FILTER (WHERE cluster_match AND known_at IS NOT NULL AND placed_at >= t.cutoff - %(r6_days)s * interval '1 day') AS r6_resolved,
           count(*) FILTER (WHERE cluster_match AND buyer_failed AND placed_at >= t.cutoff - %(r6_days)s * interval '1 day') AS r6_failed,
           count(*) FILTER (WHERE phone_match AND canceled_at >= t.cutoff - %(r10_days)s * interval '1 day') AS r10_cancellations
    FROM observed
) h
CROSS JOIN LATERAL (
    SELECT count(DISTINCT o.checkout_id) AS checkouts, count(DISTINCT o.shop_id) AS shops
    FROM sfd.orders o
    WHERE o.batch_id = %(batch)s AND (
      o.checkout_id = t.checkout_id OR o.checkout_id IN (
        SELECT c.checkout_id FROM sfd.checkouts c
        JOIN sfd.checkout_identity_snapshots s ON s.checkout_id = c.checkout_id AND s.batch_id = %(batch)s
        WHERE c.batch_id = %(batch)s AND c.placed_at < t.cutoff
          AND c.placed_at >= t.cutoff - %(r4_hours)s * interval '1 hour'
          AND s.available_at < t.cutoff AND (s.phone_key = t.phone_key OR s.device_key = t.device_key)
      )
    )
) velocity
CROSS JOIN LATERAL (
    WITH amounts AS (
        SELECT sum(i.unit_price * i.quantity + i.freight_amount) AS amount
        FROM sfd.orders o
        JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = %(batch)s
        JOIN sfd.order_items i ON i.order_id = o.order_id AND i.batch_id = %(batch)s
        WHERE o.batch_id = %(batch)s AND o.shop_id = t.shop_id AND c.currency = t.currency
          AND c.placed_at < t.cutoff AND c.placed_at >= t.cutoff - %(r7_days)s * interval '1 day'
          AND NOT EXISTS (
            SELECT 1 FROM sfd.order_events e WHERE e.order_id = o.order_id AND e.batch_id = %(batch)s
              AND e.event_type = 'canceled' AND e.available_at < t.cutoff AND e.occurred_at < t.cutoff
          )
        GROUP BY o.order_id
    ), ranked AS (
        SELECT amount, row_number() OVER (ORDER BY amount) AS position, count(*) OVER () AS total
        FROM amounts
    )
    SELECT count(*) AS n,
           avg(amount) FILTER (WHERE position IN ((total + 1) / 2, (total + 2) / 2)) AS median
    FROM ranked
) baseline
