# Kịch bản kiểm tra sau deploy

URL: https://afd.tiloneeee.app. Ghi commit, giờ kiểm tra, trình duyệt và trạng thái VPN. Dùng tài khoản buyer/seller demo đã được cấp; không ghi mật khẩu, token hoặc số điện thoại thật vào báo cáo.

## Smoke chỉ đọc (PowerShell, 1 phút)

Chạy sau khi pipeline deploy thành công:

```powershell
$base = 'https://afd.tiloneeee.app'
curl.exe --connect-timeout 10 --max-time 30 -sS -i "$base/health"
curl.exe --connect-timeout 10 --max-time 30 -sS -i "$base/api/catalog/categories"
curl.exe --connect-timeout 10 --max-time 30 -sS -i "$base/api/catalog/listings?page=1&limit=1"
curl.exe --connect-timeout 10 --max-time 30 -sS -i "$base/api/catalog/shops?page=1&limit=1"
curl.exe --connect-timeout 10 --max-time 30 -sS -i "$base/api/auth/me"
curl.exe --connect-timeout 10 --max-time 30 -sS -i "$base/.env"
```

Kỳ vọng: `/health` trả 200 và `ok` (chỉ kiểm tra frontend Nginx); ba catalog API trả 200, JSON `is_success: true`, có dữ liệu nếu đã seed. `/api/auth/me` không có token phải trả 401. `/.env` bị chặn (403/404), không trả cấu hình. Không coi HTTP 200 chứa HTML ở đường dẫn API là thành công. Kiểm tra header `X-Content-Type-Options: nosniff` và `X-Frame-Options: DENY`.

Nếu chỉ truy cập được khi bật VPN, chạy lại khi tắt VPN và ghi HTTP status/lỗi kết nối. `ERR_NETWORK_IO_SUSPENDED` hoặc timeout chưa đủ để kết luận database thiếu dữ liệu.

## Giao diện (15–20 phút)

Đặt hàng, chấm lại và quyết định **ghi dữ liệu**: chỉ dùng dữ liệu demo dành riêng để test; không chạy lại seed.

| Bước | Thao tác | Kết quả cần thấy |
| --- | --- | --- |
| Catalog | Mở `/`, tìm kiếm, lọc danh mục, mở `/shops`, chọn shop và sản phẩm. Reload URL chi tiết. | Danh sách/chi tiết đúng, không có lỗi API 5xx hoặc SPA 404. |
| Ảnh | Kiểm tra danh sách, chi tiết, giỏ hàng rồi reload. | Ảnh tải được, ảnh mẫu ổn định cho cùng sản phẩm; không có ảnh vỡ. |
| Hai thiết bị | Đăng nhập buyer ở trình duyệt A và B hoặc cửa sổ riêng tư. Mở `/buyer/orders` cả hai. | Cả hai phiên dùng được. Hai tab thường chung storage không thay thế bài test này. |
| Đăng xuất | Đăng xuất A; reload trang được bảo vệ ở A và B. | A phải đăng nhập lại; B vẫn đọc được đơn. Sau migration phiên cũ có thể cần đăng nhập lại một lần. |
| Giỏ hàng | Thêm hai sản phẩm, vào `/buyer/checkout`, chỉ chọn một. Tab khác cùng trình duyệt thêm sản phẩm thứ ba. | Sản phẩm mới không tự được chọn thanh toán; tổng tạm tính chỉ gồm mục đã chọn. |
| Checkout | Chọn địa chỉ demo, nhập số điện thoại demo theo bộ dữ liệu, chọn COD, đặt một đơn. | Có mã checkout và đơn theo shop, tổng chính thức; chỉ mục đã mua bị bỏ khỏi giỏ. Đối chiếu mã, tên, số lượng, tổng trong `/buyer/orders`. |
| Seller | Đăng nhập seller, chọn shop thuộc tài khoản, mở dashboard, danh sách và chi tiết đơn. | Đúng shop, bộ lọc hoạt động; có trạng thái, hàng hóa và assessment nếu có. |
| Menu shop | Mở menu đổi shop ở màn hình rộng và hẹp; cuộn danh sách, chọn bằng chuột và bàn phím, nhấn Escape để đóng. | Menu không bị cắt/che bởi layout; chọn shop cập nhật dữ liệu đúng shop. Sau migration 000010, placeholder đủ điều kiện có tên “Cửa hàng demo N”; tên tùy chỉnh và ID shop giữ nguyên. |
| KBS | Chọn đơn demo batch `olist_vn_sim`, ghi assessment; bấm “Chấm điểm” hoặc “Chấm lại”, reload. | Assessment được lưu, có rule version, cutoff, điểm và lý do. Không dùng điểm cố định làm chuẩn khi dữ liệu đã đổi. Không báo thành công khi lưu thất bại. |
| Giải thích | Bấm “Giải thích đánh giá” trên assessment. | Giải thích gắn đúng assessment. Nếu LLM/tunnel lỗi, UI báo lỗi; điểm đã lưu vẫn còn. Kiểm tra riêng phần này với chấm điểm. |
| Review | Thực hiện một quyết định hợp lệ trên đơn dành riêng để test rồi reload. | Có lịch sử quyết định. Nếu đơn/assessment đã đổi ở phiên khác, UI yêu cầu tải lại thay vì ghi đè im lặng. |
| Phân quyền | Buyer mở `/seller`; seller thử GET `/api/seller/shops/SHOP_ID/dashboard` với shop demo thuộc seller khác và token của chính mình. | Buyer không vào portal seller; API shop không thuộc sở hữu trả 404, không lộ dữ liệu. Không chụp/chia sẻ token. |

## Bằng chứng và điều kiện đạt

Ghi từng bước PASS/FAIL, HTTP status, thời điểm, mã đơn/assessment demo và ảnh lỗi đã che thông tin xác thực. DevTools → Network phải gọi API cùng domain qua `/api/`, không gọi IP/cổng backend hoặc KBS trực tiếp.

Đối chiếu GitHub Actions của **đúng commit**: quality, build, runtime và deploy phải hoàn tất. Tải artifact quality/build/runtime; xem Sonar JSON, Trivy summary, smoke và k6 summary. Job xanh trong chế độ report-only không có nghĩa Sonar Gate/CVE đều sạch: đọc trạng thái thực.

Đạt khi smoke, buyer/seller và phân quyền đều đạt; ghi riêng lỗi LLM/mạng. K6 CI là baseline trên stack test; không tự chạy tải lên domain demo. Kịch bản không thay thế pentest hoặc thử tải với dữ liệu thực.
