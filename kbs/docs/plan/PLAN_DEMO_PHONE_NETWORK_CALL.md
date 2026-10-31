# Plan demo: phone identity, Network UI và xác minh cuộc gọi mô phỏng

Trạng thái: đã triển khai P0/P1/P2 bản phím + TTS trên nhánh `feat/demo-phone-network-call`.
Hướng dẫn chạy và giới hạn: [PHONE_NETWORK_CALL.md](../../../docs/PHONE_NETWORK_CALL.md).
Smoke HTTP trên PostgreSQL cô lập đã kiểm Low 0 / Medium 30 / High 70 và R10 ở đơn sau;
đây là fixture tự tạo, chưa phải acceptance dataset gốc. Edge đã chạy checkout → Network →
simulator → xác nhận → seller Release; đã kiểm fallback khi thiếu TTS, chưa nghe giọng đọc thực tế.
Mốc 13:15–17:00 do chủ repo đưa ra; giả định dừng code lúc 16:00. Đây là lịch dự kiến,
không phải xác nhận giờ kết thúc cuộc thi. Không thay đổi R1–R10, ngưỡng hoặc `rules.json` để ép demo.

## 1. Nền trước triển khai và phạm vi

- Checkout React chưa nhận phone; Go tạo identity snapshot nhưng các khóa còn NULL.
- Go `RiskClient.Score()` hiện trả error, bỏ qua `risk_level` của KBS.
- KBS đã có score, Explain qua LiteLLM và Network nội bộ; Network chưa nối gateway/UI.
- `confirmed` đã hợp lệ trong schema, nhưng Go chưa có workflow xác minh cuộc gọi.
- Buyer display status đang suy ra từ review/fulfillment, không từ event `confirmed`.
- Chưa có `checkout_contacts`, `demo_phone_mappings`, `order_verifications` hoặc `/demo/phone`.

Lần triển khai này cần sửa frontend, Go và migration; KBS chỉ bổ sung classifier nếu làm
phần trả lời tự do. Scoring và policy Hold vẫn thuộc KBS. Go sở hữu contact, verification,
quyền shop và seller Hold/Release. Investigate, Propose và R9 nằm ngoài phạm vi.

Khi bắt đầu code: lấy `dev` mới nhất có phần Explain/Network; nếu PR #5 chưa merge thì cần
đưa dependency đó vào nhánh triển khai. Không tiếp tục từ một nhánh KBS cũ thiếu integration.

## 2. P0 — Thu phone và nối lịch sử

### Luồng và lớp chịu trách nhiệm

1. `frontend/src/pages/buyer/Checkout.tsx`: thêm input `type=tel`, validation và gửi `phone`.
2. Go DTO/handler → `CheckoutCommand`: nhận phone nhưng không nhận `phone_key` từ client.
3. `PhoneIdentityService`: chuẩn hóa số VN về một dạng canonical, tra mapping demo hoặc tạo HMAC.
   Các dạng tương đương như số nội địa và `+84` phải tạo cùng key; số không hợp lệ trả 400 theo convention Go.
   Validation chỉ kiểm định dạng, không tuyên bố số tồn tại hay đã được xác minh sở hữu.
4. Trong transaction checkout: lưu contact riêng và ghi `phone_key`,
   `available_at = checkouts.placed_at`, cùng batch `olist_vn_sim`.
5. Commit rồi mới gọi KBS như hiện tại. Lỗi scoring không làm checkout đã lưu trở thành thất bại.

`device_key` và `address_cluster_id` tiếp tục NULL khi chưa có nguồn thật. Không sao chép
hai khóa này từ đơn S01/S03 chỉ để đạt điểm mong muốn.

### Dữ liệu và cấu hình tối thiểu

Migration bổ sung, không đổi provenance hoặc backfill contact vào dữ liệu cũ:

| Bảng | Cột chính / ràng buộc |
|---|---|
| `sfd.checkout_contacts` | `checkout_id`, `batch_id`, `phone_e164`, `key_source` (`demo_mapping`/`hmac`), `created_at`; một dòng/checkout, FK ghép batch + checkout |
| `sfd.demo_phone_mappings` | `batch_id`, `phone_e164`, `phone_key`, `scenario_label`, `source_reference`; unique batch + phone, chỉ batch mô phỏng |

Mapping chỉ hoạt động khi `DEMO_MODE=true`, mặc định false. Script chuẩn bị demo kiểm tra
key đích có lịch sử trong batch sim; bảng chỉ chứa các số fixture do team chọn, không dùng số
của người ngoài làm số demo. Ghi rõ mapping là dữ liệu mô phỏng, không phải suy luận danh tính thật.

