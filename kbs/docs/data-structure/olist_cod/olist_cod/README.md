# Olist → KBS COD

Khung giao dịch và thời gian là dữ liệu thật từ Olist; định danh, COD và kết cục giao là lớp mô phỏng có khai báo, vì Olist không chứa các thông tin đó.

## Cách dùng

- Database PostgreSQL trống: chạy `psql -v ON_ERROR_STOP=1 -f schema.sql`, sau đó `psql -v ON_ERROR_STOP=1 -f seed.sql`. Schema đích là `sfd`; không chạy schema mới lên database đã có `sfd`.
- Database prototype có sẵn: `migration.sql` là đề xuất thay đổi cho data/BE, cần rà soát dữ liệu cũ trước khi áp dụng. Không tự backfill cutoff hay đoán rule_version. Chưa áp dụng lên server dùng chung.
- Database đã nạp gói olist_cod trước lần cập nhật này: chỉ chạy `migration_batch.sql`. Migration suy batch từ FK; rule version không có assessment hoặc dùng chung nhiều batch sẽ làm migration dừng để xử lý nguồn rõ ràng. Không chạy cả hai migration.
- Đọc `operational.json` hoặc `tables/*.csv` nếu chưa dùng PostgreSQL. CSV ô trống là NULL; dùng seed.sql để bảo toàn kiểu/null.
- Tái tạo trong workspace: `python -X utf8 analysis/build_olist_cod.py`. Kiểm thử evaluator: `python -m unittest discover -s analysis -p test_olist_kbs.py`.
- Chấm lại một order: `Evaluator(data, config).score(order_id)` trong `analysis/olist_kbs.py`. Chỉ chấm batch mô phỏng, cutoff lấy từ checkout.

## Phạm vi và provenance

Hai batch cùng schema, ID namespace `raw:` và `sim:`. Chọn 40 seller nhiều checkout nhất và giữ nguyên toàn bộ giỏ liên quan, vì vậy số seller cuối lớn hơn 40. Dữ liệu nguồn không bị sửa; `source_subset/` giữ toàn bộ cột gốc của các dòng giao dịch được chọn. Catalog không bịa tên, tồn kho hoặc giá hiện tại.

`olist_raw` có is_synthetic=false. Chuẩn hóa grain sang checkout/shop-order; phone/device/account age/household address chưa biết để NULL. Buyer dùng customer_unique_id. Payment method gốc, review nguyên văn; không có nhãn fraud hoặc COD giả. Delivery nguồn ở cấp checkout, không chứng minh outcome riêng từng shop; available_at raw giữ NULL nếu không biết. Trạng thái released ở đơn raw đã giao/chuyển chỉ là ánh xạ workflow để thỏa constraint, không phải seller đã duyệt KBS. Hủy raw không sinh event vì thiếu giờ hủy.

`olist_vn_sim` có is_synthetic=true. `transformations.json` ghi timezone, offset, tỷ giá giả định, seed, mapping vùng và overrides. `source_inventory.json` ghi SHA256 các file gốc; batch trỏ inventory cùng hash tổng hợp và hash transformation. source_record_id giữ khóa Olist. Cả 24 bảng có batch_id trực tiếp: data_batches dùng khóa chính, 23 bảng còn lại có NOT NULL và FK đến data_batches. Năm bảng vừa bổ sung là case_analyses, risk_assessments, review_decisions, rule_versions, rule_proposals; FK ghép với batch_id chặn tham chiếu khác batch. Không trộn batch vào lịch sử chấm.

Địa chỉ, số nhà, mã xã SIM đều hư cấu, không phải danh mục hành chính hay địa chỉ giao hàng thật. parse_ok chỉ xác nhận chuỗi sinh có cấu trúc; R9 vẫn tắt vì chưa có bộ xác thực VN. Confirmed là sự kiện mô phỏng dùng timestamp approval, không khẳng định payment approval là xác nhận của buyer ngoài đời. R10 bật trong phiên bản draft mới.

Raw giữ payment_value; checkout total là tổng item+freight. `quality/payment_discrepancies.jsonl` báo chênh lệch thay vì sửa nguồn. COD mô phỏng có khoản phải thu pending, không giả lập đã thu tiền. Review không sao chép sang mô phỏng vì nội dung gốc có thể mâu thuẫn outcome mới. Đơn 3 ngày cuối và outcome chưa quan sát tại as_of không có kết cục.

## Kiểm chứng

30,000 checkout nguồn; 305 seller sau giữ đủ giỏ. COD 64.82%; thất bại / toàn bộ đơn-shop COD 10.00%.

| Mã | Kịch bản | Nhãn kỳ vọng / thực tế | Điểm | Rule |
|---|---|---|---:|---|
| S01 | Lịch sử giao tốt, giá trị bình thường | low / low | 0 | — |
| S02 | Tài khoản mới, COD lớn: draft không mặc định High | low / low | 25 | R7, R8 |
| S03 | Nhiều tài khoản cùng phone, lịch sử buyer từ chối đã xác minh mô phỏng | high / high | 100 | R1, R2, R3, R6 |
| S04 | Burst thật về thời gian; nhóm định danh mô phỏng liên shop | high / high | 100 | R1, R2, R3, R4, R5, R6 |
| S05 | Ký túc xá chung địa chỉ, phone/device riêng, giao tốt | low / low | 0 | — |
| S06 | Giao lỗi do seller; không quy trách nhiệm buyer | low / low | 0 | — |
| X01 | Một checkout nhiều shop | low / low | 0 | — |
| X04 | Khách mới sạch, không lịch sử | low / low | 0 | — |

S02 được 25 điểm (R7+R8), vẫn Low theo ngưỡng draft 30/60; không sửa ngưỡng để ép High. X04 là khách mới sạch, 0 điểm. S05 chia sẻ địa chỉ nhưng phone/device riêng. S06 có nhãn trách nhiệm seller, không tính buyer abuse. Cases/resolutions là nhãn mô phỏng đã xác minh trong thế giới demo; chưa được người thật review. `evaluation/latent_outcomes.jsonl` là đáp án kín, tuyệt đối không đưa vào feature/prompt chấm điểm.

Hai checklist cuối đã có tiêu chí kiểm thử cụ thể: `evaluation/scenario_expectations.json` cố định nhãn, điểm và tập rule của 6 nhân vật cùng 2 đối chứng, độc lập với output evaluator; pipeline dừng nếu khác. Đây là hợp đồng nghiệm thu demo theo rule version hiện tại, không phải phê duyệt nghiệp vụ production. Kiểm tra schema bao gồm cả bảng rỗng; kiểm tra dữ liệu và FK bao gồm mọi hàng; thử cố tình đặt batch NULL, batch không tồn tại và batch raw vào assessment mô phỏng đều phải bị từ chối.

SQL seed đã round-trip với FK/CHECK trên SQLite; chưa kiểm kiểu, constraint name và execution trên PostgreSQL thật. `validation.json` ghi kết quả kiểm tự động. Không triển khai UI, LLM, production scoring, rule_stats service hoặc training ML. Các ngưỡng vẫn là giả định demo, score không phải xác suất gian lận. Kịch bản đã được dựng để diễn giải rule, không phải benchmark độc lập hay bằng chứng accuracy ngoài đời.
