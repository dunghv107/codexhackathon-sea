# Phone, Network và cuộc gọi mô phỏng

Triển khai P0/P1/P2 của `kbs/docs/plan/PLAN_DEMO_PHONE_NETWORK_CALL.md`.
KBS vẫn chấm điểm bằng SQL/rules; Go quản lý contact, quyền shop và kết quả xác minh.

## Cấu hình và migration

1. Áp dụng migration `000006_phone_verifications` sau migrations hiện có bằng quy trình migrate của repo.
   Database nhập từ schema Olist dùng `backend/scripts/attach_olist.sql` khi chưa có migration ledger;
   database đã có ledger tiếp tục dùng migrate-up. Không chạy lại schema hay seed trên DB đang dùng.
2. Đặt `PHONE_HMAC_SECRET` trong `.env` của Compose, hoặc env riêng của Go khi chạy trực tiếp.
   Sinh một lần bằng `python -c "import secrets; print(secrets.token_hex(32))"`, lưu riêng và giữ ổn định.
   Go từ chối khởi động nếu thiếu hoặc dưới 32 byte. Đổi secret sẽ đổi key của số chưa có mapping.
3. Đặt `DEMO_MODE=true` để dùng mapping và mô phỏng cuộc gọi. Mặc định false.
   Đây là env của Go; không đặt secret này trong frontend hoặc cấu hình LLM.

Checkout mới bắt buộc có phone. Chuẩn hóa số VN; validation sai trả HTTP 400 theo convention Go.
Contact và identity snapshot ghi cùng transaction, seller chỉ nhận số che. Số chưa có mapping dùng
HMAC-SHA256. Không ghi raw phone hoặc phone key vào log SQL của hai thao tác này.

## Dữ liệu cũ

Không backfill số thật, đổi batch hoặc sửa identity snapshot cũ. Đã có `phone_key` thì lịch sử
vẫn dùng được dù không có số thật. Thiếu key thì tín hiệu theo phone là unknown; các tín hiệu khác
vẫn được chấm bình thường. Seller hiển thị “Chưa có số điện thoại” nếu chưa có contact.
Simulator chỉ là diễn tập có quyền seller, không phải bằng chứng đã liên lạc được với khách.

## Mapping demo trên database đã có lịch sử

Gói seed có bước tự chọn và kiểm ba số demo qua Go:
`bash scripts/seed-olist-demo.sh`. Xem [hướng dẫn seed](SEED_OLIST.md) cho điều kiện
`DEMO_MODE`, mật khẩu tài khoản demo và hành vi chạy lại. Các bước psql dưới đây
dành cho mapping tự chọn; không cần chạy chúng sau khi script đã báo PASS ba số.

Dùng các số fixture do team chọn và key thực tế trong `olist_vn_sim`. Không dùng số của người ngoài.
Chạy bằng psql với kết nối đã cấu hình riêng:

```sh
psql -v phone='+84900000001' -v phone_key='<key-da-kiem-tra>' \
  -v scenario='low-candidate' -v source='synthetic demo mapping; candidate S01' \
  -f backend/scripts/demo_phone_mapping.sql
```

Script từ chối key không tồn tại và mapping trùng, không tự ghi đè. S01/S03 chỉ là ứng viên:
đặt đơn mới với cutoff hiện tại để xác nhận mức điểm. Không thay rule để ép kết quả.
Mapping không chứa device/address cluster; hai trường này vẫn unknown ở checkout mới.

## Luồng trình diễn

- Checkout có số nhận hàng → seller xem điểm, số che, Explain và tab Rủi ro/Network.
- Medium + COD + chưa xử lý + pending/hold → một verification duy nhất khi `DEMO_MODE=true`.
  Nếu lần đầu thiếu verification do lỗi sau score, seller bấm chấm lại để phục hồi.
- Card “Xác minh qua cuộc gọi” mở `/demo/phone`; cần phiên seller và quyền shop.
  Bấm Phát kịch bản để nghe TTS; luôn có nội dung chữ và phím 1/2/0 nếu không có giọng đọc.
- Phím 1 ghi `confirmed`; 2 ghi `declined`; 0 ghi `unanswered`. Bấm lặp cùng kết quả là idempotent,
  đổi kết quả sau khi kết thúc trả 409. Suspended seller chỉ xem.
- Xác nhận không đổi assessment, review, fulfillment hay buyer display status. Seller tự Release/Hold.
  Đơn đã Release/hủy hoặc verification chưa cập nhật theo assessment mới bị từ chối.
- R10 chỉ tăng ở đơn sau khi có đủ hai checkout buyer hủy sau confirmation và dữ liệu đã available.

Network dùng GET qua gateway kiểm quyền, tối đa 200 nút; shop khác chỉ hiện aggregate.
Panel có sơ đồ SVG, danh sách chữ, trạng thái lỗi/retry và cảnh báo khi giới hạn số nút.

## Smoke HTTP với PostgreSQL local

Script tạo một database `phone_smoke_<random>`, lịch sử mô phỏng mới, chạy Go + KBS trên cổng tạm,
kiểm HTTP rồi dừng hai process và xóa **chỉ database vừa tạo**. Không đọc seed/CSV, không gọi LLM,
không sử dụng hoặc sửa dữ liệu của database nguồn. File env riêng cần `DATABASE_URL` loopback với
quyền CREATEDB. Kết quả không phải acceptance S01–S08 và không để lại server demo đang chạy.

Trên Linux/WSL có Go:

```sh
mkdir -p reports
(cd backend && go build -o ../reports/backend-phone-demo ./cmd/server)
cd kbs
ROOT_DIR="$PWD"
source scripts/uv-environment.sh
uv run python ../scripts/smoke-phone-demo.py --env-file kbs/.env.local-smoke.local
```

Nếu Go chỉ có trên Windows, build binary Linux từ PowerShell trước khi chạy lệnh uv trong WSL:

```powershell
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
Push-Location backend
go build -o ../reports/backend-phone-demo ./cmd/server
Pop-Location
Remove-Item Env:GOOS, Env:GOARCH
```

Fixture tự tạo, không phải số/key S01/S03 của dataset gốc:

| Số mô phỏng | Lịch sử biết trước checkout | Kết quả checkout đầu |
|---|---|---|
| `0900000001` | 1 giao thành công | Low 0 |
| `0900000002` | 2/2 giao thất bại verified buyer | Medium 30 |
| `0900000003` | 3/3 giao thất bại verified buyer | High 70, Hold |

Smoke còn kiểm quyền, suspended seller, masking, Network redaction, phục hồi verification, idempotency,
prepaid, invalid phone, HMAC và chuỗi R10 (+15 ở đơn sau). Go integration kiểm hai schema, rollback,
multi-shop, stale assessment và cạnh tranh với Release. Log local nằm trong `reports/` đã gitignore.

Đã chạy thêm Edge headless qua UI: nhập phone ở Checkout, mở Network, xác nhận bằng phím khi
TTS không khả dụng, rồi seller Release thủ công; không có lỗi JavaScript. Ảnh kiểm tra local
trong `reports/phone-network-seller.png` và `reports/phone-simulator.png`. Chưa kiểm chất lượng giọng đọc.

Chưa có gọi điện thật, STT hoặc classifier câu trả lời tự do. TTS phụ thuộc giọng/trình duyệt của máy demo.
