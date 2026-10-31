CREATE UNIQUE INDEX IF NOT EXISTS checkout_batch_contact ON sfd.checkouts(batch_id, checkout_id);
CREATE TABLE IF NOT EXISTS sfd.checkout_contacts (
    checkout_id text PRIMARY KEY,
    batch_id text NOT NULL,
    phone_e164 text NOT NULL CHECK (phone_e164 ~ '^\+84[1-9][0-9]{8,9}$'),
    key_source text NOT NULL CHECK (key_source IN ('demo_mapping','hmac')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (batch_id, checkout_id) REFERENCES sfd.checkouts(batch_id, checkout_id)
);

CREATE TABLE IF NOT EXISTS sfd.demo_phone_mappings (
    batch_id text NOT NULL CHECK (batch_id = 'olist_vn_sim') REFERENCES sfd.data_batches(batch_id),
    phone_e164 text NOT NULL CHECK (phone_e164 ~ '^\+84[1-9][0-9]{8,9}$'),
    phone_key text NOT NULL CHECK (length(phone_key) > 0),
    scenario_label text NOT NULL,
    source_reference text NOT NULL,
    PRIMARY KEY (batch_id, phone_e164)
);

CREATE UNIQUE INDEX IF NOT EXISTS risk_assessment_order_batch ON sfd.risk_assessments(batch_id, assessment_id, order_id);
CREATE TABLE IF NOT EXISTS sfd.order_verifications (
    verification_id text PRIMARY KEY,
    batch_id text NOT NULL CHECK (batch_id = 'olist_vn_sim'),
    order_id text NOT NULL,
    assessment_id text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','declined','unanswered')),
    channel text NOT NULL DEFAULT 'demo' CHECK (channel = 'demo'),
    response_source text CHECK (response_source = 'dtmf'),
    answered_at timestamptz,
    handled_by_seller_id text REFERENCES sfd.sellers(seller_id),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (batch_id, order_id),
    FOREIGN KEY (batch_id, order_id) REFERENCES sfd.orders(batch_id, order_id),
    FOREIGN KEY (batch_id, assessment_id, order_id)
        REFERENCES sfd.risk_assessments(batch_id, assessment_id, order_id),
    CHECK ((status = 'pending' AND answered_at IS NULL AND response_source IS NULL AND handled_by_seller_id IS NULL)
        OR (status <> 'pending' AND answered_at IS NOT NULL AND response_source IS NOT NULL AND handled_by_seller_id IS NOT NULL))
);
