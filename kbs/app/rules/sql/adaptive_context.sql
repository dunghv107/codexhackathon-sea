WITH targets AS (
    SELECT o.order_id, o.checkout_id, o.shop_id, c.buyer_id, c.placed_at, c.currency,
           s.phone_key, a.address_key
    FROM sfd.orders o
    JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = o.batch_id
    LEFT JOIN sfd.checkout_identity_snapshots s ON s.checkout_id = c.checkout_id
        AND s.batch_id = c.batch_id AND s.available_at <= c.placed_at
    LEFT JOIN sfd.checkout_addresses a ON a.checkout_id = c.checkout_id AND a.batch_id = c.batch_id
    WHERE o.batch_id = %(batch)s AND o.shop_id = %(shop_id)s AND o.order_id = ANY(%(order_ids)s::text[])
)
SELECT t.order_id,
       jsonb_build_object(
           'target', to_jsonb(t) || jsonb_build_object('amount', current_items.amount::text,
                                                    'quantities', current_items.quantities),
           'history', coalesce(history.rows, '[]'::jsonb)
       ) AS adaptive_context
FROM targets t
CROSS JOIN LATERAL (
    SELECT sum(amount) AS amount, jsonb_object_agg(listing_id, quantity) AS quantities
    FROM (
        SELECT listing_id, sum(quantity) AS quantity, sum(unit_price * quantity + freight_amount) AS amount
        FROM sfd.order_items WHERE order_id = t.order_id AND batch_id = %(batch)s GROUP BY listing_id
    ) items
) current_items
CROSS JOIN LATERAL (
    SELECT jsonb_agg(to_jsonb(h) ORDER BY h.placed_at, h.order_id) AS rows
    FROM (
        SELECT o.order_id, o.checkout_id, o.shop_id, c.buyer_id, c.placed_at, c.currency,
               i.amount::text, i.quantities, s.phone_key, a.address_key,
               terminal.known_at AS terminal_at, delivery.event_type AS outcome,
               delivery.known_at AS outcome_available_at, resolution.responsible_party,
               coalesce(resolution.verification_status = 'verified', false) AS resolution_verified,
               resolution.known_at AS resolution_available_at
        FROM sfd.orders o
        JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = o.batch_id
        LEFT JOIN sfd.checkout_identity_snapshots s ON s.checkout_id = c.checkout_id
            AND s.batch_id = c.batch_id AND s.available_at < t.placed_at
        LEFT JOIN sfd.checkout_addresses a ON a.checkout_id = c.checkout_id AND a.batch_id = c.batch_id
        CROSS JOIN LATERAL (
            SELECT sum(amount) AS amount, jsonb_object_agg(listing_id, quantity) AS quantities
            FROM (
                SELECT listing_id, sum(quantity) AS quantity, sum(unit_price * quantity + freight_amount) AS amount
                FROM sfd.order_items WHERE order_id = o.order_id AND batch_id = %(batch)s GROUP BY listing_id
            ) items
        ) i
        LEFT JOIN LATERAL (
            SELECT min(greatest(e.occurred_at, e.available_at)) AS known_at
            FROM sfd.order_events e
            WHERE e.order_id = o.order_id AND e.batch_id = %(batch)s
              AND e.event_type IN ('delivery_succeeded', 'delivery_failed', 'canceled')
              AND e.occurred_at < t.placed_at AND e.available_at < t.placed_at
        ) terminal ON true
        LEFT JOIN LATERAL (
            SELECT e.event_type, greatest(e.occurred_at, e.available_at) AS known_at
            FROM sfd.order_events e
            WHERE e.order_id = o.order_id AND e.batch_id = %(batch)s
              AND e.event_type IN ('delivery_succeeded', 'delivery_failed')
              AND e.occurred_at < t.placed_at AND e.available_at < t.placed_at
            ORDER BY e.occurred_at DESC, e.event_id DESC LIMIT 1
        ) delivery ON true
        LEFT JOIN LATERAL (
            SELECT r.verification_status, r.responsible_party,
                   greatest(r.decided_at, r.available_at, cs.opened_at, cs.available_at) AS known_at
            FROM sfd.cases cs
            JOIN sfd.case_resolutions r ON r.case_id = cs.case_id AND r.batch_id = cs.batch_id
            WHERE cs.order_id = o.order_id AND cs.batch_id = %(batch)s AND cs.case_type = 'delivery'
              AND cs.opened_at < t.placed_at AND cs.available_at < t.placed_at
              AND r.decided_at < t.placed_at AND r.available_at < t.placed_at
            ORDER BY r.decided_at DESC, r.resolution_id DESC LIMIT 1
        ) resolution ON true
        WHERE o.batch_id = %(batch)s AND c.currency = t.currency
          AND (o.shop_id = t.shop_id OR c.buyer_id = t.buyer_id)
          AND c.placed_at < t.placed_at
          AND c.placed_at >= t.placed_at - %(lookback_days)s * interval '1 day'
              - %(window_hours)s * interval '1 hour'
          AND i.amount IS NOT NULL
    ) h
) history
