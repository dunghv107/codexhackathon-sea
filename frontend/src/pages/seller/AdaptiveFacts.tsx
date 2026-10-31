import { dateTime } from '../../format'
import { record, display } from './risk-evidence'

const adaptiveLabels: Record<string, string> = {
  fired: 'Có tín hiệu',
  clear: 'Không kích hoạt',
  unknown: 'Chưa đủ dữ liệu',
  buyer_listing: 'Lịch sử người mua với mặt hàng',
  shop_listing: 'Mặt hàng tại cửa hàng',
  buyer_shop_currency: 'Người mua tại cửa hàng, cùng tiền tệ',
  shop_currency: 'Cửa hàng, cùng tiền tệ',
  shop_currency_buyer_rates: 'Lịch sử trách nhiệm của người mua tại cửa hàng',
  buyer_shop_checkout_anchors: 'Các lần đặt của người mua tại cửa hàng',
  buyer_phone_history: 'Lịch sử liên hệ của người mua',
  buyer_order_responsibility: 'Trách nhiệm của từng đơn',
  buyer_failure_rate: 'Tỷ lệ lỗi giao của người mua',
  quantity_per_listing: 'Số lượng mặt hàng',
  buyer_order_amount: 'Giá trị đơn theo lịch sử người mua',
  shop_order_amount: 'Giá trị đơn theo cửa hàng',
  review_delivery_history: 'Xem lại lịch sử giao hàng',
  confirm_quantity: 'Xác nhận số lượng',
  confirm_amount: 'Xác nhận giá trị đơn',
  confirm_order: 'Xác nhận nhu cầu đặt hàng',
  confirm_duplicate_order: 'Hỏi khách có đặt trùng đơn',
  confirm_contact: 'Xác nhận thông tin liên hệ',
  insufficient_data: 'Bổ sung dữ liệu',
  explain_attribution: 'Loại lỗi của người bán/vận chuyển khỏi tín hiệu mới',
  buyer_cancellation_rate: 'Lịch sử hủy đơn sau xác nhận',
  buyer_outcome_change: 'Thay đổi kết quả giao hàng',
  cod_exposure: 'Nghĩa vụ COD còn mở',
  new_categories: 'Lịch sử ngành hàng',
  after_sale_rate: 'Lịch sử hoàn trả đã xác minh',
  evidence_reuse: 'Đối chiếu nội dung bằng chứng',
  shop_currency_resolved_buyers: 'Người mua có kết quả rõ trách nhiệm, cùng cửa hàng và tiền tệ',
  shop_category_currency_resolved_buyers: 'Người mua cùng ngành hàng, cửa hàng và tiền tệ',
  buyer_recent_vs_reference: 'Người mua: giai đoạn gần đây so với lịch sử trước đó',
  shop_currency_recent_vs_reference: 'Cửa hàng: giai đoạn gần đây so với lịch sử trước đó',
  buyer_shop_currency_open_cod: 'COD còn mở tại các lần mua trước, cùng người mua và cửa hàng',
  non_cod_order: 'Đơn không thanh toán COD',
  buyer_checkout_shop_breadth: 'Số cửa hàng trong các cửa sổ mua trước đây',
  multiple_checkouts_required: 'Ít nhất hai lần mua riêng biệt',
  buyer_delivered_category_snapshots: 'Danh mục đã lưu lúc mua của các đơn giao thành công',
  verified_independent_identity_links: 'Liên kết có các nguồn xác minh độc lập',
  verified_linked_checkout_window: 'Hoạt động đồng thời của các tài khoản đã xác minh liên kết',
  active_enforcement_and_matching_verified_outcomes: 'Quyết định xử lý còn hiệu lực và hành vi tái diễn đã xác minh',
  content_hash_and_verified_contradiction: 'Nội dung trùng và mâu thuẫn đã được đối chiếu',
  confirm_cancellation_history: 'Xác minh nguyên nhân hủy đơn trước đây',
  review_behavior_change: 'Xem lại thay đổi hành vi mua hàng',
  confirm_cod_exposure: 'Xác nhận các đơn COD đang mở',
  review_shop_expansion: 'Kiểm tra các lần mua tại nhiều cửa hàng',
  confirm_new_category: 'Xác nhận nhu cầu với ngành hàng mới',
  platform_review_group: 'Quản trị viên kiểm tra nhóm tài khoản',
  platform_review_recurrence: 'Quản trị viên đối chiếu lịch sử xử lý vi phạm',
  review_after_sale: 'Xem hồ sơ hoàn trả đã có kết luận',
  review_reused_evidence: 'Kiểm tra hồ sơ có bằng chứng được dùng lại',
  platform_review_seller: 'Quản trị viên xem xét vấn đề của người bán',
}