Số không thuộc mapping dùng `HMAC-SHA256(PHONE_HMAC_SECRET, canonical_phone)` với namespace
`hmac:v1:`. Secret do Go đọc từ env riêng, ổn định qua restart; không hard-code, log hoặc gửi
cho frontend/KBS. Thiếu secret phải báo cấu hình sai, không sinh key ngẫu nhiên mỗi lần chạy.

Seller DTO chỉ có `phone_masked`; không serialize entity contact trực tiếp. Không trả phone thật,
phone_key hay mapping qua buyer/seller list/detail, Network, log, prompt hoặc URL.
KBS chỉ cần identity snapshot, không cần truy vấn bảng contact. Không coi bảng riêng là bằng chứng
đã tách quyền DB: deployment dùng chung DB role hiện tại vẫn cần kiểm soát quyền khi ra ngoài demo.

### Chuẩn bị ba trường hợp

Truy vấn PostgreSQL đã nạp dataset; không đọc `seed.sql`, CSV lớn, latent outcomes hoặc evaluator.
Chỉ chọn key và thống kê aggregate cần thiết. Không sửa kỳ vọng acceptance gốc.

| Trường hợp | Cách chọn | Điều kiện chấp nhận |
|---|---|---|
| Low | Key của S01 là ứng viên đầu tiên | Checkout mới thực tế có score <30 |
| Medium | Key có 2 checkout resolved, cả 2 thất bại verified buyer trong cửa sổ R1 | R1 = (2 + 2×0,1)/(2 + 2) = 0,55 → 30 điểm; kiểm tổng các rule còn lại |
| High | Key của S03 là ứng viên đầu tiên | Checkout mới thực tế có score ≥60 |

Tên kịch bản cũ không bảo đảm mức điểm của đơn mới. Cutoff, shop, số tiền, tuổi buyer và các
identity khác đều có thể khác. Riêng Medium 30 điểm yêu cầu các rule khác không cộng thêm;
kiểm R3/R4/R5/R7/R8/R10 và giữ device/address cluster unknown. Một key có 2/2 thất bại chỉ
chứng minh phần R1, không tự chứng minh tổng score.

Chốt buyer, shop, giỏ hàng, currency, ba số fixture và thứ tự trình diễn trong checklist demo.
Chạy lại đúng thứ tự này vì checkout tạo thêm có thể thay đổi velocity và baseline tiền.
Nếu không tìm được key phù hợp tại cutoff hiện tại: báo thiếu dữ liệu, hoặc tạo lịch sử fixture
mới có provenance `live:demo:*` trong DB demo riêng; không chỉnh lịch sử gốc hay giả score.

### Contract Score và tiêu chí xong

Đổi `Score()` thành trả `ScoreAssessment` gồm `assessment_id`, `order_id`, `risk_level`, `score`
và error; validate enum/range/id ở biên HTTP. Cập nhật interface, client, buyer/seller callers và mocks.
Kết quả này chỉ dùng nội bộ; buyer response không nhận risk level hoặc score.

Xong khi: ba checkout mới có đủ Low/Medium/High đã kiểm thực tế; contact/snapshot atomic;
restart không đổi key; seller chỉ thấy số che; phone sai không ghi đơn hay giảm tồn kho.

## 3. P1 — Network từ KBS ra seller UI

- Go thêm `RiskClient.Network()` và `GET /api/seller/shops/{shopId}/orders/{orderId}/network`.
  Client hiện cố định POST cho mọi request, nên phải bổ sung HTTP method cho helper hiện có;
  giữ nguyên POST của Score/Explain và bounded response/timeout/no redirects.
- Handler lấy shop đã được middleware kiểm quyền; service kiểm order thuộc shop/batch trước
  khi gọi KBS. Không nhận header shop do browser tự cung cấp làm nguồn quyền.
- Trả nguyên contract `order_id`, `input_cutoff_at`, `nodes`, `edges`, `truncated` sau validation.
  Không thêm truy vấn làm lộ phone hoặc ID shop khác ở Go.
- Thêm panel tại `frontend/src/pages/seller/OrderDetail.tsx`: đồ thị SVG nhỏ theo ba cột
  checkout → loại identity → lịch sử/aggregate; kèm danh sách chữ có thể đọc bằng bàn phím.
  Dùng contract hiện có, không thêm thư viện graph chỉ cho ≤200 node.
- Có loading, retry, empty/unknown, lỗi KBS và thông báo truncated. Hiển thị cutoff và lưu ý
  các count phone/device/address có thể trùng nhau; không cộng chúng thành tổng số khách.

Xong khi: cùng shop xem được, khác shop bị chặn, NULL keys không tạo liên kết giả,
node shop khác chỉ là aggregate và panel vẫn dùng được khi KBS lỗi.

## 4. P2 — Xác minh qua cuộc gọi mô phỏng

### Điều kiện tạo và phục hồi khi lỗi

