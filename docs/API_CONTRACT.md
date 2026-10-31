# API contract — sàn mẫu (seller/buyer portal)

Schema chuẩn: `olist_cod/olist_cod/schema.sql`; phần auth là extension theo migration 000004. Hướng dẫn chạy: [OLIST_DEMO.md](OLIST_DEMO.md). KBS chấm điểm sau khi lưu checkout; đơn chưa có assessment giữ `review_status = pending`.

## Quy ước chung

- Envelope của template: `{ is_success, data, message, error: { code, message } }`. Danh sách trả `data = { items, total, page, limit, total_pages }`; query `page` (từ 1) và `limit` (mặc định 20, tối đa 100).
- Tiền là **string thập phân 2 chữ số** (`"129.90"`) kèm `currency` (ISO 3 ký tự). Backend tính bằng int64 cent, không dùng float.
- Thời gian theo RFC3339. ID là text và có thể chứa `:`, nên FE phải `encodeURIComponent` khi đặt vào path.
- Auth: `POST /api/auth/login` trả `access_token` và `refresh_token`, tạo phiên riêng cho mỗi lần đăng nhập; một tài khoản dùng được trên nhiều máy. Các request sau gửi `Authorization: Bearer <access_token>`. Khi gặp 401 thì gọi `POST /api/auth/refresh {refresh_token}`; lưu token mới vì refresh token cũ chỉ dùng một lần. Refresh không kéo dài thời hạn phiên (`JWT_REFRESH_EXPIRY_HOURS`).
- `POST /api/auth/logout` (Bearer, không cần body) chỉ thu hồi phiên hiện tại, gồm cả access token và refresh token. Các máy khác tiếp tục dùng bình thường. Tài khoản bị vô hiệu hóa/xóa vẫn bị chặn ngay trên tất cả máy.
- Migration `000009_user_sessions` lưu hash refresh token theo phiên. Chạy migration trước khi khởi động backend mới; phiên từ phiên bản cũ cần đăng nhập lại một lần. Payload login/refresh và cách frontend gọi API giữ nguyên.
- **Liên kết tài khoản:** `buyers.auth_subject` và `sellers.auth_subject` bằng `'local:' || users.id`. Server tự resolve buyer/seller từ JWT, không nhận buyer_id/seller_id từ client.
- Không có profile tương ứng → 403 `NO_BUYER_PROFILE` / `NO_SELLER_PROFILE`. Profile `suspended` → mọi request ghi bị 403 `ACCOUNT_SUSPENDED`.
- Tài nguyên không thuộc quyền của người gọi → **404** (không lộ là tài nguyên có tồn tại).
- Dữ liệu demo mới dùng `batch_id = 'olist_vn_sim'`, ID `live:<uuid>`. Dữ liệu cũ giữ nguyên batch và ID; không đổi provenance. Checkout yêu cầu buyer, shop, product và listing cùng batch mô phỏng. Decision/event trên đơn cũ giữ batch của đơn.

## Profile

`GET /api/profiles` (đăng nhập) → `{ buyer: {buyer_id, display_name, status} | null, seller: {seller_id, display_name, status} | null }`

## Catalog (public)

- `GET /api/catalog/categories` → `[{category_code, category_name_en}]`
- `GET /api/catalog/shops?page&limit` → items `{shop_id, display_name, listing_count}`
- `GET /api/catalog/shops/:shopId` → `{shop_id, display_name, listing_count}`
- `GET /api/catalog/listings?shop_id&category_code&q&currency&sort&in_stock&page&limit` → items `ListingCard`. Chỉ trả listing có `current_price`.

`ListingCard = {listing_id, shop_id, shop_name, product_id, product_name|null, category_code|null, category_name_en|null, current_price|null, currency, stock_quantity|null, description|null, image_url|null}`

Olist không có tên sản phẩm, nên `product_name` có thể null. Khi đó FE dùng tên danh mục, thay dấu gạch dưới bằng khoảng trắng và viết hoa chữ đầu; không gắn ID vào tên hiển thị.

### Tìm kiếm và nội dung sản phẩm

