# PLAN v3 – KBS Order Risk Scoring trên gói dữ liệu `olist_cod` của team data

> Tài liệu cho coding agent (Codex / Claude Code). Đọc hết trước khi code. Thay thế v1, v2.
> Thay đổi so với v2:
> - **Phần A (dựng dữ liệu) bỏ hẳn.** Team data đã giao gói `olist_cod`: schema + seed + rule + kịch bản nghiệm thu.
>   Ngày thi chỉ nạp gói (~1–2 phút), dồn thời gian cho scoring, API, UI, agent.
> - **Rule lấy nguyên `rules.json` của team data** (`draft-vn-olist-0.2`), bỏ bảng rule tự đặt ở v2 (không còn group cap).
> - **Nghiệm thu = `scenario_expectations.json`**: 8 kịch bản phải khớp đúng điểm và tập rule.
> - Vẫn: một repo, toàn bộ code viết trong giờ thi, không Neo4j, chấm bằng SQL.

## 0. Nguyên tắc bất biến (constitution)

### Quyết định tích hợp với platform hiện tại

- Frontend dùng React hiện có; Go sở hữu checkout, xác thực/ủy quyền shop và seller Hold/Release.
  Các mục bên dưới về endpoint checkout/decision trong Python hoặc Streamlit là thiết kế ban đầu;
  triển khai tích hợp dùng các route Go trong [API contract](../../../docs/API_CONTRACT.md).
- Dữ liệu demo mới ghi `olist_vn_sim`, ID/source record `live:<uuid>` theo lựa chọn của chủ repo.
  Giữ nguyên dữ liệu `app`/`olist_raw` cũ, không đổi provenance hoặc gán lại identity lịch sử.
- Go gọi KBS sau khi commit checkout. Nếu chấm lỗi, vẫn giữ đơn pending đã lưu; seller retry qua
  `/api/seller/shops/{shopId}/orders/{orderId}/score`. Không trả lỗi mua hàng sau khi đã commit.
- KBS sở hữu SQL facts, scoring, assessment bất biến và policy Hold. Go kiểm assessment mới nhất
  và ghi seller decision với seller ID thật trong cùng transaction khóa order.
- Migration 5 chỉ tạo batch nếu thiếu. KBS khởi động chỉ thêm draft rule khi batch chưa có version;
  không sửa version đã nhập. Reason xuất `contribution`, đọc tương thích `points` cũ.

### Quy tắc chấm điểm

1. LLM **không bao giờ** tính điểm. Điểm chỉ từ SQL + `rules.json`.
2. Trọng số, ngưỡng, cửa sổ chỉ nằm trong `rules.json` → `rule_versions.config`. Không hard-code.
3. **Chỉ chấm batch `olist_vn_sim`.** Mọi truy vấn lọc `batch_id = 'olist_vn_sim'`. Không trộn batch `olist_raw` vào lịch sử.
4. Point-in-time: cutoff = `checkouts.placed_at` của đơn đang chấm. Lịch sử chỉ gồm checkout có `placed_at < cutoff`
   và identity `available_at < cutoff`; event và resolution chỉ tính khi `available_at < cutoff` **và**
   `occurred_at` / `decided_at < cutoff`.
5. "Thất bại do buyer" = có `delivery_failed` **và** resolution mới nhất (trước cutoff) là
   `verification_status='verified'`, `responsible_party='buyer'`. Giao lỗi do seller/carrier không tính cho buyer.
6. Thiếu dữ liệu = unknown: khoá null không khớp với nhau; rule unknown không fire, ghi lý do vào `feature_snapshot`.
7. Velocity đếm theo **checkout**; một checkout nhiều shop không phải nhiều lần đặt.
8. Tenant: API lọc theo `shop_id`; tín hiệu liên shop chỉ trả số tổng hợp.
9. Điểm là Risk Score 0–100, không phải xác suất. Ngưỡng 30/60 là giả định demo (`status: draft_not_calibrated`).
10. `evaluation/latent_outcomes.jsonl` (nếu có) là đáp án kín: **không bao giờ** đọc trong code chấm hay prompt.