Sau Score thành công, Go gọi `VerificationService.EnsureForOrder()` trên cả luồng checkout
và seller score/retry. Service khóa order rồi kiểm trạng thái DB mới nhất:

`batch=olist_vn_sim AND latest risk=medium AND payment=cod AND fulfillment=unfulfilled
AND review IN (pending, hold)`.

Không dùng riêng HTTP risk response đã cũ để ra quyết định. Chưa có assessment hoặc KBS lỗi
thì không tạo verification. Nếu KBS đã lưu assessment nhưng Go timeout, seller retry Score
sẽ đọc assessment và tạo verification còn thiếu. Không dựng queue, worker hoặc cron riêng.
Lỗi ghi verification sau commit không làm buyer mua lại; seller có thể thử lại thao tác score.

### Một bảng cho workflow xác minh

`sfd.order_verifications`:

- `verification_id`, `batch_id`, `order_id`, `assessment_id`, `created_at`, `updated_at`.
- `status`: `pending`, `confirmed`, `declined`, `unanswered`.
- `channel='demo'`, `response_source` (`dtmf`/`llm`), nullable `answered_at`,
  nullable `handled_by_seller_id` để biết người vận hành mô phỏng.
- Unique `(batch_id, order_id)` cho bản demo một lần xác minh/đơn; FK assessment phải khớp
  order và batch, không chỉ kiểm existence của assessment ID.

Không chép raw phone/transcript/audio vào bảng này. Contact nằm ở checkout; verification
theo order, nên checkout nhiều shop có thể có nhiều verification riêng, không xác nhận hộ
order khác shop. Hỗ trợ nhiều lần gọi có audit riêng là phạm vi sau demo.

### API và UI mô phỏng

| API/Trang đề xuất | Vai trò |
|---|---|
| Seller OrderDetail | Thêm `verification` hoặc null, chỉ trường trạng thái, thời gian, nguồn và số che |
| `GET /api/seller/shops/{shopId}/orders/{orderId}/verification` | Đọc verification đúng shop; refresh trong lúc demo |
| `POST /api/seller/shops/{shopId}/orders/{orderId}/verification/respond` | Nhận `verification_id` và phím `1`/`2`; ghi kết quả idempotent |
| `/demo/phone` | Trang riêng ngoài buyer layout, yêu cầu seller/operator đã đăng nhập và `DEMO_MODE=true` |

Trang hiển thị rõ “Cuộc gọi mô phỏng — không gọi tới số điện thoại thật”. Seller card mở
trang bằng order/verification ID, không truyền phone thật hoặc JWT trong URL. API luôn kiểm
shop/order ownership và demo mode; không mở endpoint ghi kết quả công khai chỉ dựa trên ID.

