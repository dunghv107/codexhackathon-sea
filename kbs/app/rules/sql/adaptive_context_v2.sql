WITH targets AS (
    SELECT o.order_id, o.checkout_id, o.shop_id, c.buyer_id, c.placed_at, c.currency,
           s.phone_key, a.address_key, c.payment_method, b.account_created_at, track.available_at AS tracking_from
    FROM sfd.orders o
    JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = o.batch_id
    JOIN sfd.buyers b ON b.buyer_id = c.buyer_id AND b.batch_id = c.batch_id
    LEFT JOIN sfd.order_risk_tracking track ON track.order_id = o.order_id AND track.batch_id = o.batch_id
        AND track.available_at <= c.placed_at
    LEFT JOIN sfd.checkout_identity_snapshots s ON s.checkout_id = c.checkout_id
        AND s.batch_id = c.batch_id AND s.available_at <= c.placed_at
    LEFT JOIN sfd.checkout_addresses a ON a.checkout_id = c.checkout_id AND a.batch_id = c.batch_id
    WHERE o.batch_id = %(batch)s AND o.shop_id = %(shop_id)s AND o.order_id = ANY(%(order_ids)s::text[])
), identity_context AS (
    SELECT t.order_id, coalesce(jsonb_agg(to_jsonb(i)) FILTER (WHERE i.related_buyer_id IS NOT NULL), '[]'::jsonb) AS links,
           coalesce(array_agg(DISTINCT i.related_buyer_id) FILTER (WHERE i.related_buyer_id IS NOT NULL), ARRAY[]::text[]) AS buyers
    FROM targets t LEFT JOIN LATERAL (
        SELECT q.related_buyer_id, q.edge_type, q.occurred_at, q.available_at, q.source_ref
        FROM (
            SELECT DISTINCT ON (a.buyer_id,a.related_buyer_id,a.edge_type,a.source_ref)
                CASE WHEN a.buyer_id=t.buyer_id THEN a.related_buyer_id ELSE a.buyer_id END AS related_buyer_id,
                a.edge_type,a.occurred_at,a.available_at,a.source_ref,a.status
            FROM sfd.risk_identity_assertions a
            WHERE a.batch_id=%(batch)s AND t.buyer_id IN (a.buyer_id,a.related_buyer_id)
              AND a.occurred_at<t.placed_at AND a.available_at<t.placed_at
            ORDER BY a.buyer_id,a.related_buyer_id,a.edge_type,a.source_ref,a.occurred_at DESC,a.record_id DESC
        ) q WHERE q.status='verified'
    ) i ON true GROUP BY t.order_id
)
SELECT t.order_id,
       jsonb_build_object(
           'target', to_jsonb(t) || jsonb_build_object('amount', current_items.amount::text,
                                                    'quantities', current_items.quantities, 'categories', current_categories.values),
           'history', coalesce(history.rows, '[]'::jsonb),
           'identity_links', identity_context.links,
           'checkout_shops', (SELECT jsonb_agg(shop_id) FROM sfd.orders WHERE checkout_id=t.checkout_id AND batch_id=%(batch)s),
           'enforcements', coalesce(enforcements.rows, '[]'::jsonb),
           'evidence_reviews', coalesce(evidence.rows, '[]'::jsonb), 'evidence_count', evidence.owned_count
       ) AS adaptive_context
