# Demo marketplace và fraud review với Olist COD

Backend Go và frontend React dùng schema chuẩn tại `olist_cod/olist_cod/schema.sql`.
Các tài liệu ERD/IMPLEMENTATION_NOTES cũ mô tả bản thiết kế trước; schema và API_CONTRACT hiện hành là nguồn đối chiếu.
Go gọi KBS để chấm điểm từ SQL và rule config, sau đó đọc assessment đã lưu.
Không chạy evaluator của team data hoặc dùng LLM tính điểm. Các kịch bản tham chiếu nằm trong
`olist_cod/olist_cod/scenarios.json`; bộ rule hiện tại là `draft-vn-olist-0.2`.

## Database đã có schema/dữ liệu Olist

Chạy từ thư mục `backend`, đặt `DATABASE_URL` trỏ đúng database demo PostgreSQL.
Dữ liệu Olist phải được nạp riêng trước đó. Các lệnh dưới đây không nạp file dữ liệu.

```sh
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/attach_olist.sql
```

Script tạo `public.users` nếu chưa có, chạy migration 000004 và tạo migration ledger ở
version 4 nếu chưa có ledger. Không chạy migration 000002 lên database đã có schema `sfd`.
Nếu đã có ledger cũ, chạy migration runner bình thường sau bước attach để cập nhật version;
000004 có thể chạy lại. Không sửa ledger đang dirty.

Migration 000004 cũng nâng cấp database marketplace cũ qua `make migrate-up`.
Nó bổ sung các cột, kiểu sự kiện và ràng buộc batch của schema chuẩn, giữ dữ liệu cũ.
Các phần mở rộng phục vụ đăng nhập là `public.users`, `sfd.sellers`,
`buyers.auth_subject/status`, `shops.owner_seller_id`, `review_decisions.seller_id`.
`owner_actor_ref` của dữ liệu nhập được giữ nguyên; quyền truy cập do liên kết seller cục bộ quyết định.
Shop chưa liên kết không thể truy cập bằng seller API.

Migration dừng và rollback nếu dữ liệu cũ có assessment thiếu cutoff, rule không có
version tương ứng, version dùng chung nhiều batch, hoặc provenance không suy ra được.
Phải đối chiếu nguồn và sửa dữ liệu đó trước khi chạy lại; không tự gán cutoff/điểm/batch.
Migration không có rollback tự động xóa cột; muốn quay lại dùng backup.

## Liên kết tài khoản demo

Tìm shop có assessment từ database, không cần đọc file dữ liệu:

```sql
SELECT DISTINCT s.shop_id, s.display_name, o.order_id, a.score, a.risk_level,
       o.fulfillment_status, o.review_status
FROM sfd.shops s
JOIN sfd.orders o ON o.shop_id=s.shop_id
JOIN sfd.risk_assessments a ON a.order_id=o.order_id
WHERE o.batch_id='olist_vn_sim'
ORDER BY a.score DESC, o.order_id;
```

Chọn chính xác các shop cần trình diễn. Lệnh tạo tài khoản mới; không thay mật khẩu,
chiếm quyền shop hoặc ghi đè liên kết tài khoản cũ. Có thể dùng `--buyer` để liên kết
một buyer nhập sẵn, hoặc kết hợp cả buyer và shops. Mọi thay đổi cùng transaction.

```sh
# Trong backend; nhập mật khẩu qua biến môi trường riêng, không đưa vào đối số.
read -s DEMO_PASSWORD
export DEMO_PASSWORD
# Thay SHOP_ID_1,SHOP_ID_2 bằng các ID lấy từ truy vấn trên.
go run ./cmd/demo-account --username reviewer --shops 'SHOP_ID_1,SHOP_ID_2'
unset DEMO_PASSWORD
```

Tài khoản reviewer chỉ thấy các shop đã chọn. Muốn tài khoản khác quản lý cùng shop,
cần quản trị lại liên kết có chủ ý; CLI từ chối shop đã có chủ.

## Chạy ứng dụng

Backend đọc `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`
`JWT_SECRET` và `PHONE_HMAC_SECRET` (ít nhất 32 byte, giữ ổn định) từ môi trường hoặc `backend/.env`.
Bật `DEMO_MODE=true` để dùng [phone mapping và simulator](PHONE_NETWORK_CALL.md).
`DATABASE_URL` dùng cho psql/CLI/migration;
server hiện dùng các biến `DB_*`, cần trỏ cùng database. Redis dùng cho rate limit.