## 1. Gói dữ liệu và cách dùng

| File | Dùng làm gì |
|---|---|
| `olist_cod/schema.sql` | DDL schema `sfd` (đã gồm `confirmed`, `ward_code/house_no/parse_ok`, `batch_id` ở cả 24 bảng, FK ghép theo batch, `input_cutoff_at NOT NULL`, index) |
| `olist_cod/seed.sql` | Dữ liệu (238 MB, INSERT từng dòng) |
| `olist_cod/rules.json` | Cấu hình rule `draft-vn-olist-0.2`, đã có sẵn 1 dòng trong `rule_versions` |
| `olist_cod/scenarios.json` + `analysis/olist_scenario_expectations.json` | 8 kịch bản: order_id, cutoff, nhãn, điểm, tập rule mong đợi |
| `olist_cod/transformations.json`, `README.md`, `source_inventory.json` | Provenance, đưa vào README repo và slide |
| `analysis/*.py` | Code team data viết **trước** giờ thi → **không** đưa vào repo, không copy (xem mục 7) |

Nạp: `createdb` → `psql -v ON_ERROR_STOP=1 -f schema.sql` → `psql -v ON_ERROR_STOP=1 -f seed.sql`.
Đã chạy thử trên PostgreSQL 16: schema < 1 giây, seed ~72 giây, không lỗi.

Quy mô batch `olist_vn_sim` (đã kiểm): 30.000 checkout, 30.602 order, khoảng 23 tháng (12/2024 → 30/10/2026),
COD 19.446 checkout; event: 27.946 giao thành công, 1.982 giao thất bại, 60 huỷ; 2.031 case, resolution:
buyer 1.192 / seller 423 / carrier 416; ~2.000 checkout dùng chung SĐT với tài khoản khác. `risk_assessments` có
sẵn 8 dòng (8 kịch bản). `as_of` = 2026-10-30 14:00 +07.

## 2. Cấu trúc repo

```
app/
  core/config.py            # DATABASE_URL, SIM_BATCH='olist_vn_sim', RULES_PATH, OPENAI_*
  db/postgres.py            # pool psycopg, search_path sfd
  rules/
    rules.json              # copy nguyên của team data
    engine.py               # pure: facts + config → points, score, risk_level, reasons
    sql/facts.sql           # 1 truy vấn batch: order_ids[] → mọi fact (mục 4)
  repositories/  services/scoring_service.py  services/backfill.py  services/network_service.py
  agents/                   # explain, investigate, propose
  api/v1/endpoints/         # checkouts, orders, rules, proposals, agent
data/olist_cod/             # schema.sql, seed.sql (git LFS hoặc .gitignore + hướng dẫn tải), rules.json, scenarios
tests/unit  tests/acceptance
frontend/                   # Streamlit
scripts/load_data.sh        # createdb + schema + seed
```

Dependencies: `psycopg[binary,pool]`, `numpy`, `networkx`, `openai`, `streamlit`.

## 3. Ngữ nghĩa rule (`rules.json`, giữ nguyên, tóm tắt để viết SQL)

Điểm = tổng điểm các rule bật đang fire, cắt tại 100. Low < 30 ≤ Medium < 60 ≤ High. Không có trần theo nhóm.
"Khớp" = cùng giá trị khoá, khoá không null. "Resolved" = checkout lịch sử có event giao (thành công/thất bại) đã biết
trước cutoff; thời điểm biết = `max(occurred_at)` các event giao đó.

