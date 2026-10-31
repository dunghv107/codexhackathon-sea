# API contract — sàn mẫu (seller/buyer portal)

Schema chuẩn: `olist_cod/olist_cod/schema.sql`; phần auth là extension theo migration 000004. Hướng dẫn chạy: [OLIST_DEMO.md](OLIST_DEMO.md). Chấm điểm rủi ro **chưa làm**: đơn không có assessment thì giữ `review_status = pending`.

## Quy ước chung

- Envelope của template: `{ is_success, data, message, error: { code, message } }`. Danh sách trả `data = { items, total, page, limit, total_pages }`; query `page` (từ 1) và `limit` (mặc định 20, tối đa 100).
- Tiền là **string thập phân 2 chữ số** (`"129.90"`) kèm `currency` (ISO 3 ký tự). Backend tính bằng int64 cent, không dùng float.
- Thời gian theo RFC3339. ID là text và có thể chứa `:`, nên FE phải `encodeURIComponent` khi đặt vào path.
- Auth: `POST /api/auth/login` của template trả `access_token` và `refresh_token`. Các request sau gửi `Authorization: Bearer <access_token>`. Khi gặp 401 thì gọi `POST /api/auth/refresh {refresh_token}`.
- **Liên kết tài khoản:** `buyers.auth_subject` và `sellers.auth_subject` bằng `'local:' || users.id`. Server tự resolve buyer/seller từ JWT, không nhận buyer_id/seller_id từ client.
- Không có profile tương ứng → 403 `NO_BUYER_PROFILE` / `NO_SELLER_PROFILE`. Profile `suspended` → mọi request ghi bị 403 `ACCOUNT_SUSPENDED`.
- Tài nguyên không thuộc quyền của người gọi → **404** (không lộ là tài nguyên có tồn tại).
- Bản ghi do app tạo dùng `batch_id = 'app'` và ID dạng `app:<uuid>`. Gói Olist COD dùng namespace `raw:` / `sim:`. Decision và event trên đơn nhập giữ batch của đơn; dữ liệu tạo mới qua checkout thuộc `app`.

## Profile

`GET /api/profiles` (đăng nhập) → `{ buyer: {buyer_id, display_name, status} | null, seller: {seller_id, display_name, status} | null }`

## Catalog (public)

- `GET /api/catalog/categories` → `[{category_code, category_name_en}]`
- `GET /api/catalog/shops?page&limit` → items `{shop_id, display_name, listing_count}`
- `GET /api/catalog/shops/:shopId` → `{shop_id, display_name, listing_count}`
- `GET /api/catalog/listings?shop_id&category_code&page&limit` → items `ListingCard`. Chỉ trả listing có `current_price`.

`ListingCard = {listing_id, shop_id, shop_name, product_id, product_name|null, category_code|null, category_name_en|null, current_price|null, currency, stock_quantity|null}`

Olist không có tên sản phẩm, nên `product_name` có thể null. Khi đó FE hiển thị `category_name_en` cùng 6 ký tự cuối của `product_id`.

## Buyer — `/api/buyer` (đăng nhập + có buyer profile)

| Method | Path | Body / ghi chú |
|---|---|---|
| GET | `/addresses` | `[SavedAddress]` |
| POST | `/addresses` | `{label, address_raw, region, country_code(2), is_default}` |
| PUT | `/addresses/:id` | cùng body với POST |
| DELETE | `/addresses/:id` | 204 |
| POST | `/checkouts` | `{saved_address_id, payment_method, items:[{listing_id, quantity}]}` → `{checkout_id, placed_at, total_amount, currency, orders:[{order_id, shop_id, shop_name, order_total}]}` |
| GET | `/orders` | items `BuyerOrderSummary`, mới nhất trước |
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
| POST | `/shops/:shopId/listings` | `{product_name, category_code?, current_price, currency?='BRL', stock_quantity?}`. Luôn tạo **product mới** riêng cho listing này |
| PATCH | `/shops/:shopId/listings/:listingId` | `{current_price?, stock_quantity?}`. Không sửa giá snapshot của đơn cũ; không có API sửa product dùng chung |
| GET | `/shops/:shopId/orders?batch_id&order_id&review_status&fulfillment_status&risk_level&page&limit` | items `SellerOrderSummary`, `placed_at` giảm dần. `risk_level=unassessed` lọc đơn chưa có assessment |
| GET | `/shops/:shopId/orders/:orderId` | `SellerOrderDetail` |
| POST | `/shops/:shopId/orders/:orderId/decision` | `{assessment_id, action: hold\|release\|keep_hold, reason}` |
| POST | `/shops/:shopId/orders/:orderId/fulfillment` | `{status: packing\|shipped\|delivered\|canceled\|returned}` |

- `SellerListing = ListingCard + {}` (cùng các field).
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
  - ghi `review_decisions` (`actor_type = seller`, `actor_ref = seller_id`, `seller_id`, `batch_id = order.batch_id`) và cập nhật `review_status` (release → released, hold/keep_hold → hold).
- **Fulfillment** (khóa order). Chuyển trạng thái hợp lệ:

  | Từ | Sang | Điều kiện |
  |---|---|---|
  | unfulfilled | packing | đơn phải `released`, và checkout là `cod` hoặc có payment `succeeded` (nếu không → 409 `PAYMENT_NOT_READY`) |
  | packing | shipped | |
  | shipped | delivered | |
  | unfulfilled, packing | canceled | |
  | delivered | returned | |

  Chuyển sai → 409 `INVALID_TRANSITION`. Event ghi kèm (`actor_type = seller`): delivered → `delivery_succeeded`, canceled → `canceled`. Các bước khác chưa có event_type tương ứng trong schema.

## Dữ liệu Olist COD

Dùng gói canonical và đường attach database trong [OLIST_DEMO.md](OLIST_DEMO.md).
`olist_raw` giữ dữ liệu gốc; `olist_vn_sim` là dữ liệu mô phỏng, không trộn vào nhau.
Không dùng pipeline import CSV cũ để thay thế gói COD: pipeline đó không cung cấp các assessment demo.
Tài khoản được liên kết qua CLI `cmd/demo-account` với các shop/buyer được chọn rõ ràng.