```sh
cd backend
go run ./cmd/server
```

Ở terminal khác:

```sh
cd frontend
npm ci
npm run dev
```

Vite proxy `/api` đến backend cổng 8000. Khi triển khai bản build, cấu hình reverse proxy
cho `/api` và SPA fallback; Vite dev proxy không được đóng gói vào `dist`.

## Luồng demo

1. Đăng nhập reviewer → chọn shop. Danh sách ưu tiên shop có đơn đã đánh giá và hiển thị batch.
2. Dashboard → chọn High/Low hoặc mở danh sách đơn; lọc `olist_vn_sim`, trạng thái review,
   hoặc tìm đúng `order_id` từ `scenarios.json` trong shop tương ứng.
3. Chi tiết đơn hiển thị điểm đã lưu, rule version, cutoff, lý do và điểm đóng góp từng rule.
   Mở dữ kiện để kiểm tra snapshot. Điểm hỗ trợ review, không phải xác suất gian lận.
4. Xem hồ sơ tranh chấp của chính đơn: lời trình bày, tham chiếu bằng chứng và các kết luận.
   Hồ sơ hiện tại có thể đến sau cutoff; không được coi tất cả là đầu vào assessment cũ.
5. Với đơn `unfulfilled` có assessment: Hold hoặc Keep Hold, nhập lý do; sau xác minh thì Release.
   Mỗi thao tác lưu decision cùng batch với đơn, không sửa assessment.
6. Đơn được Release mới đóng gói; COD không đòi payment succeeded. Thanh toán khác cần succeeded.
   Tiếp tục shipped → delivered. Đơn đã xử lý không cho thay đổi quyết định review.
7. Nút tải lại lấy assessment mới nếu một hệ thống khác đã ghi kết quả.

`S03`/`S04` là các kịch bản High; `S01`, `S05`, `S06`, `X01`, `X04` là Low 0 điểm;
`S02` là Low 25 điểm. Không có kịch bản Medium trong hợp đồng hiện tại. Các trạng thái
đơn đã nhập quyết định thao tác nào được phép; không reset đơn lịch sử để ép demo.

Buyer vẫn có catalog, địa chỉ, checkout và theo dõi đơn. Giá nguồn NULL hiển thị chưa có giá;
seller có thể bổ sung giá/tồn kho để cho mua. Checkout mới dùng batch `olist_vn_sim`, ID
`live:<uuid>`, giữ snapshot địa chỉ/giá và tạo snapshot định danh với khóa chưa biết là NULL.
Buyer, shop, product và listing phải thuộc batch mô phỏng; dữ liệu `app`/`olist_raw` cũ giữ nguyên.
Không tự lấy phone/device từ lịch sử. Go gọi KBS sau khi commit; nếu KBS lỗi, đơn vẫn được lưu,
chưa có assessment và chưa thể Release. Seller dùng nút Chấm điểm để thử lại, Chấm lại để tạo
assessment mới, Giải thích để xem tối đa ba câu theo template.

## Kiểm tra

```sh
cd backend
go test ./...
# PostgreSQL test tạo/xóa schema olist_test_<uuid> riêng, không dùng các file dữ liệu.
TEST_DATABASE_URL="$DATABASE_URL" go test ./internal/infra/persistence -run TestOlistReviewIntegration -v
cd ../frontend
npm test
npm run build
npm run lint
```

Integration test chạy cả schema Olist chuẩn và schema marketplace cũ qua migration 000004:
kiểm tra dữ kiện/detail, phạm vi shop, batch, cutoff, stale assessment, Hold/Release,
gate giao hàng, checkout mới và identity snapshot. Không có `TEST_DATABASE_URL` thì test này skip.

Giới hạn MVP còn lại: chưa có thanh toán thật, upload bằng chứng, chấm điểm hoặc đồ thị liên shop.
Phí vận chuyển cho đơn mới bằng 0; hủy đơn chưa hoàn tồn kho. Không dùng app này để kết luận
người mua gian lận dựa trên các ngưỡng mô phỏng chưa hiệu chỉnh.
