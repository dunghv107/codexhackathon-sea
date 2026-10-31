import { record, display, evidenceSummary } from './risk-evidence'
import type { Assessment } from '../../types'

const titles: Record<string, string> = {
  R1: 'Lịch sử giao hàng theo số điện thoại', R2: 'Lịch sử giao hàng tại cụm địa chỉ',
  R3: 'Giao thất bại lặp lại', R4: 'Đặt hàng tại nhiều cửa hàng', R5: 'Đặt nhiều lần trong thời gian ngắn',
  R6: 'Nhiều người mua dùng chung cụm địa chỉ', R7: 'Giá trị COD bất thường',
  R8: 'Tài khoản mới kèm tín hiệu khác', R9: 'Xác thực địa chỉ', R10: 'Hủy đơn sau khi xác nhận',
}
const factLabels: Record<string, string> = {
  resolved: 'Lần mua đã có kết quả giao', failed: 'Thất bại đã xác minh do người mua',
  smoothed_rate: 'Tỷ lệ sau làm trơn', failure_rate: 'Tỷ lệ thất bại', count: 'Số lần ghi nhận',
  checkouts: 'Lần đặt hàng riêng biệt', shops: 'Cửa hàng liên quan', buyers: 'Người mua khác nhau',
  amount: 'Giá trị đơn', median: 'Trung vị giá trị đơn cùng shop', history_orders: 'Đơn lịch sử cùng shop',
  ratio: 'Tỷ lệ so với trung vị', age_days: 'Tuổi tài khoản (ngày)', cod: 'Thanh toán COD',
}
const paramLabels: Record<string, string> = {
  minimum: 'Số lần tối thiểu', minimum_shops: 'Số cửa hàng tối thiểu',
  minimum_distinct_checkouts: 'Số lần mua tối thiểu', minimum_checkouts: 'Số lần mua tối thiểu',
  minimum_buyers: 'Số người mua tối thiểu', minimum_resolved_checkouts: 'Số lần mua có kết quả tối thiểu',
  minimum_history_orders: 'Số đơn lịch sử tối thiểu', ratio_strictly_above: 'Tỷ lệ phải lớn hơn',
  buyer_failure_rate_strictly_above: 'Tỷ lệ thất bại phải lớn hơn', age_days_strictly_below: 'Tuổi tài khoản nhỏ hơn (ngày)',
  requires: 'Đồng thời có tín hiệu',
}
export function RiskFacts({ assessment: a }: { assessment: Assessment }) {
  const facts = record(a.feature_snapshot)
  const unknown = record(facts.unknown_reasons)
  const rules = a.rule_config?.rules ?? []
  const reasons = (Array.isArray(a.reasons) ? a.reasons : []).map(record).filter(r => typeof r.rule_id === 'string')
  const fired = new Set(reasons.map(r => r.rule_id))
  return <div className="risk-evidence">
    <p className="subtle">Các con số dưới đây lấy từ kết quả KBS đã lưu cho đơn này. Điểm hỗ trợ review, không phải xác suất hay kết luận gian lận.</p>
    {a.rule_config?.score_cap != null && <p className="subtle">KBS cộng điểm của các tín hiệu kích hoạt, giới hạn tổng ở {a.rule_config.score_cap} điểm. Mỗi rule chỉ đóng góp một lần.</p>}
    {reasons.length === 0 && <p className="evidence-empty">Không có rule đóng góp điểm trong assessment này. Thiếu lịch sử không đồng nghĩa với an toàn.</p>}
    {reasons.map((r, i) => {
      const id = String(r.rule_id), rule = rules.find(rule => rule.rule_id === id)
      const observed = record(facts[id]), params = rule?.params ?? {}
      const summary = evidenceSummary(id, observed, params)
      const tiers = Array.isArray(params.tiers) ? params.tiers.filter((tier): tier is [number, number] => Array.isArray(tier) && tier.length === 2 && tier.every(n => typeof n === 'number')) : []
      return <article className="evidence-card" key={`${id}-${i}`}>
        <header><div><span className="eyebrow">{id} · Tín hiệu đã kích hoạt</span><h3>{titles[id] ?? rule?.description ?? id}</h3></div>
          <div className="evidence-points"><strong>{display(r.contribution ?? r.points)}</strong><span>điểm cộng</span></div></header>
        <p className="evidence-summary">{summary ?? rule?.description ?? display(r.description ?? r.message)}</p>
        {!summary && <p className="subtle">{params.window_days != null ? `Xét lịch sử trong ${display(params.window_days)} ngày.` : params.window_hours != null ? `Xét trong ${display(params.window_hours)} giờ.` : 'Xét dữ kiện tại thời điểm đặt hàng.'}</p>}
        <dl className="evidence-numbers">{Object.entries(observed).filter(([key]) => key in factLabels).map(([key,value]) =>
          <div key={key}><dt>{factLabels[key]}</dt><dd>{key === 'cod' && value != null ? (Number(value) === 1 ? 'Có' : 'Không') : display(value)}</dd></div>)}</dl>
        {tiers.length > 0 && <div className="evidence-calculation">
          <p><strong>Cách quy đổi thành điểm</strong></p>
          {observed.smoothed_rate != null && params.alpha != null && params.prior != null && observed.failed != null && observed.resolved != null && <p>
            ({display(observed.failed)} + {display(params.alpha)} × {display(params.prior)}) ÷ ({display(observed.resolved)} + {display(params.alpha)}) = <strong>{display(observed.smoothed_rate)}</strong> theo KBS.
          </p>}
          <p>{tiers.map(([threshold,points]) => `Từ ${display(threshold)} → ${points} điểm`).join(' · ')}. Chọn một bậc cao nhất đạt điều kiện.</p>
        </div>}
        {Object.entries(params).some(([key]) => key in paramLabels) && <div className="evidence-calculation"><strong>Điều kiện trong phiên bản rule này</strong>
          <ul>{Object.entries(params).filter(([key]) => key in paramLabels).map(([key,value]) => <li key={key}>{paramLabels[key]}: {display(value)}</li>)}</ul></div>}
      </article>
    })}
    <details className="evidence-other"><summary>Các tín hiệu còn lại · {rules.filter(rule => !fired.has(rule.rule_id)).length}</summary>
      <p className="subtle">Không cộng điểm có thể do chưa đạt điều kiện, thiếu dữ liệu hoặc rule chưa được bật.</p>
      {rules.filter(rule => !fired.has(rule.rule_id)).map(rule => {
        const absent = rule.rule_id in unknown || Object.keys(record(facts[rule.rule_id])).length === 0
        return <div className="evidence-other-row" key={rule.rule_id}><span>{rule.rule_id} · {titles[rule.rule_id] ?? rule.description}</span>
          <strong>{!rule.enabled ? 'Chưa áp dụng' : absent ? 'Chưa đủ dữ liệu' : 'Không kích hoạt'}</strong></div>
      })}
      <p className="subtle">Thiếu dữ liệu không được tính là bằng chứng an toàn.</p>
    </details>
    {a.rule_config?.status === 'draft_not_calibrated' && <p className="subtle">Bộ rule demo chưa hiệu chỉnh bằng dữ liệu thực tế.</p>}
    <details className="evidence-other"><summary>Dữ kiện kỹ thuật tại thời điểm chấm</summary><pre className="evidence-json">{JSON.stringify(a.feature_snapshot, null, 2) ?? 'Chưa biết'}</pre></details>
  </div>
}