export function AdaptiveFacts({ value }: { value: unknown }) {
  const adaptive = record(value)
  if (adaptive.mode !== 'shadow' || !Array.isArray(adaptive.signals)) return null
  const coverage = record(adaptive.coverage)
  return (
    <section
      className="space-y-3 rounded-lg border border-amber-200 bg-amber-50/40 p-4"
      aria-label="Rule thử nghiệm"
    >
      <h3 className="font-semibold">Rule thử nghiệm (shadow)</h3>
      <p className="text-sm text-amber-900">
        Cấu hình demo chưa hiệu chỉnh. Các tín hiệu này không cộng điểm hoặc thay đổi Hold/Release.
        A17 chỉ cảnh báo thiếu dữ liệu.
      </p>
      <p className="subtle">
        Baseline: {display(adaptive.baseline_version)} · Dữ liệu trước{' '}
        {dateTime(typeof adaptive.input_cutoff_at === 'string' ? adaptive.input_cutoff_at : null)}
      </p>
      {coverage.status === 'insufficient_data' && (
        <div className="text-sm">
          <p className="font-medium">Chưa đủ dữ liệu cho các tín hiệu thử nghiệm</p>
          <ul className="mt-1 list-disc space-y-1 pl-5">
            {Object.entries(record(coverage.missing_features)).map(([name, reason]) => (
              <li key={name}>
                {adaptiveLabels[name] ?? name}: {display(reason)}
              </li>
            ))}
          </ul>
        </div>
      )}
      {adaptive.signals.map((value, index) => {
        const signal = record(value)
        const status = typeof signal.status === 'string' ? signal.status : 'unknown'
        const evidence = Array.isArray(signal.evidence) ? signal.evidence : []
        return (
          <details key={index} className="rounded border border-gray-200 bg-white p-3 text-sm">
            <summary className="cursor-pointer font-medium">
              {display(signal.rule_id)} · {display(signal.description)} —{' '}
              {adaptiveLabels[status] ?? 'Chưa đủ dữ liệu'}
            </summary>
            {status === 'fired' && (
              <p className="mt-2">
                Gợi ý: {adaptiveLabels[String(signal.action)] ?? display(signal.action)}.
              </p>
            )}
            {signal.missing_reason != null && (
              <p className="mt-2 text-amber-900">{display(signal.missing_reason)}</p>
            )}
            {evidence.map((value, index) => {
              const fact = record(value)
              const baseline = record(fact.baseline)
              return (
                <div key={index} className="mt-3 space-y-1 border-t border-gray-100 pt-2">
                  <p className="font-medium">{display(fact.label)}</p>
                  <p>
                    Quan sát: {display(fact.observed)} · Ngưỡng so sánh: {display(fact.comparison_threshold)}
                  </p>
                  {fact.observed_sample_count != null && <p>Số mẫu quan sát: {display(fact.observed_sample_count)}</p>}
                  <p>
                    Nhóm so sánh: {adaptiveLabels[String(baseline.cohort)] ?? display(baseline.cohort)}
                    {' '}· Số mẫu: {display(baseline.sample_count)}
                  </p>
                  {fact.missing_reason != null && (
                    <p className="text-amber-900">{display(fact.missing_reason)}</p>
                  )}
                </div>
              )
            })}
          </details>
        )
      })}
    </section>
  )
}
