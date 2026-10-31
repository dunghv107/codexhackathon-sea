# Seed Olist và kiểm tra KBS trên server

Chạy từ checkout của repository có Git LFS. Stack phải chạy migration đến `000006`,
ba service `postgres`, `kbs`, `backend` phải sẵn sàng. Backend phải chạy với
`DEMO_MODE=true`; sau khi đổi `.env`, recreate backend bằng Compose để nhận env mới.
Script dùng Python/psycopg có trong
container KBS; máy chủ chỉ cần Bash, Docker Compose, Git LFS và `sha256sum`.

## Một lệnh seed

Với stack chạy bằng `compose.yaml` và `.env` ở root repository:

```bash
bash scripts/seed-olist-demo.sh
```

Script tự tải LFS nếu `seed.sql` còn là pointer, rồi kiểm SHA-256. Khi clone lần đầu,
cài Git LFS trước `git clone`; nếu đã clone thì chạy `git lfs install` một lần.
Nếu tải LFS báo lỗi quyền/quota, xử lý lỗi tải trước khi chạy lại; pointer không phải SQL.

Với server dùng `scripts/deploy.sh`, thư mục `current` chỉ chứa cấu hình release,
không chứa checkout/data. Giữ một checkout riêng có các file của gói này, chạy từ
root checkout đó và trỏ đến Compose của release đang chạy:

```bash
bash scripts/seed-olist-demo.sh \
  --env-file /home/alex/codexhackathon-sea/.env \
  --env-file /home/alex/codexhackathon-sea/current/release.env \
  -f /home/alex/codexhackathon-sea/current/compose.yaml
```

Điều chỉnh đường dẫn nếu server dùng `AFD_DEPLOY_DIR` khác. Không cần copy riêng
script vào `/tmp`: script tìm helper và data tương đối với checkout của nó.
Nhập mật khẩu cho `buyer_demo` và `seller_demo` khi được hỏi. Chạy tự động có thể
truyền `BOOTSTRAP_BUYER_PASSWORD` và `BOOTSTRAP_SELLER_PASSWORD` qua môi trường;
không ghi mật khẩu vào lệnh hoặc commit. Chạy lại không đổi mật khẩu đã tạo.

## Script thực hiện gì

1. Nạp `schema.sql` và `seed.sql` vào một database tạm, kiểm số dòng của 24 bảng
   theo `manifest.json`, rồi nhập vào DB của KBS trong một transaction.
2. Giữ hai batch `olist_raw` và `olist_vn_sim`, ID, thời điểm và provenance nguồn.
   Batch/rule đã tạo bởi migration/KBS được giữ nguyên nếu tương thích; lịch sử
   trùng ID hoặc rule khác cấu hình làm phần nhập rollback. Không drop schema ứng dụng.
3. Lưu checksum và manifest vào `public.olist_seed_imports`. Cùng checksum thì bỏ
   qua phần nhập khi chạy lại. Gói khác checksum hoặc DB đã nhập thủ công có ID
   trùng sẽ bị từ chối; script không tự sửa hoặc xoá lịch sử đó.
4. Gọi bootstrap hiện có để tạo hai tài khoản, shop `Olist Demo VN`, ba sản phẩm
   VND và địa chỉ buyer. Liên kết các shop của 8 kịch bản với `seller_demo` nếu shop
   chưa có chủ; từ chối chiếm shop của seller khác. Giữ `owner_actor_ref` nguồn.
5. Gọi HTTP KBS `score?force=true` cho 8 kịch bản và so điểm, mức, tập rule, version,
   cutoff với acceptance contract; gọi Network cho từng đơn. In shop/order ID để
   mở trong seller portal. Điểm mới do KBS tính và lưu; High có thể tạo policy Hold.
6. Chọn ba phone key bằng `RiskRepository.facts` và `RuleEngine` của KBS tại cutoff
   hiện tại. Đơn dùng để thử ứng viên nằm trong transaction luôn rollback. Chỉ khi
   xác thực tài khoản demo và chọn đủ ba mức mới ghi mapping, rồi kiểm checkout qua Go.

Kết quả lịch sử: S01=0, S02=25, S03=100, S04=100, S05=0, S06=0, X01=0, X04=0.
KBS chỉ chấm `olist_vn_sim`; `olist_raw` vẫn được lưu để giữ gói nguồn.
Database tạm được dọn khi script kết thúc. Cần dung lượng cho cả DB tạm, dữ liệu
nhập và WAL trong lúc chạy; dành vài GB trống cho gói này.