| Rule | Fact | Fire khi | Điểm |
|---|---|---|---|
| R1 | Trên checkout resolved khớp `phone_key`, trong 180 ngày: `(n_buyer_failed + 2·0.1) / (n_resolved + 2)`; không có resolved → unknown | ≥0.25 / ≥0.5 / ≥0.75 | 15 / 30 / 45 |
| R2 | Như R1 theo `address_cluster_id` | ≥0.25 / ≥0.5 / ≥0.75 | 10 / 20 / 30 |
| R3 | Số checkout thất bại do buyer khớp `phone_key`, biết trong 90 ngày | ≥ 3 | 40 |
| R4 | Checkout trong 24h khớp `phone_key` **hoặc** `device_key` + checkout hiện tại; số shop của tập đó | ≥ 3 checkout **và** ≥ 3 shop | 20 |
| R5 | Checkout trong 1h khớp `phone_key` **hoặc** `address_cluster_id` + checkout hiện tại | ≥ 3 **và** (R3 hoặc R6 fire) | 25 |
| R6 | Checkout 90 ngày khớp cụm; số buyer khác nhau (gồm buyer hiện tại); tỷ lệ thất bại do buyer trên checkout resolved trong cụm | ≥ 3 buyer **và** ≥ 3 resolved **và** tỷ lệ > 0.25 | 20 |
| R7 | Đơn COD; tỷ số giá trị đơn / trung vị giá trị các order cùng shop, cùng currency, 90 ngày, bỏ đơn đã huỷ (biết trước cutoff); cần ≥ 5 đơn nền | tỷ số > 3 | 15 |
| R8 | Tuổi tài khoản (ngày) tại cutoff | 0 ≤ tuổi < 7 **và** (R7 hoặc R5 fire) | 10 |
| R9 | Tắt (`enabled: false`): chưa có bộ xác thực địa chỉ VN | — | 0 |
| R10 | Số checkout khớp `phone_key` có huỷ bởi buyer sau `confirmed`, huỷ trong 90 ngày | ≥ 2 | 15 |

Giá trị đơn = tổng `unit_price × quantity + freight_amount` của order. Thứ tự đánh giá theo `dependency_order` trong
`rules.json` (R5 sau R3/R6, R8 sau R5/R7).

## 4. SQL facts

View nền, đều lọc batch:

```sql
CREATE VIEW sim_ctx AS
SELECT o.order_id, o.shop_id, c.checkout_id, c.buyer_id, c.placed_at, c.payment_method, c.currency,
       (SELECT sum(i.unit_price*i.quantity + i.freight_amount) FROM order_items i WHERE i.order_id = o.order_id) AS amount,
       s.phone_key, s.device_key, s.address_cluster_id, s.available_at AS identity_available_at,
       b.account_created_at
FROM orders o
JOIN checkouts c ON c.checkout_id = o.checkout_id
JOIN buyers b ON b.buyer_id = c.buyer_id
JOIN checkout_identity_snapshots s ON s.checkout_id = c.checkout_id
WHERE o.batch_id = 'olist_vn_sim';
```

`facts.sql` nhận `%(order_ids)s text[]`, các CTE:
1. `t`: đơn cần chấm + cutoff = `placed_at`.
2. `hist`: với mỗi t, các checkout lịch sử (`placed_at < cutoff`, `identity_available_at < cutoff`), khác checkout t.
   Giữ cờ khớp phone / device / cluster.
3. `deliv`: với mỗi (t, checkout lịch sử), `max(occurred_at)` các event giao có `available_at < cutoff AND occurred_at < cutoff`
   → `known_at`; cờ có `delivery_failed`.
4. `buyer_fault`: resolution mới nhất trước cutoff (`available_at < cutoff AND decided_at < cutoff`) theo `decided_at`
   là verified + buyer, và checkout có `delivery_failed` → `buyer_failed`.
5. Các tổng hợp R1–R10 theo bảng mục 3, trả cả tử số, mẫu số, trung vị (R7) để đưa vào `feature_snapshot`.