Kịch bản tối thiểu: “Bạn có xác nhận nhận đơn này không? Bấm 1 để xác nhận, 2 nếu không xác nhận”.
Nút “Bắt đầu” phát TTS; luôn có text và phím bấm nếu máy thiếu giọng hoặc không phát được.
TTS dùng [SpeechSynthesis](https://developer.mozilla.org/en-US/docs/Web/API/SpeechSynthesis),
kiểm giọng trên máy demo trước giờ trình diễn, không giả định mọi máy có giọng Việt.

`1 → confirmed`, `2 → declined`, bỏ dở giữ pending. Có thể thêm “Kết thúc — không trả lời”
để ghi unanswered trong demo; không cần bộ đếm/worker timeout ở server cho MVP.
Đây là luồng phím xác định; không gọi LLM để phân loại một giá trị vốn đã rõ.

### Ghi event và bảo vệ trạng thái

Transaction response khóa order (mọi đường ghi verification đều lấy cùng khóa này), kiểm assessment hiện tại
và điều kiện eligible trước khi chốt. Nếu pending đã lệch assessment, trả 409 để seller refresh/
retry Score; Ensure cập nhật reference của bản pending nếu assessment mới vẫn eligible.
Không tự mở lại bản đã có kết quả.

- Bấm lặp cùng kết quả: trả kết quả cũ, không thêm event.
- Bấm kết quả trái với terminal state: 409, không ghi đè bằng chứng.
- `confirmed`: cập nhật verification và thêm đúng một `order_events.confirmed` trong cùng
  transaction. `actor_type='platform'` để ghi đúng bản chất mô phỏng; reason nêu simulated
  confirmation, `source_record_id` dựa trên verification ID để unique theo batch.
- `declined`/`unanswered`: chỉ cập nhật verification, không tự hủy đơn.
- `occurred_at = available_at = thời điểm server ghi nhận`; không backdate về cutoff.
- Không đổi score, assessment, review_status hoặc fulfillment. Seller vẫn Hold/Release thủ công.

Event xác nhận không làm buyer display status thành confirmed. Nó cũng không tự kích hoạt R10:
R10 cần **ít nhất hai checkout** cùng phone có buyer cancellation sau confirmation trong cửa sổ
rule và cả hai event phải đã biết trước cutoff của đơn sau. Test riêng điều này, không chỉ
assert rằng event mới tồn tại.

### Phần mở rộng nếu còn thời gian

Thêm ô trả lời bằng text trước, rồi STT nếu thiết bị demo hỗ trợ ổn định. Khi đó mới thêm
classifier với enum `confirmed`/`declined`/`unclear` và structured output. Tái sử dụng transport
Responses/LiteLLM, cấu hình và timeout của Explain; không tái sử dụng schema chọn rule ID.
Go gọi classifier server-to-server sau kiểm quyền, không đưa API key ra browser.

KBS classifier chỉ trả phân loại; Go vẫn kiểm trạng thái và quyết định có ghi event. Input
mơ hồ, phủ định, từ chối, timeout hoặc output sai → unclear, giữ pending và yêu cầu dùng phím.
Chỉ gửi transcript ngắn cần phân loại, không phone/raw contact. Không lưu audio/transcript dài
cho MVP. Phần này có test phủ định và injection, không dựa vào một prompt đơn lẻ để cấp quyền.

## 5. Các test bắt buộc và checklist demo

| Nhóm | Kiểm tra tối thiểu |
|---|---|
| Phone | Chuẩn hóa tương đương, reject input sai, HMAC ổn định, mapping chỉ bật ở demo, không lộ raw phone/key |
| Checkout | Contact + snapshot cùng transaction; rollback không giảm stock; multi-shop dùng một phone snapshot |
| Scoring | Ba checkout mới đúng mức đã ghi nhận; raw batch bị loại; manual retry tạo verification còn thiếu |
| Network | Route GET đúng, tenant isolation, unknown/empty, 200-node cap và redaction không bị gateway làm mất |
| Verification | Chỉ Medium COD eligible; Low/High/prepaid/fulfilled/released không tạo; repeat/concurrent submit một event |
| Race | Force-rescore, Release, cancel đồng thời với trả lời: recheck dưới lock; không xác nhận đơn đã đổi trạng thái |
| Lịch sử | Confirmation hiện tại không đổi score hiện tại; chỉ các đơn sau đủ điều kiện mới có thể fire R10 |
| UI | Không có risk trong buyer payload; seller card update; không có TTS vẫn dùng phím được |

Demo trình diễn: Low → xem điểm; Medium → mở phone simulator → bấm 1 → seller xem confirmed
nhưng order vẫn pending/hold → seller Release; High → policy Hold → Explain/Network.
Thử thêm một lượt phím 2 để chứng minh không tự hủy hoặc Release.

Test unit/component dùng mock; integration dùng PostgreSQL cô lập. Acceptance 8 kịch bản gốc
cần DB dataset và không thay expected. E2E trình duyệt cần frontend + Go + KBS + DB cùng chạy.
`scripts/verify.sh`, Go tests, frontend test/build và smoke thật đều phải có kết quả trước demo.

## 6. Lịch ưu tiên và điểm dừng

60 phút phone + 40 phút Network + 90 phút AI Call = 190 phút: bắt đầu 13:15 sẽ tới 16:25.
Không thể giữ nguyên cả ba ước lượng nếu một người dừng code lúc 16:00.

### Một người: ưu tiên demo lõi

| Giờ giả định | Việc | Mốc ra quyết định |
|---|---|---|
| 13:15–14:15 | P0 phone + ba mức | Chưa có ba mức thì tiếp tục P0 và cắt phần sau |
| 14:15–14:55 | P1 Network UI | Ưu tiên panel đơn giản, không thêm animation |
| 14:55–15:40 | P2 bản phím + TTS + seller card | Mục tiêu rút gọn, không STT/LLM trả lời tự do; 45 phút là timebox, không cam kết đủ |
| 15:40–16:00 | Smoke và sửa lỗi chặn demo | Nếu P2 chưa chạy chắc chắn, bỏ khỏi kịch bản trình diễn |
| 16:00–17:00 | Freeze, chạy lại demo và slide | Không mở rộng tính năng |

### Hai người

P0 và contract Score chốt trước 14:15. Sau đó một người làm Network (đến khoảng 14:55),
một người làm verification/phone simulator (90 phút, đến khoảng 15:45). Chốt quyền sở hữu
các file OrderDetail/router để tránh conflict; dùng 15 phút cuối tích hợp. STT chỉ thêm khi
core đã xanh và vẫn còn buffer, không lấy thời gian chạy thử để hoàn thiện voice.

Commit/PR nên chia: phone identity → Network UI → call simulation; classifier/STT là phần
tùy chọn sau đó. Migration đánh số theo HEAD lúc triển khai. Không tạo nhánh/commit/runtime
change chỉ để lập plan này.
