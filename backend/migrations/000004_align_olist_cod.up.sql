-- Align existing marketplace databases and accept an already-loaded canonical Olist schema.
BEGIN;
SET LOCAL search_path TO sfd, public;

-- Authentication is an app extension; imported actors and source provenance remain intact.
ALTER TABLE buyers ADD COLUMN IF NOT EXISTS auth_subject text UNIQUE;
ALTER TABLE buyers ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active','suspended'));
CREATE TABLE IF NOT EXISTS sellers (
    seller_id text PRIMARY KEY, auth_subject text UNIQUE, display_name text NOT NULL,
    account_created_at timestamptz, status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','suspended')),
    batch_id text NOT NULL REFERENCES data_batches(batch_id), source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id)
);
ALTER TABLE shops ADD COLUMN IF NOT EXISTS owner_actor_ref text;
ALTER TABLE shops ADD COLUMN IF NOT EXISTS owner_seller_id text REFERENCES sellers(seller_id);
ALTER TABLE shops ALTER COLUMN owner_seller_id DROP NOT NULL;
UPDATE shops SET owner_actor_ref = owner_seller_id WHERE owner_actor_ref IS NULL;
CREATE INDEX IF NOT EXISTS idx_shops_owner ON shops(owner_seller_id);
ALTER TABLE review_decisions ADD COLUMN IF NOT EXISTS seller_id text REFERENCES sellers(seller_id);
-- Imported seller decisions use actor_ref; they need not belong to a local login.
DO $$ DECLARE c record; BEGIN
    FOR c IN SELECT conname FROM pg_constraint
        WHERE conrelid = 'review_decisions'::regclass AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%seller_id%'
    LOOP EXECUTE format('ALTER TABLE review_decisions DROP CONSTRAINT %I', c.conname); END LOOP;
END $$;

ALTER TABLE checkout_addresses ADD COLUMN IF NOT EXISTS ward_code text;
ALTER TABLE checkout_addresses ADD COLUMN IF NOT EXISTS house_no text;
ALTER TABLE checkout_addresses ADD COLUMN IF NOT EXISTS parse_ok boolean;
ALTER TABLE order_events DROP CONSTRAINT IF EXISTS order_events_event_type_check;
ALTER TABLE order_events ADD CONSTRAINT order_events_event_type_check
    CHECK (event_type IN ('created','confirmed','address_changed','canceled','delivery_failed','delivery_succeeded','return_requested'));

ALTER TABLE case_analyses ADD COLUMN IF NOT EXISTS batch_id text REFERENCES data_batches(batch_id);
ALTER TABLE risk_assessments ADD COLUMN IF NOT EXISTS batch_id text REFERENCES data_batches(batch_id);
ALTER TABLE review_decisions ADD COLUMN IF NOT EXISTS batch_id text REFERENCES data_batches(batch_id);
ALTER TABLE rule_versions ADD COLUMN IF NOT EXISTS batch_id text REFERENCES data_batches(batch_id);
ALTER TABLE rule_proposals ADD COLUMN IF NOT EXISTS batch_id text REFERENCES data_batches(batch_id);
UPDATE case_analyses a SET batch_id = c.batch_id FROM cases c WHERE a.case_id=c.case_id AND a.batch_id IS NULL;
UPDATE risk_assessments a SET batch_id = o.batch_id FROM orders o WHERE a.order_id=o.order_id AND a.batch_id IS NULL;
UPDATE review_decisions d SET batch_id = o.batch_id FROM orders o WHERE d.order_id=o.order_id AND d.batch_id IS NULL;
-- Ambiguous or unused legacy versions fail NOT NULL below; never invent their provenance.
UPDATE rule_versions v SET batch_id = (
    SELECT min(a.batch_id) FROM risk_assessments a WHERE a.rule_version=v.rule_version
    HAVING count(DISTINCT a.batch_id)=1
) WHERE v.batch_id IS NULL;
UPDATE rule_proposals p SET batch_id=v.batch_id FROM rule_versions v
    WHERE p.base_rule_version=v.rule_version AND p.batch_id IS NULL;
ALTER TABLE case_analyses ALTER COLUMN batch_id SET NOT NULL;
ALTER TABLE risk_assessments ALTER COLUMN batch_id SET NOT NULL;
ALTER TABLE review_decisions ALTER COLUMN batch_id SET NOT NULL;
ALTER TABLE rule_versions ALTER COLUMN batch_id SET NOT NULL;
ALTER TABLE rule_proposals ALTER COLUMN batch_id SET NOT NULL;
-- An unknown historical cutoff cannot safely be reconstructed from a current checkout.
ALTER TABLE risk_assessments ALTER COLUMN input_cutoff_at SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS olist_orders_batch ON orders(order_id,batch_id);
CREATE UNIQUE INDEX IF NOT EXISTS olist_cases_batch ON cases(case_id,batch_id);
CREATE UNIQUE INDEX IF NOT EXISTS olist_rules_batch ON rule_versions(rule_version,batch_id);
CREATE UNIQUE INDEX IF NOT EXISTS olist_assessments_batch ON risk_assessments(assessment_id,batch_id);
DO $$ DECLARE c record; BEGIN
    FOR c IN SELECT * FROM (VALUES
        ('case_analyses','olist_analysis_case','FOREIGN KEY (case_id,batch_id) REFERENCES cases(case_id,batch_id)'),
        ('risk_assessments','olist_assessment_order','FOREIGN KEY (order_id,batch_id) REFERENCES orders(order_id,batch_id)'),
        ('risk_assessments','olist_assessment_rule','FOREIGN KEY (rule_version,batch_id) REFERENCES rule_versions(rule_version,batch_id)'),
        ('review_decisions','olist_decision_order','FOREIGN KEY (order_id,batch_id) REFERENCES orders(order_id,batch_id)'),
        ('review_decisions','olist_decision_assessment','FOREIGN KEY (assessment_id,batch_id) REFERENCES risk_assessments(assessment_id,batch_id)'),
        ('rule_proposals','olist_proposal_rule','FOREIGN KEY (base_rule_version,batch_id) REFERENCES rule_versions(rule_version,batch_id)')
    ) AS constraints(table_name, constraint_name, definition)
    LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid=c.table_name::regclass AND conname=c.constraint_name) THEN
            EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I %s',c.table_name,c.constraint_name,c.definition);
        END IF;
    END LOOP;
END $$;
CREATE INDEX IF NOT EXISTS idx_events_type_available ON order_events(order_id,event_type,available_at);
CREATE INDEX IF NOT EXISTS idx_checkouts_placed ON checkouts(placed_at);
INSERT INTO data_batches(batch_id,source_system,is_synthetic,source_reference,loaded_at)
VALUES ('app','marketplace',false,'User-created marketplace records',now()) ON CONFLICT DO NOTHING;
COMMIT;
