# Ghi chú BE/FE trước khi dựng database

## Các màn hình/luồng đọc từ đâu

| Chức năng | Entity |
|---|---|
| Danh sách sản phẩm theo shop | shops → listings → products |
| Sổ địa chỉ | buyers → saved_addresses; quê quán tùy chọn ở buyer_profiles |
| Tạo checkout | checkouts + checkout_addresses + orders + order_items; snapshot địa chỉ và giá trong một transaction |
| Thanh toán | payments theo checkout; COD không cần succeeded trước đóng gói |
| Seller xem danh sách đơn | orders lọc theo shop_id từ session, join checkout/items; assessment mới nhất phù hợp |
| Chi tiết risk | risk_assessments.feature_snapshot/reasons/score/risk_level; không gọi LLM để tính lại |
| Hold/Release | review_decisions + orders.review_status cùng transaction |
| Timeline | order_events, không suy timestamp từ current status |
| Hồ sơ hậu mãi | cases → messages/evidence/resolutions; có quyền truy cập và cutoff |

## Trạng thái tách biệt

- `review_status`: pending → hold hoặc released. Hold chỉ là gate trước fulfillment, không phải trạng thái thanh toán hay kết luận fraud.
- `fulfillment_status`: unfulfilled → packing → shipped → delivered; canceled/returned theo điều kiện nghiệp vụ. Schema CHECK không thay state machine; backend phải kiểm chuyển trạng thái.
- `payments.status`: unknown/pending/succeeded/failed/refunded; không tự bỏ Hold khi thanh toán thành công.
- Medium có thể released kèm gợi ý xác nhận; engine lỗi/sync chưa xong giữ pending. Không tự hủy đơn High.

## Grain và tiền

Một checkout nhiều shop tạo một order mỗi shop. Giá order = tổng(quantity × unit_price + freight_amount). Tổng checkout bằng tổng các order cùng currency. Không nhân payment theo shop. Phí giao phải được phân bổ rõ, không lặp toàn phí trên từng item. Giá hiện tại listing không sửa giá snapshot item lịch sử.

Olist là dữ liệu lịch sử, không có catalog hiện tại đủ tên/ảnh/stock. Olist source order tương ứng checkout trong model. Các source order thiếu item/seller ở staging, không đoán shop. Mốc giờ Brazil chưa có timezone không tự gắn +07:00.

## Constraints và contract cần BE hoàn thiện

DDL đã có PK/FK, unique checkout/shop, FK ghép order-item/listing cùng shop, FK decision/assessment cùng order và một địa chỉ mặc định tối đa/buyer. Không cần viết lại những constraint này bằng cách chỉ kiểm ở FE.

Các điều kiện vẫn ở service:

1. Tổng tiền, snapshot lúc checkout, item không rỗng, địa chỉ của buyer và quyền tenant.
2. Mọi assessment mới có input_cutoff_at; cột đang nullable vì kế thừa draft. Trước migration mới, nhóm có thể chốt NOT NULL cho bản ghi mới hoàn toàn.
3. `rule_version` assessment phải tồn tại và config bất biến; bản DDL hiện chưa có FK này. Nếu thêm FK, tạo rule_versions trước hoặc ADD CONSTRAINT sau khi các bảng đã tạo.
4. Score/level đối chiếu đúng version, không lấy rule hiện tại để diễn giải assessment cũ. Chỉ tái chấm khi có yêu cầu, không ghi đè lịch sử.
5. Selector seller lấy shop từ auth; kiểm quyền khi đọc/ghi và chống release assessment cũ do concurrent update. Schema chưa có RLS/auth.
6. Sửa sổ địa chỉ không sửa địa chỉ đơn cũ. Quê quán không bắt buộc và không chấm điểm khác tỉnh.
7. Data source/provenance: batch thật và synthetic không trộn nhãn. Seed ở các gói cũ không thuộc gói handoff này.

## Triển khai database

Chạy `schema.sql` một lần trong database PostgreSQL trống bằng `psql -v ON_ERROR_STOP=1 -f schema.sql`; schema tên sfd. Không chạy đè DB đã có sfd. Không có migration nâng cấp hay dữ liệu demo trong gói này.

FE không cần dùng tất cả 25 bảng trực tiếp: chỉ gọi API theo use case. Hai portal và contract quyền đã bổ sung trong SELLER_BUYER_UI.md; API/UI thực thi chưa được code.

## Sửa v2: seller riêng

Thêm sellers; shops.owner_actor_ref thay bằng owner_seller_id NOT NULL FK sellers. Buyers và sellers có auth_subject/status riêng; review_decisions có seller_id để audit đúng actor. Không dùng users chung. Shop không thể có owner không tồn tại. Backend vẫn phải kiểm seller sở hữu shop khi quyết định đơn.

Schema mới dùng cho database trống; nếu đã chạy bản v1 phải viết migration và backfill owner từ tài khoản seller được xác minh, không tự đoán từ buyer/Olist. Seed của gói demo cũ không tương thích v2.