Import, bootstrap và kiểm HTTP là các bước riêng. Nếu bootstrap/HTTP lỗi sau khi
import đã commit, dữ liệu vẫn còn: sửa lỗi và chạy lại. Mỗi lần kiểm HTTP tạo thêm
8 assessment lịch sử bất biến. Bước phone tạo tối đa ba checkout COD ở lần đầu,
mỗi đơn mua một `live:bootstrap-listing-1` và trừ một sản phẩm khỏi tồn kho thực tế.
Chạy lại tái dùng đơn tương ứng của buyer demo, kể cả khi lần trước mất HTTP response;
không reset quyết định seller hoặc kết quả cuộc gọi, không ghi đè assessment cũ.
Không xoá receipt để cố nhập lại trên DB đang có dữ liệu.

## Ba số demo sau khi seed thành công

Các số dưới đây chỉ là fixture mô phỏng, không phải số điện thoại gốc của Olist.
Chúng chỉ nối checkout mới với lịch sử thông qua `phone_key` trong `olist_vn_sim`.

| Số nhập ở Checkout | Mức kiểm tra | Cách chọn |
| --- | --- | --- |
| `0900000001` | Low | Ưu tiên key S01, kiểm lại bằng KBS |
| `0900000002` | Medium | Chính xác 2 resolved / 2 buyer-failed trong cửa sổ R1; chỉ R1 fire |
| `0900000003` | High | Ưu tiên key S03, kiểm lại bằng KBS |

Với rule `draft-vn-olist-0.2`, Medium trên có R1 = 0,55 và 30 điểm. Candidate từ
lịch sử nhiều lần giao thất bại chỉ là gợi ý tìm kiếm; thời điểm, resolution mới nhất,
batch và toàn bộ rule đều do SQL/engine hiện tại kiểm tra. Không sao chép bộ chấm tham chiếu.

Script kiểm điểm qua seller API, kiểm identity thực sự dùng mapping và số đã che;
đơn Medium phải có `order_verifications`. Console in số demo, điểm và order ID đã kiểm.
Đăng nhập `seller_demo`, chọn `Olist Demo VN` để xem các đơn; card xác minh dùng
simulator `/demo/phone`. Nếu đã xử lý đơn ở lần trước, script giữ nguyên kết quả đó;
để diễn tập cuộc gọi mới, buyer tạo một checkout COD khác bằng số Medium.

Mapping đã có được giữ nguyên và kiểm lại, không tự chuyển sang key khác. Nếu lịch
sử hết cửa sổ, rule/giỏ hàng thay đổi hoặc mapping không còn cho đúng mức, script
báo lỗi thay vì ép điểm. Nếu HTTP lỗi sau khi mapping/checkout đã commit, các bản
ghi đó được giữ lại để chạy lại; không có transaction chung giữa Go và script.

## Phạm vi dữ liệu và kiểm thử

Dataset do thành viên team chuẩn bị trước hackathon từ Olist công khai cùng lớp
mô phỏng có khai báo; script sinh dữ liệu không thuộc bài nộp. Gói được version tại
`kbs/docs/data-structure/olist_cod/olist_cod/`: seed bằng Git LFS, schema, rule,
kịch bản, acceptance contract và metadata nguồn. Các CSV/JSON xuất trùng, code
phân tích tham chiếu và đáp án kín không được đưa vào gói chạy.

Script không gán số điện thoại thật cho dữ liệu cũ, không dịch thời gian lịch sử
và không chạy backfill toàn bộ đơn. Gói 8 kịch bản vẫn không có Medium: Medium
được kiểm bằng checkout mới riêng, không sửa acceptance contract lịch sử.
Kiểm Go/KBS qua HTTP ở đây chưa thay thế kiểm trình duyệt hoặc gọi LLM thật.

Kiểm script trên PostgreSQL 17 riêng biệt, không dùng DB đang làm việc:

```bash
docker build --target runtime -t codexhackathon/kbs:seed-test kbs
docker build --target runtime -t codexhackathon/backend:seed-test backend
bash tests/seed-olist-demo.sh
```

Test tạo Compose project riêng, kiểm rollback khi trùng ID, nhập đủ gói, chạy lại
giữ password/giá/tồn kho/provenance, 8 điểm lịch sử, ba checkout qua Go, xác minh Medium,
mapping lệch mức và dọn DB tạm. Nó tự xoá đúng
container và volume của project test khi kết thúc.