FROM targets t
JOIN identity_context ON identity_context.order_id=t.order_id
CROSS JOIN LATERAL (
    SELECT sum(amount) AS amount, jsonb_object_agg(listing_id, quantity) AS quantities
    FROM (
        SELECT listing_id, sum(quantity) AS quantity, sum(unit_price * quantity + freight_amount) AS amount
        FROM sfd.order_items WHERE order_id = t.order_id AND batch_id = %(batch)s GROUP BY listing_id
    ) items
) current_items
CROSS JOIN LATERAL (
    SELECT CASE WHEN bool_and(s.item_id IS NOT NULL AND s.category_code IS NOT NULL)
                THEN jsonb_agg(DISTINCT s.category_code) END AS values
    FROM sfd.order_items i LEFT JOIN sfd.order_item_risk_snapshots s
    ON s.item_id=i.item_id AND s.batch_id=i.batch_id AND s.available_at<=t.placed_at
    WHERE i.order_id=t.order_id AND i.batch_id=%(batch)s
) current_categories
CROSS JOIN LATERAL (
    SELECT jsonb_agg(to_jsonb(h) ORDER BY h.placed_at, h.order_id) AS rows
    FROM (
        SELECT o.order_id, o.checkout_id, o.shop_id, c.buyer_id, c.placed_at, c.currency,
               i.amount::text, i.quantities, s.phone_key, a.address_key,
               terminal.known_at AS terminal_at, delivery.event_type AS outcome,
               delivery.known_at AS outcome_available_at, resolution.responsible_party,
               coalesce(resolution.verification_status = 'verified', false) AS resolution_verified,
               resolution.known_at AS resolution_available_at, c.payment_method,
               b.account_created_at, categories.values AS categories, track.available_at AS tracking_from,
               closed.known_at AS cod_closed_at, confirmed.occurred_at AS confirmed_at,
               canceled.occurred_at AS cancellation_at, cancel_resolution.responsible_party AS cancellation_party,
               coalesce(cancel_resolution.verification_status='verified' AND cancel_resolution.outcome_code IN ('cancellation','legitimate'),false) AS cancellation_verified,
               greatest(cancel_resolution.known_at,confirmed.known_at,canceled.known_at) AS cancellation_known_at,
               after_sale.present AS after_sale_present, after_sale.verified AS after_sale_verified,
               after_sale.party AS after_sale_party, after_sale.code AS after_sale_code,
               after_sale.known_at AS after_sale_known_at, seller_case.verified AS seller_case,
               seller_case.known_at AS seller_case_known_at
        FROM sfd.orders o
        JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = o.batch_id
        JOIN sfd.buyers b ON b.buyer_id=c.buyer_id AND b.batch_id=c.batch_id
        LEFT JOIN sfd.order_risk_tracking track ON track.order_id=o.order_id AND track.batch_id=o.batch_id AND track.available_at<t.placed_at
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
        CROSS JOIN LATERAL (
            SELECT CASE WHEN bool_and(s.item_id IS NOT NULL AND s.category_code IS NOT NULL)
                        THEN jsonb_agg(DISTINCT s.category_code) END AS values
            FROM sfd.order_items item LEFT JOIN sfd.order_item_risk_snapshots s
            ON s.item_id=item.item_id AND s.batch_id=item.batch_id AND s.available_at<=c.placed_at
            WHERE item.order_id=o.order_id AND item.batch_id=%(batch)s
        ) categories
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
        LEFT JOIN LATERAL (
            SELECT min(greatest(e.occurred_at,e.available_at)) AS known_at FROM sfd.order_events e
            WHERE e.order_id=o.order_id AND e.batch_id=%(batch)s
              AND e.event_type IN ('delivery_succeeded','canceled','returned')
              AND e.occurred_at<t.placed_at AND e.available_at<t.placed_at
        ) closed ON true
        LEFT JOIN LATERAL (
            SELECT min(e.occurred_at) AS occurred_at, max(e.available_at) AS known_at FROM sfd.order_events e
            WHERE e.order_id=o.order_id AND e.batch_id=%(batch)s AND e.event_type='confirmed'
              AND e.occurred_at<t.placed_at AND e.available_at<t.placed_at
        ) confirmed ON true
        LEFT JOIN LATERAL (
            SELECT min(e.occurred_at) AS occurred_at, max(e.available_at) AS known_at FROM sfd.order_events e
            WHERE e.order_id=o.order_id AND e.batch_id=%(batch)s AND e.event_type='canceled'
              AND e.occurred_at>confirmed.occurred_at AND e.occurred_at<t.placed_at AND e.available_at<t.placed_at
        ) canceled ON true
        LEFT JOIN LATERAL (
            SELECT r.responsible_party,r.verification_status,a.outcome_code,
                   greatest(r.available_at,r.decided_at,cs.available_at,a.available_at) AS known_at
            FROM sfd.cases cs JOIN sfd.case_resolutions r ON r.case_id=cs.case_id AND r.batch_id=cs.batch_id
            LEFT JOIN sfd.risk_case_outcomes a ON a.resolution_id=r.resolution_id AND a.batch_id=r.batch_id AND a.available_at<t.placed_at
            WHERE cs.order_id=o.order_id AND cs.batch_id=%(batch)s AND cs.case_type='cancellation'
              AND cs.opened_at<t.placed_at AND cs.available_at<t.placed_at
              AND r.decided_at<t.placed_at AND r.available_at<t.placed_at
            ORDER BY r.decided_at DESC,r.resolution_id DESC LIMIT 1
        ) cancel_resolution ON true
        CROSS JOIN LATERAL (
            SELECT count(*)>0 AS present,
                   coalesce(bool_and(coalesce(q.verification_status='verified' AND q.outcome_code IS NOT NULL,false)),false) AS verified,
                   CASE WHEN bool_or(q.verification_status='verified' AND q.responsible_party='buyer'
                                 AND q.outcome_code IN ('return_abuse','refund_abuse')) THEN 'buyer' ELSE 'unknown' END AS party,
                   CASE WHEN bool_or(q.verification_status='verified' AND q.responsible_party='buyer'
                                 AND q.outcome_code IN ('return_abuse','refund_abuse')) THEN 'return_abuse' ELSE 'legitimate' END AS code,
                   max(q.known_at) AS known_at
            FROM (
                SELECT cs.case_id,r.verification_status,r.responsible_party,a.outcome_code,
                       greatest(cs.available_at,r.available_at,r.decided_at,a.available_at) AS known_at
                FROM sfd.cases cs LEFT JOIN LATERAL (
                    SELECT r.* FROM sfd.case_resolutions r WHERE r.case_id=cs.case_id AND r.batch_id=cs.batch_id
                    AND r.available_at<t.placed_at AND r.decided_at<t.placed_at
                    ORDER BY r.decided_at DESC,r.resolution_id DESC LIMIT 1
                ) r ON true
                LEFT JOIN sfd.risk_case_outcomes a ON a.resolution_id=r.resolution_id AND a.batch_id=r.batch_id AND a.available_at<t.placed_at
                WHERE cs.order_id=o.order_id AND cs.batch_id=%(batch)s AND cs.case_type IN ('return','refund')
                  AND cs.opened_at<t.placed_at AND cs.available_at<t.placed_at
            ) q
        ) after_sale
        CROSS JOIN LATERAL (
            SELECT coalesce(bool_or(r.verification_status='verified' AND r.responsible_party='seller'),false) AS verified,
                   max(greatest(cs.available_at,r.available_at,r.decided_at)) FILTER (WHERE r.verification_status='verified' AND r.responsible_party='seller') AS known_at
            FROM sfd.cases cs LEFT JOIN LATERAL (
                SELECT r.* FROM sfd.case_resolutions r WHERE r.case_id=cs.case_id AND r.batch_id=cs.batch_id
                AND r.available_at<t.placed_at AND r.decided_at<t.placed_at
                ORDER BY r.decided_at DESC,r.resolution_id DESC LIMIT 1
            ) r ON true
            WHERE cs.order_id=o.order_id AND cs.batch_id=%(batch)s AND cs.opened_at<t.placed_at AND cs.available_at<t.placed_at
        ) seller_case
        WHERE o.batch_id = %(batch)s AND c.currency = t.currency
          AND (o.shop_id = t.shop_id OR c.buyer_id = t.buyer_id OR c.buyer_id=ANY(identity_context.buyers))
          AND c.placed_at < t.placed_at
          AND (c.buyer_id=t.buyer_id OR c.placed_at >= t.placed_at - %(lookback_days)s * interval '1 day'
              - %(window_hours)s * interval '1 hour')
          AND i.amount IS NOT NULL
    ) h
) history

