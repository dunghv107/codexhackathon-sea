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
