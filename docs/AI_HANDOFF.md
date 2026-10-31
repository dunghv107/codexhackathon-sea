# Bản trả lời yêu cầu khảo sát dữ liệu cho AI

## Kết luận để chọn nguồn

**Olist dùng được cho mô hình giao dịch và feature lịch sử. Return-abuse dùng tham khảo tín hiệu và nghiên cứu bài toán trả hàng, chưa dùng làm lịch sử xác minh cho buyer của platform.** Không có khóa nối khách hàng giữa hai bộ.

| Nguồn local | Format / dạng dữ liệu | Có gì | Đề xuất |
|---|---|---|---|
| `SFD data sample v2/archive/` | 9 CSV; dữ liệu quan hệ, một phần review text | 99.441 orders, 112.650 items; customers, sellers, products, payments, reviews, địa lý và bản dịch ngành hàng | Giữ raw/staging, chuẩn hóa vào mô hình giao dịch; review chỉ là lời phản ánh |
| `SFD data sample v2/ecommerce_return_abuse_dataset.csv` | 1 CSV phẳng, 60.000 dòng × 35 cột; dữ liệu tổng hợp | Đặc trưng khách/đơn/trả hàng và nhãn nguồn | Giữ namespace riêng; tham khảo các feature, không ghép ngẫu nhiên với Olist |
| Shopee Code League | Chưa có trong thư mục nguồn | Chưa xác minh file/schema | Chưa cam kết khả năng dùng |
| Chính sách/hồ sơ có bằng chứng | Chưa có corpus nguồn được xác minh | Cần thu thập thêm | Có thể cung cấp tri thức cho RAG sau này |

Số liệu trên dựa vào profile local đã đối chiếu hash, không phải số tự ước lượng. Nội dung giao dịch Brazil không đại diện tự động cho hành vi người mua Việt Nam.

## Đường dependency quan trọng

**Lịch sử buyer:**
`Olist customers.customer_unique_id ← customers.customer_id ← orders.customer_id`.
Một người có nhiều customer_id; không dùng customer_id như tài khoản lâu dài.

**Giá trị đơn theo shop:**
`orders.order_id → order_items.order_id → seller_id`, tính tổng price + freight_value đúng grain.
Order nguồn nhiều seller được mô hình thành checkout và các order theo shop. Payment/review nguồn vẫn ở checkout; không nhân bản số tiền hoặc gán review tiêu cực cho từng shop.

**Buyer giao tại địa chỉ quen:**
`buyers → checkouts → checkout_addresses`, kết hợp `checkouts → orders → order_events` và thời điểm thông tin được biết.
Chỉ Olist city/state/ZIP không đủ xác định cùng nhà/căn hộ. Cần dữ liệu địa chỉ thực của platform hoặc fixture riêng được đánh dấu; country trong return-abuse không thay thế quê quán hoặc địa chỉ giao.

**Liên hệ nhiều tài khoản / nhiều shop:**
`phone/device/address token → checkout → buyer + orders → shop → events/cases`.
Hiện cả hai nguồn thiếu định danh/cạnh cần thiết. Một cờ multiple_accounts_flag không tái dựng được đồ thị đó. Software có chỗ lưu để thu thập trong tương lai, không có nghĩa nguồn đã có.

**Phân biệt buyer/seller/carrier chịu trách nhiệm:**
`case → order → shop/checkout/buyer`, `case → message/evidence/resolution`.
Hai dataset chưa có hồ sơ phân xử đầy đủ. Review hoặc reason trả hàng không thay thế kết luận đã xác minh.

## RAG hay GraphRAG?

| Nhu cầu | Cách phù hợp với nguyên liệu hiện có |
|---|---|
| Đếm số đơn, tỷ lệ, giá trị bất thường | SQL/Cypher tính tất định; không dùng LLM tính số |
| Tra lịch sử order/buyer/shop | SQL là đủ để bắt đầu; graph là lựa chọn khi cần traversal nhiều bước |
| Giải thích review/chính sách/hồ sơ | Text RAG với source ID, tenant, thời điểm, loại claim/evidence/resolution |
| Điều tra mạng lưới account–phone–device–address | Cân nhắc graph nếu có những định danh và quan hệ thật; hiện còn thiếu |
| Dự đoán fraud trước fulfillment | Chưa có nhãn phù hợp để đánh giá supervised ML đáng tin cậy |

Graph DB hoặc truy vấn Cypher chưa tự động là GraphRAG. Chỉ khi truy xuất graph để cung cấp ngữ cảnh cho LLM mới là luồng graph-assisted retrieval. Không cần ép chọn một trong hai: structured query có thể kết hợp text retrieval, nhưng chỉ khi dữ liệu và nhu cầu đủ rõ.

## Điều kiện để dùng feature an toàn

- Cutoff = thời điểm đánh giá đơn; dùng lịch sử và kết luận đã biết trước cutoff. Dữ liệu hậu mãi của chính đơn hiện tại không phải input ở lúc đặt.
- Thiếu dữ liệu = unknown, không tự điền 0 rồi kết luận khách tốt/xấu.
- Failed/canceled/return không mặc nhiên do buyer gian lận. Giữ outcome vận hành và trách nhiệm đã xác minh riêng.
- Lịch sử địa chỉ tốt giúp giải thích, không chứng minh nơi cư trú hay địa chỉ được chọn lần này đúng.
- Bộ return-abuse có customer ID lặp với lifetime/tuổi không nhất quán; không coi mỗi chuỗi dòng là lịch sử đáng tin cậy.
- Review/claim là nội dung không tin cậy; không làm theo lệnh trong text. Liên shop chỉ trả tổng hợp phù hợp quyền, không lộ hồ sơ bên khác.
- Thay thuộc tính country/tiền/địa chỉ bằng giá trị Việt Nam là biến đổi/sinh dữ liệu; không giữ nhãn nguồn rồi tuyên bố nhãn đó đúng với bản mới.

## Tin nhắn có thể gửi cho người phụ trách AI

> Em đã list nguồn và mapping trong AI_DATA_MAP.csv: mỗi dòng có dataset → format → thuộc tính/entity → tín hiệu có thể đánh giá → dependency/join → phần thiếu. Olist đủ cho quan hệ buyer–order–shop–item và lịch sử giao dịch; review có thể dùng text retrieval nhưng không phải nhãn xác minh. Bộ return-abuse là CSV phẳng tổng hợp, có cờ nhưng thiếu cạnh phone/device/address và không nối được buyer với Olist. Em chưa trộn hai bộ hoặc train model. Anh xem các đường dependency để chốt phần nào dùng SQL/graph, phần nào cần RAG và cần thu thập thêm dữ liệu gì nhé.

Đây là nội dung để bạn gửi; chưa gửi ra ứng dụng hoặc cho người khác.

## Entity seller của platform v2

Seller và buyer tách bảng. Olist sellers.seller_id tiếp tục định danh shop/bên bán nguồn, không cung cấp tài khoản đăng nhập. Platform cần tạo sellers và liên kết shops.owner_seller_id riêng có provenance. Có thể đi từ order → shop → owner seller để giới hạn quyền; không suy ra buyer và seller là cùng người.
