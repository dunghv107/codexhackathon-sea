# ERD cho BE/FE và điểm nối với AI

Bản thiết kế để review và dựng database; không chứa dataset demo. DDL trong schema.sql là nguồn chuẩn. Các cạnh là FK đã khai báo; trường token phone/device không phải FK sang bảng nguồn có thật.

Ký hiệu: `||` đúng một; `o{` không hoặc nhiều; `o|` không hoặc một. Quan hệ một chiều FK không bắt buộc parent phải có child; service phải tạo snapshot và ít nhất một item khi checkout.

## Luồng nghiệp vụ

Buyer → checkout nhiều shop → một order mỗi shop → chấm điểm → Hold/Release → đóng gói/giao. Payment và review Olist ở cấp checkout, không nhân bản qua shop.

## 01_platform

```mermaid
erDiagram
    sellers {
        text seller_id PK
        text auth_subject
        text display_name
        timestamptz account_created_at
        text status
        text batch_id FK
        text source_record_id
    }
    buyers {
        text buyer_id PK
        text auth_subject
        text status
        text display_name
        timestamptz account_created_at
        text batch_id FK
        text source_record_id
    }
    buyer_profiles {
        text buyer_id PK, FK
        text hometown_region
        timestamptz declared_at
        text batch_id FK
    }
    saved_addresses {
        text saved_address_id PK
        text buyer_id FK
        text label
        text address_raw
        text address_key
        text region
        text country_code
        boolean is_default
        timestamptz created_at
        text batch_id FK
    }
    shops {
        text shop_id PK
        text display_name
        text owner_seller_id FK
        text batch_id FK
        text source_record_id
    }
    products {
        text product_id PK
        text display_name
        text category_code
        text category_name_en
        text batch_id FK
        text source_record_id
    }
    listings {
        text listing_id PK
        text shop_id FK
        text product_id FK
        numeric_18_2_ current_price
        text currency
        integer stock_quantity
        text batch_id FK
        text source_record_id
    }
    checkouts {
        text checkout_id PK
        text buyer_id FK
        timestamptz placed_at
        text payment_method
        text currency
        numeric_18_2_ total_amount
        text batch_id FK
        text source_record_id
    }
    checkout_addresses {
        text checkout_id PK, FK
        text address_raw
        text address_key
        text postal_prefix
        text city
        text region
        text country_code
        text batch_id FK
        text source_record_id
    }
    orders {
        text order_id PK
        text checkout_id FK
        text shop_id FK
        text fulfillment_status
        text review_status
        text batch_id FK
        text source_record_id
    }
    order_items {
        text item_id PK
        text order_id FK
        text shop_id FK
        text listing_id FK
        integer quantity
        numeric_18_2_ unit_price
        numeric_18_2_ freight_amount
        text batch_id FK
        text source_record_id
    }
    payments {
        text payment_id PK
        text checkout_id FK
        text method
        numeric_18_2_ amount
        text status
        timestamptz paid_at
        text batch_id FK
        text source_record_id
    }
    reviews {
        text review_record_id PK
        text checkout_id FK
        integer rating
        text title
        text message
        text source_created_at_raw
        text source_answered_at_raw
        timestamptz available_at
        text batch_id FK
        text source_record_id
    }
    order_events {
        text event_id PK
        text order_id FK
        text event_type
        text actor_type
        text reason
        timestamptz occurred_at
        timestamptz available_at
        text batch_id FK
        text source_record_id
    }
    buyers ||--o| buyer_profiles : "buyer_id"
    buyers ||--o{ saved_addresses : "buyer_id"
    sellers ||--o{ shops : "owner_seller_id"
    products ||--o{ listings : "product_id"
    shops ||--o{ listings : "shop_id"
    buyers ||--o{ checkouts : "buyer_id"
    checkouts ||--o| checkout_addresses : "checkout_id"
    shops ||--o{ orders : "shop_id"
    checkouts ||--o{ orders : "checkout_id"
    listings ||--o{ order_items : "listing_id, shop_id"
    orders ||--o{ order_items : "order_id, shop_id"
    checkouts ||--o{ payments : "checkout_id"
    checkouts ||--o{ reviews : "checkout_id"
    orders ||--o{ order_events : "order_id"
```

## 02_risk_and_kbs

