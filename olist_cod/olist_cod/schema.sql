-- PostgreSQL design v1; new isolated schema, never drops existing data.
-- Execute once in an empty target; app must SET search_path TO sfd, public.
BEGIN;
CREATE SCHEMA sfd;
SET LOCAL search_path TO sfd, public;

-- Nguồn dữ liệu; phân biệt nguồn thật và mô phỏng
CREATE TABLE data_batches (
    batch_id text PRIMARY KEY,
    source_system text NOT NULL,
    is_synthetic boolean NOT NULL,
    source_reference text NOT NULL,
    source_sha256 text,
    loaded_at timestamptz NOT NULL,
    CHECK (source_sha256 IS NULL OR length(source_sha256) = 64)
);

-- Một người mua theo namespace nguồn
CREATE TABLE buyers (
    buyer_id text PRIMARY KEY,
    display_name text,
    account_created_at timestamptz,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id)
);

-- Một shop với actor quản lý demo
CREATE TABLE shops (
    shop_id text PRIMARY KEY,
    display_name text NOT NULL,
    owner_actor_ref text,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id)
);

-- Thông tin sản phẩm; thiếu tên nguồn được giữ riêng
CREATE TABLE products (
    product_id text PRIMARY KEY,
    display_name text,
    category_code text,
    category_name_en text,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id)
);

-- Sản phẩm do một shop bán; giá hiện tại chỉ phục vụ demo
CREATE TABLE listings (
    listing_id text PRIMARY KEY,
    shop_id text NOT NULL REFERENCES shops(shop_id),
    product_id text NOT NULL REFERENCES products(product_id),
    current_price numeric(18,2),
    currency text NOT NULL,
    stock_quantity integer,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (listing_id, shop_id),
    UNIQUE (shop_id, product_id),
    CHECK (current_price IS NULL OR current_price >= 0),
    CHECK (stock_quantity IS NULL OR stock_quantity >= 0),
    CHECK (length(currency) = 3)
);

-- Một lần mua nhiều shop; đơn vị đếm tần suất mua
CREATE TABLE checkouts (
    checkout_id text PRIMARY KEY,
    buyer_id text NOT NULL REFERENCES buyers(buyer_id),
    placed_at timestamptz NOT NULL,
    payment_method text NOT NULL,
    currency text NOT NULL,
    total_amount numeric(18,2) NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id),
    CHECK (total_amount >= 0),
    CHECK (length(currency) = 3),
    CHECK (payment_method IN ('cod','credit_card','boleto','voucher','debit_card','mixed','not_defined','simulated'))
);

-- Snapshot nơi nhận của một checkout; không phải địa chỉ hiện tại buyer
CREATE TABLE checkout_addresses (
    checkout_id text PRIMARY KEY REFERENCES checkouts(checkout_id),
    address_raw text,
    address_key text,
    ward_code text,
    house_no text,
    parse_ok boolean,
    postal_prefix text,
    city text,
    region text,
    country_code text,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL
);

-- Một đơn của một shop sau checkout; đơn vị Hold/Release
CREATE TABLE orders (
    order_id text PRIMARY KEY,
    checkout_id text NOT NULL REFERENCES checkouts(checkout_id),
    shop_id text NOT NULL REFERENCES shops(shop_id),
    fulfillment_status text NOT NULL DEFAULT 'unfulfilled',
    review_status text NOT NULL DEFAULT 'pending',
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (checkout_id, shop_id),
    UNIQUE (order_id, shop_id),
    CHECK (fulfillment_status IN ('unfulfilled','packing','shipped','delivered','canceled','returned')),
    CHECK (review_status IN ('pending','hold','released')),
    CHECK (fulfillment_status IN ('unfulfilled','canceled') OR review_status = 'released'),
    UNIQUE (order_id, batch_id)
);

-- Dòng hàng; giá snapshot lịch sử
CREATE TABLE order_items (
    item_id text PRIMARY KEY,
    order_id text NOT NULL,
    shop_id text NOT NULL,
    listing_id text NOT NULL,
    quantity integer NOT NULL,
    unit_price numeric(18,2) NOT NULL,
    freight_amount numeric(18,2) NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    FOREIGN KEY (order_id, shop_id) REFERENCES orders(order_id, shop_id),
    FOREIGN KEY (listing_id, shop_id) REFERENCES listings(listing_id, shop_id),
    UNIQUE (batch_id, source_record_id),
    CHECK (quantity > 0),
    CHECK (unit_price >= 0),
    CHECK (freight_amount >= 0)
);

-- Dòng thanh toán ở cấp checkout; không sao chép theo shop
CREATE TABLE payments (
    payment_id text PRIMARY KEY,
    checkout_id text NOT NULL REFERENCES checkouts(checkout_id),
    method text NOT NULL,
    amount numeric(18,2) NOT NULL,
    status text NOT NULL,
    paid_at timestamptz,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id),
    CHECK (amount >= 0),
    CHECK (status IN ('unknown','pending','succeeded','failed','refunded'))
);

