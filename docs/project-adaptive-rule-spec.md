# Đặc tả mở rộng rule thích ứng cho SFD

Ngày: 31/10/2026. Người dùng yêu cầu đặc tả để team triển khai, không sửa code.
Repo đối chiếu: dunghv107/codexhackathon-sea, dev, commit `a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f`.

## 1. Quyết định thiết kế

Giữ rule engine xác định và giải thích được. Bổ sung 18 rule ứng viên SFD-A01–A18 theo dữ liệu dự án; mở trước nhóm buyer, basket và COD. Không đặt số lượng rule làm tiêu chí thành công.

“Không hard chỉ số” gồm hai việc khác nhau:

- **Tham số hóa:** window, minimum sample, trọng số, dependency, action, ngưỡng phân mức nằm trong cấu hình version hóa.
- **Thích ứng dữ liệu:** giá trị so sánh lấy từ baseline theo buyer/shop/category/currency tại thời điểm đánh giá. Đưa số cố định từ Python sang JSON thôi chưa làm rule thích ứng.

Không có hệ thống hoàn toàn không cần tham số. Percentile mục tiêu, mức bất định chấp nhận và ngân sách review vẫn là quyết định vận hành, cần calibration. Không tự gán P95/P99 hay một mức tiền chung rồi coi đó là chuẩn.

## 2. Những gì code hiện tại thực sự hỗ trợ

| Thành phần đã đọc | Hiện trạng và tác động |
|---|---|
| [config.py](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/kbs/app/rules/config.py) | RuleId chỉ R1–R10; validator bắt đúng 10 rule. requires chỉ hai biểu thức cố định. Thêm JSON chưa đủ để thêm rule. |
| [engine.py](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/kbs/app/rules/engine.py) | Dispatch theo rule_id; R5/R8 kiểm dependency trực tiếp trong code; score cộng điểm rồi cap. Rule không có handler rơi về 0. |
| [facts.sql](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/kbs/app/rules/sql/facts.sql) | Có buyer_id nhưng lịch sử chủ yếu nối phone/device/cluster. R7 dùng median shop cùng currency, loại canceled đã biết, chưa bắt buộc giao thành công. |
| [risk_repository.py](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/kbs/app/repositories/risk_repository.py) | Đọc config active từ rule_versions trong DB; JSON chỉ bootstrap. Lưu snapshot/unknown; policy Release khi Low, Hold nếu không Low. |
| [scoring_service.py](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/kbs/app/services/scoring_service.py) | Tái dùng assessment theo order và rule_version. Baseline thay đổi nhưng version không đổi có thể khiến cache/replay không phản ánh chính sách mới. |
| [CreateCheckout](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/backend/internal/infra/persistence/order_repository_impl.go) | Snapshot ghi phone_key. Không ghi device_key/address_cluster_id trong INSERT này. |
| [Phone/network/call](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/docs/PHONE_NETWORK_CALL.md) | Cuộc gọi là mô phỏng; không phải xác minh khách thật. Không dùng kết quả này làm nhãn production. |