Bayes, so bậc, điều kiện phụ thuộc làm trong `engine.py` (Python, pure), không trong SQL.
Mục tiêu: chấm 1 đơn < 300 ms; nếu chậm, thêm index `checkout_identity_snapshots(batch_id, phone_key)` và tương tự.

## 5. Scoring, backfill, đơn mới

- `score(order_ids, force=False)`: chạy `facts.sql` một lần cho cả danh sách → engine → ghi `risk_assessments`
  (`batch_id='olist_vn_sim'`, `rule_version='draft-vn-olist-0.2'`, `input_cutoff_at`, `feature_snapshot`, `reasons`).
  Đã có bản cùng rule_version và không force → trả bản cũ; force → chấm lại, trả `matches_previous`.
- Backfill: toàn bộ order của batch sim, lô 1.000; in phân bố low/medium/high. Không ghi đè 8 dòng sẵn có.
- **Đơn mới qua API** ghi vào batch `olist_vn_sim` (FK ghép bắt buộc cùng batch, và lịch sử chỉ tính trong cùng batch),
  `source_record_id` dạng `live:<uuid>`, identity `available_at = placed_at`. Ghi rõ trong README; hỏi team data
  có muốn batch riêng không (nếu có, rule phải đọc lịch sử từ cả hai batch).
- High → `orders.review_status='hold'` + `review_decisions(actor_type='policy', action='hold')`.

## 6. API, đồ thị, agent

API (`/api/v1`, header `X-Shop-ID`):

| Method | Path | Ghi chú |
|---|---|---|
| POST | /checkouts | tạo checkout 1..n shop → assessment từng order |
| GET | /orders | đơn của shop, lọc `risk_level`, `review_status` |
| GET | /orders/{id} | chi tiết, assessment mới nhất, reasons + facts, timeline event |
| POST | /orders/{id}/score?force=true | chấm lại |
| POST | /orders/{id}/decision | hold / release / keep_hold, cùng transaction với `review_status` |
| POST | /orders/{id}/events | thêm event (demo replay) |
| GET | /orders/{id}/network | đồ thị con |
| GET | /rules, /rules/versions; GET/POST /rule-proposals (+approve/reject) | |
| POST | /orders/{id}/explain, /orders/{id}/investigate, /rule-proposals/generate | agent |

Đồ thị (`network_service`): từ checkout của đơn đi qua `phone_key` / `device_key` / `address_cluster_id` tới checkout khác
trước cutoff, 2 bước, ≤ 200 node, dựng bằng `networkx`; node shop khác chỉ hiện "shop khác".

Agent (OpenAI, structured output):

Triển khai KBS hiện tại: Explain diễn giải tối đa ba rule đã fire bằng tiếng Việt dễ hiểu,
gồm lịch sử liên quan và câu hỏi xác minh với khách. Kiểm tra rule và số liệu theo assessment;
LLM không tính điểm hoặc quyết định xử lý. Cache LRU theo assessment
trong từng process, fallback có mã lý do. Network đã có API nội bộ với tối đa 200 node;
checkout ngoài shop chỉ xuất số tổng hợp theo loại liên kết, không xuất node định danh riêng.
Chi tiết và cấu hình: [Explain và Network](../kbs-explain-network.md). Investigate và Propose bên
dưới vẫn là kế hoạch, chưa triển khai; frontend/gateway Network thuộc phần integration tiếp theo.

- **Explain**: ≤ 3 đoạn tiếng Việt từ `reasons` + `feature_snapshot`; rule_id dùng để kiểm tra nội bộ, không đưa vào lời giải thích; kiểm số; template fallback.
  Cache theo assessment (bảng phụ hoặc file cache nếu không muốn sửa schema).
- **Investigate**: tools whitelist SQL template, `network`, `similar_cases` (cosine trên vector fact so với các đơn có
  `case_resolutions`), `get_policy`. Resolution có `responsible_party` giúp phân biệt lỗi buyer / seller / carrier.
