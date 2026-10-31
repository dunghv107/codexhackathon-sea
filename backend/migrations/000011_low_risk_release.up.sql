BEGIN;

-- Read decisions after concurrent order writers finish, so a seller Hold cannot be missed.
LOCK TABLE sfd.orders IN EXCLUSIVE MODE;

-- Release existing Low orders using their latest assessment; retain manual holds and fulfillment history.
WITH eligible AS (
    SELECT o.order_id, o.batch_id, a.assessment_id, a.rule_version
    FROM sfd.orders o
    CROSS JOIN LATERAL (
        SELECT assessment_id, rule_version, risk_level
        FROM sfd.risk_assessments
        WHERE order_id = o.order_id AND batch_id = o.batch_id
        ORDER BY assessed_at DESC, assessment_id DESC
        LIMIT 1
    ) a
    WHERE o.batch_id = 'olist_vn_sim' AND o.fulfillment_status = 'unfulfilled'
      AND a.risk_level = 'low' AND o.review_status IN ('pending', 'hold')
      AND (o.review_status = 'pending' OR (
          SELECT d.actor_type = 'policy' AND d.action IN ('hold', 'keep_hold')
          FROM sfd.review_decisions d
          WHERE d.order_id = o.order_id AND d.batch_id = o.batch_id
          ORDER BY d.created_at DESC, d.decision_id DESC
          LIMIT 1
      ))
    ORDER BY o.order_id
    FOR UPDATE OF o
), released AS (
    UPDATE sfd.orders o SET review_status = 'released'
    FROM eligible e
    WHERE o.order_id = e.order_id AND o.batch_id = e.batch_id
    RETURNING o.order_id, o.batch_id, e.assessment_id, e.rule_version
)
INSERT INTO sfd.review_decisions
    (batch_id, decision_id, order_id, assessment_id, actor_type, actor_ref, action, reason, created_at)
SELECT batch_id, 'live:' || gen_random_uuid()::text, order_id, assessment_id,
       'policy', rule_version, 'release', 'Low risk score', clock_timestamp()
FROM released;

COMMIT;