CROSS JOIN LATERAL (
    SELECT jsonb_agg(to_jsonb(e)) AS rows FROM (
        SELECT DISTINCT ON (buyer_id,abuse_code) buyer_id,abuse_code,action,occurred_at,available_at,record_id
        FROM sfd.risk_enforcement_events WHERE batch_id=%(batch)s AND buyer_id=ANY(identity_context.buyers)
          AND occurred_at<t.placed_at AND available_at<t.placed_at
        ORDER BY buyer_id,abuse_code,occurred_at DESC,record_id DESC
    ) e
) enforcements
CROSS JOIN LATERAL (
    WITH known AS (
        SELECT e.evidence_id,cs.case_id,cs.order_id,c.buyer_id,r.content_sha256,r.verdict,
               greatest(e.available_at,cs.available_at,r.available_at,r.occurred_at) AS available_at
        FROM sfd.case_evidence e JOIN sfd.cases cs ON cs.case_id=e.case_id AND cs.batch_id=e.batch_id
        JOIN sfd.orders o ON o.order_id=cs.order_id AND o.batch_id=cs.batch_id
        JOIN sfd.checkouts c ON c.checkout_id=o.checkout_id AND c.batch_id=o.batch_id
        LEFT JOIN LATERAL (
            SELECT r.* FROM sfd.risk_evidence_reviews r WHERE r.evidence_id=e.evidence_id AND r.batch_id=e.batch_id
              AND r.available_at<t.placed_at AND r.occurred_at<t.placed_at
            ORDER BY r.occurred_at DESC,r.record_id DESC LIMIT 1
        ) r ON true
        WHERE e.batch_id=%(batch)s AND e.available_at<t.placed_at AND cs.available_at<t.placed_at
          AND cs.opened_at<t.placed_at AND c.placed_at<t.placed_at
          AND cs.opened_at>=t.placed_at-%(evidence_days)s * interval '1 day'
    )
    SELECT count(*) AS owned_count,
           jsonb_agg(jsonb_build_object('evidence_id',k.evidence_id,'verdict',k.verdict,'available_at',k.available_at,
               'independent_cases', (SELECT count(DISTINCT other.case_id) FROM known other
                    WHERE other.content_sha256=k.content_sha256 AND other.order_id<>k.order_id AND other.verdict<>'unresolved')))
               FILTER (WHERE k.verdict IS NOT NULL) AS rows
    FROM known k WHERE k.buyer_id=t.buyer_id
) evidence
