BEGIN;
SET LOCAL search_path TO sfd, public;

ALTER TABLE order_events DROP CONSTRAINT IF EXISTS order_events_event_type_check;
ALTER TABLE order_events ADD CONSTRAINT order_events_event_type_check CHECK (event_type IN
    ('created','confirmed','address_changed','packing','shipped','canceled','delivery_failed',
     'delivery_succeeded','return_requested','returned'));
ALTER TABLE cases DROP CONSTRAINT IF EXISTS cases_case_type_check;
ALTER TABLE cases ADD CONSTRAINT cases_case_type_check
    CHECK (case_type IN ('delivery','cancellation','return','refund','other'));

CREATE UNIQUE INDEX IF NOT EXISTS risk_buyers_batch ON buyers(buyer_id,batch_id);
CREATE UNIQUE INDEX IF NOT EXISTS risk_items_batch ON order_items(item_id,batch_id);
CREATE UNIQUE INDEX IF NOT EXISTS risk_resolutions_batch ON case_resolutions(resolution_id,batch_id);
CREATE UNIQUE INDEX IF NOT EXISTS risk_evidence_batch ON case_evidence(evidence_id,batch_id);

-- Only new checkouts get snapshots; current categories cannot reconstruct old purchases.
CREATE TABLE order_item_risk_snapshots (
    item_id text NOT NULL, batch_id text NOT NULL, category_code text,
    available_at timestamptz NOT NULL,
    PRIMARY KEY (item_id,batch_id),
    FOREIGN KEY (item_id,batch_id) REFERENCES order_items(item_id,batch_id)
);
CREATE TABLE order_risk_tracking (
    order_id text NOT NULL, batch_id text NOT NULL, available_at timestamptz NOT NULL,
    PRIMARY KEY (order_id,batch_id),
    FOREIGN KEY (order_id,batch_id) REFERENCES orders(order_id,batch_id)
);

CREATE TABLE risk_identity_assertions (
    record_id text PRIMARY KEY, batch_id text NOT NULL,
    buyer_id text NOT NULL, related_buyer_id text NOT NULL,
    edge_type text NOT NULL CHECK (edge_type IN ('verified_document','verified_payment','verified_account')),
    status text NOT NULL CHECK (status IN ('verified','revoked')),
    occurred_at timestamptz NOT NULL, available_at timestamptz NOT NULL,
    source_ref text NOT NULL CHECK (length(source_ref) BETWEEN 1 AND 500), recorded_by text NOT NULL,
    CHECK (buyer_id < related_buyer_id), CHECK (available_at >= occurred_at),
    FOREIGN KEY (buyer_id,batch_id) REFERENCES buyers(buyer_id,batch_id),
    FOREIGN KEY (related_buyer_id,batch_id) REFERENCES buyers(buyer_id,batch_id)
);
CREATE INDEX risk_identity_asof ON risk_identity_assertions(batch_id,buyer_id,related_buyer_id,occurred_at);
CREATE INDEX risk_identity_related ON risk_identity_assertions(batch_id,related_buyer_id,occurred_at);

CREATE TABLE risk_enforcement_events (
    record_id text PRIMARY KEY, batch_id text NOT NULL, buyer_id text NOT NULL,
    action text NOT NULL CHECK (action IN ('restricted','reinstated')),
    abuse_code text NOT NULL CHECK (abuse_code IN ('delivery','cancellation','after_sale')),
    occurred_at timestamptz NOT NULL, available_at timestamptz NOT NULL,
    source_ref text NOT NULL CHECK (length(source_ref) BETWEEN 1 AND 500), recorded_by text NOT NULL,
    CHECK (available_at >= occurred_at),
    FOREIGN KEY (buyer_id,batch_id) REFERENCES buyers(buyer_id,batch_id)
);
CREATE INDEX risk_enforcement_asof ON risk_enforcement_events(batch_id,buyer_id,abuse_code,occurred_at);

CREATE TABLE risk_case_outcomes (
    resolution_id text NOT NULL, batch_id text NOT NULL,
    outcome_code text NOT NULL CHECK (outcome_code IN ('delivery_failure','cancellation','return_abuse','refund_abuse','legitimate')),
    available_at timestamptz NOT NULL, source_ref text NOT NULL, recorded_by text NOT NULL,
    PRIMARY KEY (resolution_id,batch_id),
    FOREIGN KEY (resolution_id,batch_id) REFERENCES case_resolutions(resolution_id,batch_id)
);
CREATE TABLE risk_evidence_reviews (
    record_id text PRIMARY KEY, batch_id text NOT NULL, evidence_id text NOT NULL,
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    verdict text NOT NULL CHECK (verdict IN ('contradiction_verified','no_contradiction','unresolved')),
    occurred_at timestamptz NOT NULL, available_at timestamptz NOT NULL,
    source_ref text NOT NULL CHECK (length(source_ref) BETWEEN 1 AND 500), recorded_by text NOT NULL,
    CHECK (available_at >= occurred_at),
    FOREIGN KEY (evidence_id,batch_id) REFERENCES case_evidence(evidence_id,batch_id)
);
CREATE INDEX risk_evidence_asof ON risk_evidence_reviews(batch_id,evidence_id,occurred_at);
CREATE INDEX risk_evidence_hash ON risk_evidence_reviews(batch_id,content_sha256);

-- Platform-only signals never enter the seller's feature_snapshot.
CREATE TABLE risk_platform_observations (
    assessment_id text NOT NULL, batch_id text NOT NULL, signals jsonb NOT NULL,
    PRIMARY KEY (assessment_id,batch_id),
    FOREIGN KEY (assessment_id,batch_id) REFERENCES risk_assessments(assessment_id,batch_id),
    CHECK (jsonb_typeof(signals) = 'array')
);
COMMIT;