- `GET /api/catalog/listings/:listingId` trả `ListingCard`; 404 nếu không tồn tại hoặc chưa có giá.
- `q`: tìm không phân biệt hoa thường theo tên listing/product, danh mục và mã listing; tối đa 120 ký tự. `%`/`_` được tìm như ký tự thường.
- `sort`: bỏ trống (theo ID), `name`, `price_asc`, `price_desc`. Sắp xếp giá cần `currency` (ISO ba ký tự), không so sánh BRL với VND.
- `in_stock=true`: bỏ các listing tồn kho bằng 0; tồn kho NULL vẫn theo quy tắc mua hàng hiện hành.
- Migration `000007` thêm `listings.title`, `description`, `image_url` nullable. Tên tối đa 200 ký tự, mô tả 5000 ký tự, ảnh URL HTTPS tối đa 2048 byte, không chứa user/password. Backend không tải URL ảnh; browser hiển thị ảnh với fallback khi lỗi.
- PATCH chuỗi rỗng để xóa mô tả/ảnh; bỏ field để giữ nguyên. Không chấp nhận tên rỗng.
- Success response luôn chứa `data`, kể cả danh sách rỗng `[]`, để frontend kết thúc trạng thái tải.

## Buyer — `/api/buyer` (đăng nhập + có buyer profile)

| Method | Path | Body / ghi chú |
|---|---|---|
| GET | `/addresses` | `[SavedAddress]` |
| POST | `/addresses` | `{label, address_raw, region, country_code(2), is_default}` |
| PUT | `/addresses/:id` | cùng body với POST |
| DELETE | `/addresses/:id` | 204 |
| POST | `/checkouts` | `{saved_address_id, payment_method, items:[{listing_id, quantity}]}` → `{checkout_id, placed_at, total_amount, currency, orders:[{order_id, shop_id, shop_name, order_total}]}` |
| GET | `/orders?status&page&limit` | items `BuyerOrderSummary`, mới nhất trước; `status` là `display_status` trung tính của buyer |
| GET | `/orders/:orderId` | `BuyerOrderDetail` |
| POST | `/orders/:orderId/cancel` | `{reason}`. Chỉ hợp lệ khi `fulfillment_status = unfulfilled` |

- `SavedAddress = {saved_address_id, label, address_raw, region, country_code, is_default, created_at}`. `address_key` = `address_raw` chuyển về chữ thường, trim và gộp khoảng trắng. Mỗi buyer có tối đa một địa chỉ mặc định: đặt default mới thì bỏ default cũ trong cùng transaction.
- `payment_method` ∈ `cod, credit_card, debit_card, boleto, voucher`.
- **Checkout** chạy trong một transaction:
  - items không rỗng, `quantity` từ 1 đến 99;
  - listing tồn tại và có `current_price`;
  - tất cả item cùng một currency, nếu không → 400 `MIXED_CURRENCY`;
  - `stock_quantity` khác null thì phải ≥ quantity, rồi trừ tồn; không đủ → 409 `OUT_OF_STOCK`;
  - địa chỉ phải thuộc buyer.
  - Bản ghi được tạo:
    - một `checkout` với `placed_at = now()`;
    - `checkout_addresses` chụp lại `address_raw`, `address_key`, `region`, `country_code`;
    - mỗi shop một `order` (`unfulfilled` / `pending`);
    - `order_items` với `unit_price` = giá hiện tại và `freight_amount = 0` (chưa có nguồn phí ship);
    - một `payment` (`amount` = tổng, `status = pending`);
    - mỗi order một `order_events` loại `created`, `actor_type = buyer`, `available_at = occurred_at`;
    - một `checkout_identity_snapshots`, `available_at = placed_at`, phone/device/address cluster chưa biết để NULL.
  - Tổng tiền: `order_total` = Σ(quantity × unit_price + freight_amount) và `total_amount` = Σ `order_total`.
- `BuyerOrderSummary = {order_id, checkout_id, placed_at, shop_id, shop_name, item_count, order_total, currency, fulfillment_status, display_status}`
- `display_status` ∈ `confirming` (chưa released và còn unfulfilled), `confirmed`, `packing`, `shipped`, `delivered`, `canceled`, `returned`. FE hiển thị `confirming` thành "Đơn đang được xác nhận". **Buyer không bao giờ nhận `review_status`, score hay risk_level.**
- `BuyerOrderDetail = BuyerOrderSummary + {items:[{item_id, listing_id, product_name|null, category_name_en|null, quantity, unit_price, freight_amount}], address:{address_raw, city, region, postal_prefix, country_code, ward_code, house_no, parse_ok} (các giá trị có thể null), payment:{method, status}, events:[{event_type, occurred_at}]}`

## Seller — `/api/seller` (đăng nhập + có seller profile)

| Method | Path | Body / ghi chú |
|---|---|---|
| GET | `/shops` | `[{shop_id, display_name, batch_id, assessed_orders}]` thuộc seller, ưu tiên shop có assessment |
| POST | `/shops` | `{display_name}` → shop mới |