```mermaid
erDiagram
    checkout_identity_snapshots {
        text checkout_id PK, FK
        text phone_key
        text device_key
        text address_cluster_id
        timestamptz available_at
        text batch_id FK
    }
    cases {
        text case_id PK
        text order_id FK
        text case_type
        timestamptz opened_at
        timestamptz available_at
        text batch_id FK
        text source_record_id
    }
    case_messages {
        text message_id PK
        text case_id FK
        text speaker_role
        text body
        timestamptz occurred_at
        timestamptz available_at
        text batch_id FK
        text source_record_id
    }
    case_evidence {
        text evidence_id PK
        text case_id FK
        text submitted_by
        text media_type
        text content_ref
        timestamptz available_at
        text batch_id FK
        text source_record_id
    }
    case_resolutions {
        text resolution_id PK
        text case_id FK
        text verification_status
        text responsible_party
        text rationale
        text decided_by
        timestamptz decided_at
        timestamptz available_at
        text batch_id FK
        text source_record_id
    }
    case_analyses {
        text analysis_id PK
        text case_id FK
        timestamptz input_cutoff_at
        text model_ref
        text prompt_version
        jsonb signals
        text summary
        timestamptz created_at
    }
    risk_assessments {
        text assessment_id PK
        text order_id FK
        timestamptz assessed_at
        text rule_version
        integer score
        text risk_level
        jsonb feature_snapshot
        jsonb reasons
        timestamptz input_cutoff_at
    }
    review_decisions {
        text decision_id PK
        text order_id FK
        text assessment_id FK
        text actor_type
        text actor_ref
        text seller_id FK
        text action
        text reason
        timestamptz created_at
    }
    rule_versions {
        text rule_version PK
        jsonb config
        text config_sha256
        timestamptz activated_at
        text approved_by
    }
    rule_proposals {
        text proposal_id PK
        text base_rule_version FK
        text rule_id
        jsonb proposed_changes
        jsonb evidence
        text rationale
        text status
        timestamptz created_at
        timestamptz reviewed_at
        text reviewed_by
    }
    checkouts {
        text checkout_id PK
        text buyer_id FK
        timestamptz placed_at
        text payment_method
        text currency
        numeric_18_2_ total_amount
        text batch_id FK
        text source_record_id
    }
    orders {
        text order_id PK
        text checkout_id FK
        text shop_id FK
        text fulfillment_status
        text review_status
        text batch_id FK
        text source_record_id
    }
    sellers {
        text seller_id PK
        text auth_subject
        text display_name
        timestamptz account_created_at
        text status
        text batch_id FK
        text source_record_id
    }
    checkouts ||--o| checkout_identity_snapshots : "checkout_id"
    orders ||--o{ cases : "order_id"
    cases ||--o{ case_messages : "case_id"
    cases ||--o{ case_evidence : "case_id"
    cases ||--o{ case_resolutions : "case_id"
    cases ||--o{ case_analyses : "case_id"
    orders ||--o{ risk_assessments : "order_id"
    risk_assessments ||--o{ review_decisions : "assessment_id, order_id"
    sellers ||--o{ review_decisions : "seller_id"
    rule_versions ||--o{ rule_proposals : "base_rule_version"
    checkouts ||--o{ orders : "checkout_id"
```

## Provenance và các quan hệ cần lưu ý

- `data_batches` nối tới mọi bảng có batch_id; xem ERD_full.mmd để có toàn bộ cạnh. Mỗi batch ghi nguồn và is_synthetic.
- `risk_assessments.rule_version` hiện là tham chiếu logic tới rule_versions.rule_version, chưa có FK SQL; backend phải kiểm phiên bản tồn tại. Đánh dấu rõ để BE chốt constraint trước migration.
- `review_decisions` dùng FK ghép (assessment_id, order_id), `order_items` dùng FK ghép để bảo đảm đúng shop. Sơ đồ không thay thế các CHECK/UNIQUE trong SQL.
- Những bảng shared giữa hai hình là cùng một bảng, không tạo hai bản.
- Seller và buyer là hai entity riêng. sellers → shops qua owner_seller_id NOT NULL; buyers → checkouts. Hai portal/auth subject được resolve theo loại tài khoản, không dùng buyer_id để thao tác seller.
- Giới hạn platform: đã thiết kế tài khoản nhưng chưa triển khai auth, variant SKU, nhiều kiện, settlement/refund ledger. Một seller có thể sở hữu nhiều shop; chưa có staff hoặc đồng sở hữu.
- Schema giữ phần mở rộng AI để thấy dependency. BE có thể ưu tiên module platform và assessment/decision; agent analysis, rule proposal làm sau. Không cần quyết định GraphRAG để tạo core database.