- **Propose**: `rule_stats` = precision theo resolution verified buyer; chỉ đổi điểm/ngưỡng rule có sẵn; ghi
  `rule_proposals` (`batch_id`, `base_rule_version`), duyệt → `rule_versions` mới.

## 7. Thể lệ và khai báo

- Gói `olist_cod` là **dữ liệu** team data chuẩn bị trước → đưa vào repo ở `data/olist_cod/` (không kèm `analysis/`),
  README ghi: "Dataset do thành viên team chuẩn bị trước hackathon từ Olist (công khai) + lớp mô phỏng có khai báo;
  script sinh dữ liệu không thuộc bài nộp."
- `analysis/olist_kbs.py` là bộ chấm tham chiếu viết trước giờ thi → **không** copy, không import. Code chấm viết lại
  bằng SQL tại chỗ. Bộ chấm đó còn lặp toàn bộ đơn cho mỗi lần chấm nên cũng không dùng được cho API.
- Nên hỏi BTC một câu: mang dataset do team tự sinh trước có được không.

## 8. Test

- `unit`: engine (bậc, điều kiện phụ thuộc, unknown không fire, cap 100).
- `acceptance`: 8 kịch bản trong `olist_scenario_expectations.json` khớp **đúng** `score`, `risk_level`, tập rule.
  Đây là test quan trọng nhất; khi lệch, sửa SQL cho khớp ngữ nghĩa mục 3, không sửa kỳ vọng.
- Point-in-time: thêm event/resolution có `available_at` sau cutoff cho một checkout cùng SĐT → điểm không đổi.
- Tất định: force 3 lần → cùng kết quả. Tenant: shop A không thấy đơn shop B.
- X01: checkout nhiều shop không làm R4/R5 fire.

## 9. Timeline ngày thi (7 giờ)

| Giờ | Việc | Xong khi |
|---|---|---|
| 0:00–0:20 | Repo từ template, constitution, `docker compose up` postgres, `scripts/load_data.sh` | DB có 2 batch, 8 assessment sẵn |
| 0:20–2:00 | `rules.json` + engine + `facts.sql` + scoring + test 8 kịch bản | 8/8 khớp điểm và rule |
| 2:00–2:30 | Backfill + phân bố nhãn | Mọi order sim có assessment |
| 2:30–3:30 | API multi-tenant, tạo checkout, decision, events | Tạo đơn mới ra điểm ngay |
| 3:30–4:30 | Streamlit: danh sách, chi tiết + reasons + timeline, Hold/Release, form tạo đơn | Demo không cần agent |
| 4:30–5:15 | Explain + fallback; network view | |
| 5:15–6:00 | Investigate (similar cases dùng resolution), propose nếu kịp | |
| 6:00–6:30 | Chạy kịch bản demo đầu-cuối, README "built during hackathon" | |
| 6:30–7:00 | Slide, luyện pitch | |

Mốc kiểm soát: 2:00 chưa 8/8 → demo bằng các kịch bản đã khớp, ghi lệch vào known issues, đi tiếp.
Cắt nếu trễ: propose → investigate → network → explain chỉ template. Không cắt: scoring, reasons, Hold/Release.

## 10. Việc cần báo lại team data

- Gói nạp được trên PostgreSQL 16 thật (đã chạy: schema + seed ~72 giây, không lỗi) → cập nhật README của họ.
- Chủ repo đã chọn: đơn demo mới ghi vào `olist_vn_sim`, ID/source record `live:<uuid>`; dữ liệu cũ giữ nguyên.
- Chỉ có 60 đơn huỷ trong batch sim → R10 gần như không fire; nếu muốn demo R10, cần một kịch bản riêng.
- Gói không có kịch bản Medium: S02 được 25 điểm (Low). Nếu muốn demo đủ 3 mức, xin thêm một kịch bản Medium
  (ví dụ R4 + R7 = 35) mà không sửa ngưỡng.