Mọi route `/shops/:shopId/...` đều kiểm `shops.owner_seller_id = seller đang đăng nhập`; không đúng → 404.

| Method | Path | Body / ghi chú |
|---|---|---|
| GET | `/shops/:shopId/dashboard` | `{review_status:{pending,hold,released}, fulfillment_status:{unfulfilled,packing,shipped,delivered,canceled,returned}, risk_level:{low,medium,high,unassessed}}`, đều là số đếm. Risk lấy theo assessment mới nhất của từng order |
| GET | `/shops/:shopId/listings?page&limit` | items `SellerListing` |
| POST | `/shops/:shopId/listings` | `{product_name, category_code?, current_price, currency?='VND', stock_quantity?, description?, image_url?}`. Luôn tạo **product mới** riêng cho listing này |
| PATCH | `/shops/:shopId/listings/:listingId` | `{product_name?, description?, image_url?, current_price?, stock_quantity?}`. Tên/mô tả/ảnh là nội dung riêng của listing; không sửa product dùng chung hoặc giá snapshot của đơn cũ |
| GET | `/shops/:shopId/orders?batch_id&order_id&review_status&fulfillment_status&risk_level&page&limit` | items `SellerOrderSummary`, `placed_at` giảm dần. `risk_level=unassessed` lọc đơn chưa có assessment |
| GET | `/shops/:shopId/orders/:orderId` | `SellerOrderDetail` |
| POST | `/shops/:shopId/orders/:orderId/score?force=false` | Gọi KBS rồi trả `SellerOrderDetail`; `force=true` ghi assessment mới |
| POST | `/shops/:shopId/orders/:orderId/explain` | `{assessment_id, source: "template" | "llm", sentences: string[], fallback_reason: string | null}`; tối đa 3 câu |
| POST | `/shops/:shopId/orders/:orderId/decision` | `{assessment_id, action: hold\|release\|keep_hold, reason}` |
| POST | `/shops/:shopId/orders/:orderId/fulfillment` | `{status: packing\|shipped\|delivered\|canceled\|returned}` |

- `SellerListing = ListingCard + {}` (cùng các field).
- Score/explain chỉ áp dụng cho `olist_vn_sim`; batch khác trả 409 `UNSUPPORTED_RISK_BATCH`.
- Explain giữ nguyên mã dự phòng từ KBS (`disabled`, `no_fired_rules`, `provider_timeout`, `provider_unavailable`, `invalid_output`, `provider_refusal`) để UI giải thích nguồn kết quả. LLM diễn giải dữ kiện lịch sử và gợi ý câu hỏi xác minh; không tính điểm hoặc quyết định Release/Hold. Nội dung không bao gồm kết quả cuộc gọi hiện tại.
- `POST /api/seller/shops/:shopId/orders/:orderId/verification/voice-preview`: JSON `{audio: base64, mime_type: audio/webm|audio/ogg|audio/mp4|audio/wav}`, tối đa 600 KB âm thanh. Yêu cầu seller hoạt động, sở hữu shop/order, demo mode và verification tồn tại. Trả `{transcript, intent: confirmed|declined|unclear}` trong envelope, không ghi DB. Lỗi ASR/classifier trả 503 `VOICE_UNAVAILABLE`; không trả payload provider. UI xác nhận thủ công qua `/verification/respond` hiện có. Backend nội bộ gọi KBS `/api/v1/orders/{id}/voice-preview`.
  Go lấy `X-Shop-ID` từ shop đã kiểm quyền, không chuyển header của client sang KBS.
  KBS lỗi trả 503 `KBS_UNAVAILABLE`. Riêng checkout đã commit vẫn trả thành công khi chấm điểm lỗi;
  đơn còn pending và seller có thể chấm lại. Không tự retry mua hàng để retry scoring.
- Khi còn `unfulfilled`, assessment mới Low tự Release đơn pending hoặc đang Hold do policy;
  giữ Hold/Keep Hold thủ công theo quyết định mới nhất. Assessment mới Medium/High tự Hold để seller duyệt.
  Thay đổi gate được ghi vào `review_decisions` với `actor_type = policy` cùng transaction với assessment.
  Cache Low cũng áp dụng Release nếu vẫn là assessment mới nhất; cache Medium/High không ghi đè quyết định seller.
  Migration 11 tự Release các đơn Low cũ đủ điều kiện. Không thay đổi fulfillment hoặc điều kiện thanh toán.