-- Review nguồn ở cấp checkout; không phải verified dispute
CREATE TABLE reviews (
    review_record_id text PRIMARY KEY,
    checkout_id text NOT NULL REFERENCES checkouts(checkout_id),
    rating integer,
    title text,
    message text,
    source_created_at_raw text,
    source_answered_at_raw text,
    available_at timestamptz,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id),
    CHECK (rating IS NULL OR rating BETWEEN 1 AND 5)
);

-- Sự kiện gốc để dựng lịch sử theo thời điểm
CREATE TABLE order_events (
    event_id text PRIMARY KEY,
    order_id text NOT NULL REFERENCES orders(order_id),
    event_type text NOT NULL,
    actor_type text NOT NULL,
    reason text,
    occurred_at timestamptz NOT NULL,
    available_at timestamptz,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    UNIQUE (batch_id, source_record_id),
    CHECK (event_type IN ('created','confirmed','address_changed','canceled','delivery_failed','delivery_succeeded','return_requested')),
    CHECK (actor_type IN ('buyer','seller','carrier','platform','unknown')),
    CHECK (available_at IS NULL OR available_at >= occurred_at)
);

-- Hồ sơ lịch sử hỗ trợ detect đơn tương lai
CREATE TABLE cases (
    case_id text PRIMARY KEY,
    order_id text NOT NULL REFERENCES orders(order_id),
    case_type text NOT NULL,
    opened_at timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    CHECK (case_type IN ('delivery','return','refund','other')),
    CHECK (available_at >= opened_at),
    UNIQUE (case_id, batch_id)
);

-- Lời trình bày của từng bên; không phải kết luận
CREATE TABLE case_messages (
    message_id text PRIMARY KEY,
    case_id text NOT NULL REFERENCES cases(case_id),
    speaker_role text NOT NULL,
    body text NOT NULL,
    occurred_at timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    CHECK (speaker_role IN ('buyer','seller','carrier','platform')),
    CHECK (available_at >= occurred_at)
);

-- Tham chiếu bằng chứng; cờ có ảnh không thay thế ảnh
CREATE TABLE case_evidence (
    evidence_id text PRIMARY KEY,
    case_id text NOT NULL REFERENCES cases(case_id),
    submitted_by text NOT NULL,
    media_type text NOT NULL,
    content_ref text NOT NULL,
    available_at timestamptz NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    CHECK (media_type IN ('text','image','document'))
);

-- Các kết luận có thời điểm; giữ lịch sử khi thay đổi
CREATE TABLE case_resolutions (
    resolution_id text PRIMARY KEY,
    case_id text NOT NULL REFERENCES cases(case_id),
    verification_status text NOT NULL,
    responsible_party text NOT NULL,
    rationale text NOT NULL,
    decided_by text NOT NULL,
    decided_at timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    source_record_id text NOT NULL,
    CHECK (verification_status IN ('unresolved','insufficient_evidence','verified')),
    CHECK (responsible_party IN ('buyer','seller','carrier','shared','unknown')),
    CHECK (available_at >= decided_at)
);

-- Output LLM có phiên bản; không phải ground truth
CREATE TABLE case_analyses (
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    analysis_id text PRIMARY KEY,
    case_id text NOT NULL REFERENCES cases(case_id),
    input_cutoff_at timestamptz NOT NULL,
    model_ref text NOT NULL,
    prompt_version text NOT NULL,
    signals jsonb NOT NULL,
    summary text NOT NULL,
    created_at timestamptz NOT NULL,
    CHECK (created_at >= input_cutoff_at),
    FOREIGN KEY (case_id, batch_id) REFERENCES cases(case_id, batch_id)
);

-- Snapshot chấm điểm có thể giải thích và tái hiện
CREATE TABLE rule_versions (
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    rule_version text PRIMARY KEY,
    config jsonb NOT NULL,
    config_sha256 text NOT NULL CHECK (length(config_sha256) = 64),
    activated_at timestamptz NOT NULL,
    approved_by text NOT NULL,
    UNIQUE (rule_version, batch_id)
);

CREATE TABLE risk_assessments (
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    assessment_id text PRIMARY KEY,
    order_id text NOT NULL REFERENCES orders(order_id),
    assessed_at timestamptz NOT NULL,
    rule_version text NOT NULL REFERENCES rule_versions(rule_version),
    score integer NOT NULL,
    risk_level text NOT NULL,
    feature_snapshot jsonb NOT NULL,
    reasons jsonb NOT NULL,
    UNIQUE (assessment_id, order_id),
    CHECK (score BETWEEN 0 AND 100),
    CHECK (risk_level IN ('low','medium','high')),
    UNIQUE (assessment_id, batch_id),
    FOREIGN KEY (order_id, batch_id) REFERENCES orders(order_id, batch_id),
    FOREIGN KEY (rule_version, batch_id) REFERENCES rule_versions(rule_version, batch_id)
);

