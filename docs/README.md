# Bàn giao ERD + khảo sát dữ liệu AI

Phiên bản v2 — ngày 30/10/2026. Seller và buyer tách riêng; thay bản v1 thiếu tài khoản seller. **Gói thiết kế database cho BE/FE và bản đồ nguồn dữ liệu cho AI, không phải demo.** Không kèm seed, dữ liệu khách mô phỏng hoặc model.

## Gửi file nào cho ai?

| Người nhận | Đọc theo thứ tự |
|---|---|
| BE | `ERD.md` → `schema.sql` → `field_dictionary.csv` → `IMPLEMENTATION_NOTES.md` |
| FE | `ERD.md`, phần platform và luồng trạng thái trong `IMPLEMENTATION_NOTES.md` |
| AI | `AI_HANDOFF.md` → `AI_DATA_MAP.csv` (hoặc bản Markdown) → `all_source_columns.csv` |

Xem `SELLER_BUYER_UI.md` cho hai portal riêng, quyền và luồng seller quản lý shop/đơn/risk.

## File chính

- **ERD.md:** hai sơ đồ Mermaid với PK/FK, cột và cardinality; bảng xuất hiện ở cả hai hình vẫn là cùng một bảng.
- **ERD_full.mmd:** sơ đồ đầy đủ 25 bảng; các file 01/02 là sơ đồ theo module. Mở bằng công cụ hỗ trợ Mermaid; không cần Figma.
- **entity_overview.svg:** ảnh tổng quan entity có thể mở bằng browser; chỉ tóm tắt cột, không thay thế sơ đồ quan hệ đầy đủ.
- **schema.sql:** DDL PostgreSQL, tạo mới schema `sfd`, không DROP dữ liệu. Là bản thiết kế review được, chưa chạy trên PostgreSQL thật.
- **field_dictionary.csv:** 193 cột của cả 25 bảng, kiểu SQL, required, PK, FK, default, module. CSV UTF-8 BOM.
- **AI_DATA_MAP.csv / .md:** 22 mục nguồn/tín hiệu/dependency/giới hạn để AI quyết định KBS.
- **all_source_columns.csv:** ánh xạ/giữ lại toàn bộ 87 cột nguồn đã khảo sát; không phải importer.
- **source_inventory.json:** 10 CSV local, số dòng, cột và checksum đã đối chiếu ở bước chuẩn bị trước.

## Quy mô thiết kế

14 bảng platform; 10 bảng identity/risk/KBS; 1 bảng nguồn gốc dữ liệu. Tách module để BE không phải triển khai toàn bộ agent ngay. Database là cấu trúc mục tiêu; không có nghĩa hai dataset hiện tại chứa đủ dữ liệu cho mọi cột.

Mô hình kế thừa phần thiết kế có sẵn để giữ nhất quán. Chưa chốt auth, SKU variants và refund settlement. Bản này phù hợp phạm vi prototype đã mô tả, không tự nhận tương đương toàn bộ Shopee.

## Kiểm tra thực hiện

Generator `analysis/build_handoff.py` đọc cùng schema.sql để xuất ERD và dictionary. Đã kiểm bằng SQLite phần DDL portable, đủ 25 bảng, 193 cột, 22 dòng mapping; không có bảng lạc module. Kiểu PostgreSQL/quyền/RLS, loader, API và UI chưa chạy. Không thay đổi raw hoặc schema thiết kế trước đó.

Sinh lại từ thư mục `E:\SFD`: `python analysis/build_handoff.py`. File tự sinh được ghi đè; README/AI_HANDOFF/IMPLEMENTATION_NOTES giữ nguyên. Không cần cài dependency.