- Reason mới là `{rule_id, contribution, description}`; KBS và UI vẫn đọc `points` từ assessment cũ.
- Phiên bản `draft-vn-olist-adaptive-shadow-0.2` giữ nguyên điểm R1–R10 và có đủ 18 rule thích ứng;
  bản `0.1` với 8 rule vẫn giữ nguyên. Kết quả người bán thêm
  `feature_snapshot.adaptive = {mode:"shadow", calibration:"demo_not_calibrated", baseline_version,
  policy_version, input_cutoff_at, signals, coverage, evidence_groups}`.
  Mỗi signal có `rule_id`, `status: fired|clear|unknown`, `description`, `action`, `contribution:0`,
  `evidence` và `missing_reason`. Evidence lưu observed, comparison_threshold, baseline
  (cohort/sample_count/threshold/quantile/window_start/window_end/available_at/version/source_ref/selection_policy)
  cùng lý do thiếu dữ liệu. API không trả raw contact keys hoặc ID đơn của shop khác trong các evidence này.
  `coverage` có required_features, known_features, missing_features và **blocking:false**.
  A17 chỉ cảnh báo; các rule shadow không thay đổi điểm hoặc Hold/Release.
  Assessment cũ có thể không có `adaptive`; FE vẫn hiển thị được.
  Bản 0.2 chỉ trả 15 signal cho seller; A12/A13/A16 lưu riêng cho quản trị sàn.
  Evidence có thêm `observed_sample_count` khi so sánh hai tỷ lệ; signal có `audience`.
  Chạy migration `000012_adaptive_evidence` trước backend/KBS mới. Checkout mới lưu snapshot danh mục
  và mốc bắt đầu theo dõi trạng thái. Không backfill danh mục cũ từ catalog hiện tại.
- `SellerOrderSummary = {batch_id, is_synthetic, source_system, order_id, checkout_id, placed_at, payment_method, item_count, order_total, currency, review_status, fulfillment_status, risk_level|null, score|null}`
- `SellerOrderDetail = SellerOrderSummary + {items:[…như buyer], address:{…như buyer}, payment:{method, status}, events:[{event_type, actor_type, reason|null, occurred_at, available_at|null}], latest_assessment: {assessment_id, assessed_at, input_cutoff_at, rule_version, score, risk_level, feature_snapshot, reasons, rule_config} | null, decisions:[{decision_id, assessment_id, actor_type, action, reason, created_at}]}`
  - Thêm `cases: [{case_id, case_type, opened_at, available_at, messages, evidence, resolutions}]`, chỉ hồ sơ của đơn đã kiểm quyền. Messages có speaker_role/body/occurred_at/available_at; evidence có media_type/content_ref/submitted_by/available_at; resolutions có verification_status/responsible_party/rationale/decided_at/available_at. Đây là hồ sơ hiện tại, không phải tất cả đều có trước cutoff.
  - Payment chỉ gồm method và status của checkout. **Không trả amount hay tổng checkout**, vì checkout có thể gồm đơn của shop khác.
- **Decision** chạy trong một transaction và khóa order bằng `SELECT … FOR UPDATE`:
  - seller phải đang `active`;
  - order chưa có assessment → 409 `NOT_ASSESSED`;
  - `assessment_id` khác assessment mới nhất → 409 `STALE_ASSESSMENT`;
  - `fulfillment_status ≠ unfulfilled` → 409 `INVALID_STATE`;
  - `reason` không được rỗng;
  - `keep_hold` chỉ hợp lệ khi đơn đang `hold`;
  - ghi `review_decisions` (`actor_type = seller`, `actor_ref = seller_id`, `seller_id`, `batch_id = order.batch_id`) và cập nhật `review_status` (release → released, hold/keep_hold → hold).
- **Fulfillment** (khóa order). Chuyển trạng thái hợp lệ:

  | Từ | Sang | Điều kiện |
  |---|---|---|
  | unfulfilled | packing | đơn phải `released`, và checkout là `cod` hoặc có payment `succeeded` (nếu không → 409 `PAYMENT_NOT_READY`) |
  | packing | shipped | |
  | shipped | delivered | |
  | unfulfilled, packing | canceled | |
  | delivered | returned | |

  Chuyển sai → 409 `INVALID_TRANSITION`. Event ghi kèm (`actor_type = seller`): delivered → `delivery_succeeded`, canceled → `canceled`; packing/shipped/returned ghi event cùng tên từ migration 12.

## Quản trị rủi ro — `/api/admin/risk`