-- Log thay đổi gate do policy hoặc seller
CREATE TABLE review_decisions (
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    decision_id text PRIMARY KEY,
    order_id text NOT NULL,
    assessment_id text NOT NULL,
    actor_type text NOT NULL,
    actor_ref text NOT NULL,
    action text NOT NULL,
    reason text NOT NULL,
    created_at timestamptz NOT NULL,
    FOREIGN KEY (assessment_id, order_id) REFERENCES risk_assessments(assessment_id, order_id),
    CHECK (actor_type IN ('policy','seller')),
    CHECK (action IN ('hold','release','keep_hold')),
    FOREIGN KEY (assessment_id, batch_id) REFERENCES risk_assessments(assessment_id, batch_id),
    FOREIGN KEY (order_id, batch_id) REFERENCES orders(order_id, batch_id)
);

CREATE INDEX idx_checkouts_1 ON checkouts (buyer_id, placed_at);

CREATE INDEX idx_orders_2 ON orders (shop_id, review_status);

CREATE INDEX idx_orders_3 ON orders (checkout_id);

CREATE INDEX idx_order_items_4 ON order_items (order_id);

CREATE INDEX idx_payments_5 ON payments (checkout_id);

CREATE INDEX idx_reviews_6 ON reviews (checkout_id);

CREATE INDEX idx_checkout_addresses_7 ON checkout_addresses (address_key);

CREATE INDEX idx_order_events_8 ON order_events (order_id, available_at);

CREATE INDEX idx_cases_9 ON cases (order_id);

CREATE INDEX idx_case_messages_10 ON case_messages (case_id, available_at);

CREATE INDEX idx_case_evidence_11 ON case_evidence (case_id, available_at);

CREATE INDEX idx_case_resolutions_12 ON case_resolutions (case_id, available_at);

CREATE INDEX idx_case_analyses_13 ON case_analyses (case_id, created_at);

CREATE INDEX idx_risk_assessments_14 ON risk_assessments (order_id, assessed_at);

CREATE INDEX idx_review_decisions_15 ON review_decisions (order_id, created_at);


-- Optional self-declared hometown; never used as a risk feature.
CREATE TABLE buyer_profiles (
    buyer_id text PRIMARY KEY REFERENCES buyers(buyer_id),
    hometown_region text,
    declared_at timestamptz NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id)
);
-- Current address book. Order history uses checkout_addresses snapshots instead.
CREATE TABLE saved_addresses (
    saved_address_id text PRIMARY KEY,
    buyer_id text NOT NULL REFERENCES buyers(buyer_id),
    label text NOT NULL,
    address_raw text NOT NULL,
    address_key text NOT NULL,
    region text NOT NULL,
    country_code text NOT NULL CHECK (length(country_code) = 2),
    is_default boolean NOT NULL,
    created_at timestamptz NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id)
);
CREATE UNIQUE INDEX one_default_address ON saved_addresses(buyer_id) WHERE is_default;
-- Tokens, not raw phone numbers/device fingerprints. Nullable means unknown.
CREATE TABLE checkout_identity_snapshots (
    checkout_id text PRIMARY KEY REFERENCES checkouts(checkout_id),
    phone_key text,
    device_key text,
    address_cluster_id text,
    available_at timestamptz NOT NULL,
    batch_id text NOT NULL REFERENCES data_batches(batch_id)
);
CREATE INDEX identity_phone ON checkout_identity_snapshots(phone_key);
CREATE INDEX identity_device ON checkout_identity_snapshots(device_key);
CREATE INDEX identity_cluster ON checkout_identity_snapshots(address_cluster_id);

CREATE TABLE rule_proposals (
    batch_id text NOT NULL REFERENCES data_batches(batch_id),
    proposal_id text PRIMARY KEY,
    base_rule_version text NOT NULL REFERENCES rule_versions(rule_version),
    rule_id text NOT NULL,
    proposed_changes jsonb NOT NULL,
    evidence jsonb NOT NULL,
    rationale text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending','approved','rejected')),
    created_at timestamptz NOT NULL,
    reviewed_at timestamptz,
    reviewed_by text,
    CHECK ((status = 'pending' AND reviewed_at IS NULL AND reviewed_by IS NULL)
        OR (status <> 'pending' AND reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL)),
    FOREIGN KEY (base_rule_version, batch_id) REFERENCES rule_versions(rule_version, batch_id)
);
ALTER TABLE risk_assessments ADD COLUMN input_cutoff_at timestamptz NOT NULL;
-- For new assessments service must set cutoff = checkout.placed_at on first scoring.
-- Old saved versions are immutable; changing a rule creates a new version.

CREATE INDEX idx_events_type_available ON order_events(order_id, event_type, available_at);
CREATE INDEX idx_checkouts_placed ON checkouts(placed_at);
COMMIT;