[Schema thiết kế](https://github.com/dunghv107/codexhackathon-sea/blob/a9dad7024ac2a9a7c4e10c2813e907f5b3f52c6f/docs/schema.sql) có checkout, order-items, product/category, identity, case/resolution, review và payment. Schema này không thay thế việc kiểm tra migrations và độ đầy đủ DB đang chạy. Các mức sẵn sàng dưới đây chỉ nói về cấu trúc và đường ghi đã đọc.

## 3. Quy ước baseline dùng chung

### Đơn vị và thời gian

- Tần suất mua đếm distinct checkout. Tiền/phản ứng Hold–Release ở cấp shop-order. Không nhân total checkout theo số shop.
- Mọi nguồn phải có event_time và available_at; chỉ dùng dữ kiện đã tồn tại trước cutoff. Checkout hiện tại được tính một lần trong velocity/exposure nếu contract quy định.
- Outcome và resolution phải thuộc đúng shop-order. Không lấy lỗi giao của một shop ghép với kết luận buyer của shop khác trong cùng checkout.
- Lịch sử trạng thái dựng as-of từ events. Current fulfillment_status không đủ tái hiện exposure quá khứ. Bổ sung event cho các chuyển trạng thái chưa có.
- Tách batch synthetic và dữ liệu thật; tách currency. Không dùng hometown hoặc khác tỉnh làm tín hiệu fraud.

### Chọn nhóm so sánh

Mỗi feature khai báo fallback riêng. Ví dụ quantity: buyer–SKU → shop–SKU → shop–category. Amount: buyer–shop–currency → shop–category–currency → shop–currency. Không dùng fallback nếu đơn vị hoặc ngữ nghĩa khác nhau.

Chỉ dùng nhóm có đủ mẫu và đủ độ mới theo policy. Mẫu thiếu, phân phối suy biến hoặc baseline quá cũ → unknown với lý do; không tự gán 0. Cold-start có thể dùng policy xác nhận COD riêng, không coi account mới là fraud.

### Cách tính thích ứng

| Kiểu feature | Phép so sánh đề xuất | Tham số cần hiệu chỉnh |
|---|---|---|
| Amount, quantity, số checkout, exposure | Vị trí x trong phân phối lịch sử tương ứng; ngưỡng Q(q) từ baseline | q, lookback, min_samples, max_age, cohort hierarchy |
| Tỷ lệ thất bại/hủy | (bad + alpha × cohort_prior)/(resolved + alpha); so với baseline tỷ lệ cùng cohort, có minimum resolved | alpha, min_resolved, ngưỡng chênh lệch hoặc percentile |
| Thay đổi hành vi | So cửa sổ gần với lịch sử của chính buyer; yêu cầu mức chênh có ý nghĩa và đủ mẫu hai cửa sổ | recent/reference windows, minimum effect, support |
| Liên kết danh tính | Loại cạnh + độ tin cậy + outcome xác minh + thời gian; không chỉ đếm cạnh | freshness, minimum independent evidence, allowed edge types |
| Mâu thuẫn bằng chứng | Điều kiện nghiệp vụ có nguồn kiểm chứng | evidence status/type, authority; không cần ngưỡng tiền |

Với velocity, baseline phải gồm cửa sổ không có đơn, hoặc dùng phân phối tại các thời điểm checkout tương đương đã được định nghĩa rõ. Với exposure, so cùng thời điểm trong vòng đời đơn; không so tổng đang mở với giá trị một đơn. Không dùng P99 khi chỉ có vài quan sát rồi coi đó là ước lượng đáng tin.

Baseline mặc định cho basket lấy đơn giao thành công đã biết tại cutoff. Baseline tỷ lệ phải gồm cả kết cục tốt và xấu; chỉ dùng đơn thành công sẽ làm sai mẫu số. Lưu selection policy vì tập đã được cho qua có thể bị selection bias.

## 4. Bộ 18 rule theo dự án

Mã mới độc lập với các R11–R26 và F01–F28 ở tài liệu trước. Đây là đặc tả đề xuất, chưa được engine hỗ trợ.

### Nhóm buyer và lịch sử trách nhiệm

| ID | Điều kiện logic, không chốt số | Dữ liệu / phản ứng |
|---|---|---|
| A01 | Tỷ lệ thất bại verified-buyer theo buyer_id cao hơn baseline cohort, đủ resolved history | checkouts → orders → events → case_resolutions. Review; không mất lịch sử khi đổi phone. |
| A02 | Tỷ lệ hủy sau confirmed do buyer tăng bất thường so với baseline, loại nguyên nhân seller/carrier | Events và kết luận trách nhiệm. Thiếu attribution → unknown; xác nhận trước đóng gói. |
| A03 | Tỷ lệ outcome bất lợi gần đây của buyer tăng so với lịch sử dài, cả hai cửa sổ đủ mẫu | Dùng cùng grain/outcome với A01; phát hiện đổi hành vi. Cùng nhóm điểm A01, không cộng đôi. |

### Nhóm tần suất và COD exposure

| ID | Điều kiện logic, không chốt số | Dữ liệu / phản ứng |
|---|---|---|
| A04 | Velocity cùng buyer/shop vượt baseline và exposure hoặc quantity cũng bất thường | Checkout timestamps, items và trạng thái as-of. Xác nhận; retry không tăng số lần mua. |
| A05 | Tổng COD đang mở của buyer/shop vượt baseline exposure phù hợp | Cộng tiền shop-order chưa được giải phóng nghĩa vụ tại cutoff. Đây là kiểm soát exposure; review, không kết luận fraud. |
| A06 | Nhiều checkout gần trùng basket/địa chỉ cùng còn mở, mức lặp khác lịch sử | Fingerprint basket chuẩn hóa theo listing/quantity; window cấu hình. Hỏi đặt trùng; đơn thay thế đã hủy không tính. |
| A07 | Buyer mở rộng đồng thời sang nhiều shop ngoài lịch sử, kèm exposure bất thường | Distinct checkout/shop. Platform aggregate; một checkout nhiều shop không tự kích hoạt. |

### Nhóm giỏ hàng

| ID | Điều kiện logic, không chốt số | Dữ liệu / phản ứng |
|---|---|---|
| A08 | Quantity của listing/SKU vượt baseline buyer hoặc shop/SKU và chưa có bối cảnh mua sỉ hợp lệ | order_items, listings, products. Review mua sỉ; dùng snapshot category nếu category có thể đổi. |
| A09 | Giá trị shop-order vượt baseline buyer và khác thường với shop/category | Giá snapshot cùng currency, baseline đủ mẫu. Tăng nhu cầu xác nhận; không kết luận từ amount đơn lẻ. |
| A10 | Buyer đột ngột mua nhóm ngành hàng mới kèm amount/quantity bất thường | Lịch sử product/category và basket. Tín hiệu bổ trợ A08/A09; mua category mới riêng lẻ không bị phạt. |

### Nhóm identity và liên kết

| ID | Điều kiện logic, không chốt số | Dữ liệu / phản ứng |
|---|---|---|
| A11 | Contact nhận hàng mới so với buyer, đồng thời có basket/exposure anomaly | phone snapshot; địa chỉ chỉ khi chuẩn hóa đủ tốt. Xác nhận contact; nhận quà/chuyển nhà là phản ví dụ. |
| A12 | Nhóm buyer liên kết bằng cạnh đáng tin có outcome xấu xác minh và hành vi đồng bộ | Typed identity links + case outcomes. Platform review nhóm; chưa bật từ phone nhập tay đơn độc. |
| A13 | Tài khoản mới xuất hiện sau xử lý abuse, nối thực thể đủ tin cậy và lặp hành vi trước | Enforcement audit + identity history; cần bổ sung. Không quy tái phạm từ shared IP/address. |

### Nhóm hậu mãi và seller

| ID | Điều kiện logic, không chốt số | Dữ liệu / phản ứng |
|---|---|---|
| A14 | Lịch sử return/refund verified-buyer bất thường theo category/cohort | cases/resolutions + delivered denominator. Cần thêm outcome_code phân biệt abuse với trách nhiệm thông thường; review hậu mãi. |
| A15 | Bằng chứng cùng nội dung bị tái sử dụng trong hồ sơ độc lập và mâu thuẫn đã được xác minh | case_evidence hiện có ref; cần content hash và trạng thái đối chiếu. Human review, không tự từ chối refund. |
| A16 | Seller/category có tỷ lệ case verified-seller tăng bất thường | Cases/resolutions, số đơn tương ứng. Cảnh báo platform về seller; không cộng điểm buyer. |

### Hai rule bảo vệ quyết định

| ID | Điều kiện logic, không chốt số | Dữ liệu / phản ứng |
|---|---|---|
| A17 | Thiếu feature bắt buộc cho loại quyết định hoặc baseline quá cũ/không đủ | coverage, unknown reasons. Trả insufficient-data; không tự Release chỉ vì score thấp, cũng không tự cộng fraud points. |
| A18 | Kết luận độc lập xác định lỗi thuộc seller/carrier hoặc case cũ được sửa kết luận | Resolution history as-of. Loại đúng sự kiện khỏi buyer-risk; không trừ điểm toàn cục để xóa tín hiệu khác. |

## 5. Mức sẵn sàng và thứ tự làm

- **Đợt đầu:** A01, A04, A06, A08, A09, A11 theo phone, A17, A18. Có cấu trúc dữ liệu nền; cần feature SQL, baseline và kiểm độ đầy đủ. Không mặc định dữ liệu thật đủ mẫu.
- **Sau khi có history/state contract:** A02, A03, A05, A07, A10. Exposure đặc biệt cần lịch sử chuyển trạng thái và ngữ nghĩa nghĩa vụ COD.
- **Sau khi mở rộng nguồn:** A12–A16. Cần liên kết tin cậy, enforcement/evidence outcome hoặc workflow platform. Không lấy callback cuộc gọi mô phỏng làm xác thực thật.

Coupon abuse, card testing và session-based ATO vẫn là backlog ngoài bộ này: schema payments hiện không chứng minh đã có attempt/token/gateway outcome; auth session không tự cung cấp đủ audit ATO; chưa xác minh module campaign. Không nhét rule chưa có dữ liệu vào engine để tăng số lượng.

## 6. Cấu hình và replay

Dùng rule_versions hiện có. Mỗi phiên bản mới chứa params, weights, dependencies, policy và tham chiếu baseline bất biến. Thay baseline hoặc policy tạo version mới; không sửa âm thầm baseline của version đã đánh giá.

Ví dụ cấu trúc mô tả, **không phải JSON có thể nạp vào schema hiện tại**:

```text
rule_id: SFD-A08
status: shadow
feature: quantity_per_listing
baseline:
  cohort_order: [buyer_listing, shop_listing, shop_category]
  reference: <immutable_baseline_version>
  quantile: <calibrated_parameter>
  lookback: <calibrated_parameter>
  min_samples: <validated_support_policy>
condition: feature > baseline.threshold AND baseline.valid
missing: unknown
evidence_group: basket_anomaly
action: confirm_quantity
```

Snapshot cần ghi observed value, threshold thực tế, cohort đã chọn, sample_count, baseline window/version, available_at, cutoff, missing_reason và source refs nội bộ. UI giải thích được “số lượng cao hơn lịch sử phù hợp” kèm số đã tính, không để LLM tự tính lại.

Baseline cho scoring lịch sử chỉ được xây từ dữ kiện sẵn có tại cutoff đó. Baseline hôm nay không được dùng replay đơn năm trước. Nếu lưu snapshot baseline riêng, giữ immutable reference; nếu lưu trong config, giữ config immutable.

Dependency nên là danh sách rule hợp lệ + chế độ any/all đã kiểm tra, không dùng eval chuỗi tùy ý. Rule chưa có handler phải bị từ chối khi activate, không âm thầm trả 0. Không cần xây một DSL tổng quát.

## 7. Điểm và hành động

Đầu tiên chạy các rule mới ở shadow, chưa cộng vào score đang điều khiển Hold/Release. Calibration so sánh theo nhóm evidence:

- Delivery history: R1/R2/R3/R6 cùng A01/A03 có thể dựa trên cùng sự kiện.
- Burst/exposure: R4/R5 cùng A04/A05/A07.
- Basket: R7/R8 cùng A08/A09/A10/A11.

Thử max/cap theo nhóm với backtest; cap và weight nằm trong version. Không mặc định chọn giá trị. A17/A18 là gate/chỉnh dữ kiện, không phải điểm fraud. A15/A16 có thời điểm và đối tượng xử lý riêng.

Coverage tính trên feature cần thiết cho quyết định, không chia số rule có dữ liệu cho tổng rule trong catalog. Thiếu device không phải lý do giữ mọi đơn nếu policy đang triển khai chỉ cần buyer/order history.

Low risk chỉ đủ cho auto-release nếu policy được hiệu chỉnh, dữ liệu bắt buộc đủ và không có seller Hold cần bảo toàn. Account mới thiếu lịch sử dùng luồng xác nhận/cold-start đã định nghĩa, không gắn nhãn gian lận.

## 8. Các phần team phải sửa cùng nhau

1. config.py: mở danh sách rule được hỗ trợ, bỏ yêu cầu đúng 10; giữ validation ID, dependency không chu trình, bounds và handler tồn tại.
2. facts.sql + risk_repository.facts: feature buyer/basket, as-of outcome, baseline parameters; giữ batch/shop isolation và distinct checkout.
3. engine.py: handler rule mới, dependency cấu hình, unknown; tách shadow khỏi score hành động.
4. rule_versions: phiên bản mới có baseline/policy reference; chỉ sửa JSON không cập nhật cấu hình active trong DB.
5. schemas/risk.py + assessment serialization: baseline metadata/coverage; Features hiện chỉ Decimal hoặc None nên không chứa được toàn bộ metadata này nếu giữ nguyên type.
6. scoring_service + cache/replay: nhận diện đúng version baseline/policy; tái hiện snapshot cũ không phụ thuộc dữ liệu tương lai.
7. apply_review_policy + backend review: gate coverage và bảo toàn seller Hold, kiểm concurrent update.
8. Explain/UI/API: hiển thị unknown, cohort, reason/action mới; kiểm tất cả chỗ giới hạn ID R1–R10 trước khi mở rộng. LLM chỉ giải thích facts có sẵn.
9. Tests/fixtures: thêm dữ liệu đủ để đối chiếu baseline và rule, không gắn nhãn synthetic thành ground truth thật.

## 9. Acceptance cho team

| Tình huống | Kết quả cần có |
|---|---|
| Buyer đổi phone, có lịch sử xấu đủ bằng chứng | A01 vẫn nhìn thấy lịch sử buyer; không nhập lịch sử của phone khác thành của buyer. |
| Checkout nhiều shop, retry cùng khóa | Đếm một checkout; tiền shop-order không nhân tổng checkout. |
| Buyer mua số lượng lớn nhưng bình thường trong cohort mua sỉ | Không cảnh báo chỉ từ số lượng tuyệt đối. |
| Cùng số lượng, hai SKU có phân phối khác nhau | Threshold và kết quả có thể khác; snapshot giải thích được. |
| Cohort ít mẫu hoặc tất cả giá trị giống nhau | Fallback hợp lệ hoặc unknown; không chia cho 0/đưa ra percentile giả tin cậy. |
| Một shop giao lỗi, shop khác có case buyer trong cùng checkout | Không ghép thành bằng chứng buyer-failure sai shop-order. |
| Kết luận được biết sau checkout | Không tác động assessment tại checkout; có thể tác động đánh giá sau theo cutoff mới. |
| Low score nhưng thiếu feature bắt buộc | A17 giữ quyết định ở luồng insufficient-data; không auto-release do điểm 0. |
| Nhiều rule cùng một bằng chứng | Group aggregation không nhân trọng số tùy tiện. |
| Thay baseline version và replay | Assessment cũ giữ nguyên; đánh giá mới ghi version mới. |
| Seller đang Hold, policy mới Low | Không ghi đè Hold thủ công ngoài quy trình hiện hành. |
| Callback cuộc gọi synthetic | Hiển thị provenance, không biến thành verified production evidence. |

Chọn tham số trên giai đoạn calibration trước, kiểm giai đoạn sau với nhãn trưởng thành. Báo precision review, incremental detection, đơn tốt bị giữ, tải review và missingness theo cohort. Dữ liệu mô phỏng kiểm logic; không dùng để tuyên bố hiệu quả ngoài thực tế.

## 10. Cơ sở nghiên cứu

[BRIGHT](https://arxiv.org/abs/2205.13084) hỗ trợ hướng quan hệ và thời gian; [Tax et al.](https://arxiv.org/abs/2107.01979) hỗ trợ gắn detection với quyết định/điều tra; [Delayed Information](https://arxiv.org/abs/1811.06109) hỗ trợ kiểm soát nhãn đến trễ. Các rule, baseline và thứ tự triển khai ở đây là thiết kế cho repo này, không phải ngưỡng được các paper chứng minh.

Chưa kiểm DB/runtime hoặc migrations toàn bộ; chưa đo hiệu quả. Tài liệu là đặc tả triển khai, chưa thay đổi engine hay cấu hình active.