Hai endpoint yêu cầu Bearer của `ADMIN` hoặc `SYSTEM_ADMIN`; tài khoản thường trả 403.
Giao diện `/admin/risk` có bộ lọc tín hiệu và form ghi nhận dữ liệu kiểm chứng.

- `GET /observations?status=all|fired|clear|unknown&page&limit`: mặc định `fired`, page 1, limit 20.
  Chỉ trả assessment mới nhất của từng đơn trong `olist_vn_sim`, có signal phù hợp bộ lọc.
  Mỗi item gồm `{assessment_id, order_id, shop_id, shop_name, rule_version, baseline_version,
  assessed_at, input_cutoff_at, signals}`; signals chỉ gồm A12/A13/A16. Không trả ID tài khoản
  liên kết, nguồn giấy tờ hay dấu vân tay bằng chứng.
- `POST /evidence`: `{record_id, kind, occurred_at, source_ref, data}` → 201 `{record_id}`.
  `record_id` do caller tạo, tối đa 128 byte; trùng mã trả 409, không ghi đè.
  `source_ref` là nguồn kiểm chứng/mã hồ sơ đối chiếu, tối đa 500 byte; thời điểm RFC3339 không được ở tương lai.
  Server tự ghi `available_at = thời điểm nhận` và `recorded_by = admin:<user-id>`.
  Không nhận batch, người ghi nhận, thời điểm biết dữ liệu hoặc SHA256 do client tự khai.
  Tổng JSON tối đa 850 KB; field không hỗ trợ trả 400; tham chiếu không tồn tại trong batch trả 404.

| `kind` | Các field của `data` |
| --- | --- |
| `identity_link` | `buyer_id`, `related_buyer_id` khác nhau; `edge_type`: `verified_document`, `verified_payment`, `verified_account`; `status`: `verified`, `revoked`. |
| `enforcement` | `buyer_id`; `action`: `restricted`, `reinstated`; `abuse_code`: `delivery`, `cancellation`, `after_sale`. Đây là audit quyết định đã thực hiện, không tự khóa/khôi phục tài khoản. |
| `case_verdict` | `order_id`, `case_id` tùy chọn (bỏ trống tạo hồ sơ), `case_type`: `delivery`, `cancellation`, `return`, `refund`; `verification_status`: `unresolved`, `insufficient_evidence`, `verified`; `responsible_party`: `buyer`, `seller`, `carrier`, `shared`, `unknown`; `outcome_code` và `rationale` bắt buộc. |
| `evidence_review` | `evidence_id` đã có trong hồ sơ, `content_base64` là nội dung tệp tối đa 600 KB, `verdict`: `contradiction_verified`, `no_contradiction`, `unresolved`. Server băm SHA256 từ nội dung; không lưu tệp hoặc tải URL. |

`outcome_code` phải khớp loại hồ sơ: `delivery_failure`, `cancellation`, `return_abuse`, `refund_abuse`,
hoặc `legitimate`. Hai nhãn `_abuse` chỉ hợp lệ khi `verification_status=verified` và
`responsible_party=buyer`. Hồ sơ hiện có phải thuộc đúng đơn và đúng loại.
Kết luận mới được thêm vào lịch sử; không sửa kết luận cũ. Thu hồi liên kết dùng cùng cặp người mua,
loại cạnh và `source_ref`; kết luận mới nhất đã biết quyết định trạng thái của nguồn đó.

Nguồn admin được xem là dữ liệu đã đối chiếu thủ công, không phải xác minh tự động của nhà cung cấp.
A12/A13 yêu cầu ít nhất hai loại cạnh và hai nguồn độc lập; phone/IP/address chung không đáp ứng điều kiện.
A15 chỉ cảnh báo khi nội dung trùng ở hồ sơ của đơn khác và có mâu thuẫn đã xác minh; không tự từ chối refund.
Ghi nhận dữ liệu trước rồi tạo checkout mới để quan sát tín hiệu. Dữ liệu đến sau cutoff không làm thay đổi
assessment cũ, kể cả khi bấm chấm lại. Các rule vẫn shadow, contribution 0; A17 không chặn auto-release.

## Dữ liệu Olist COD

Dùng gói canonical và đường attach database trong [OLIST_DEMO.md](OLIST_DEMO.md).
`olist_raw` giữ dữ liệu gốc; `olist_vn_sim` là dữ liệu mô phỏng, không trộn vào nhau.
Không dùng pipeline import CSV cũ để thay thế gói COD: pipeline đó không cung cấp các assessment demo.
Tài khoản được liên kết qua CLI `cmd/demo-account` với các shop/buyer được chọn rõ ràng.
