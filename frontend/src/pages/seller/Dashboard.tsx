import { Link } from 'react-router'
import { currentShop, enc, useLoad } from '../../api'
import { dateTime, fulfillmentLabel, money, reviewLabel, riskLabel } from '../../format'
import { useStore } from '../../store'
import type { Dashboard as DashboardData, Page, SellerOrderSummary } from '../../types'
import { Badge, Loaded, Section } from '../../ui'

function Counts({
  filter,
  counts,
  labels,
}: {
  filter: string
  counts: Record<string, number>
  labels: Record<string, string>
}) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
      {Object.entries(counts).map(([key, n]) => (
        <Link
          key={key}
          to={`/seller/orders?${filter}=${key}`}
          className={`metric ${filter === 'fulfillment_status' ? 'metric-compact' : ''}`}
        >
          <Badge map={labels} value={key} />
          <strong>{n}</strong>
          {filter !== 'fulfillment_status' && (
            <span className="subtle">
              {key === 'pending'
                ? 'Chưa có assessment hoặc chưa chốt'
                : key === 'hold'
                  ? 'Đang tạm giữ, chờ kiểm tra'
                  : 'Được phép xử lý giao hàng'}
            </span>
          )}
        </Link>
      ))}
    </div>
  )
}
const riskColors: Record<string, string> = {
  high: '#c8372a',
  medium: '#d98a00',
  low: '#1f8a4c',
  unassessed: '#a59a89',
}
export default function Dashboard() {
  const shop = useStore(currentShop)!
  const base = `/api/seller/shops/${enc(shop.shop_id)}`
  const dashboard = useLoad<DashboardData>(`${base}/dashboard`)
  const held = useLoad<Page<SellerOrderSummary>>(
    `${base}/orders?review_status=hold&fulfillment_status=unfulfilled&limit=5`,
  )
  return (
    <>
      <header className="page-head">
        <div>
          <h1>Tổng quan</h1>
          <p className="text-gray-500 mt-1">{shop.display_name} · Theo dõi đơn hàng và hoạt động review</p>
        </div>
        <Link to="/seller/orders" className="ui-button rounded-full bg-gray-900 text-white px-5 py-3">
          Mở hàng đợi review →
        </Link>
      </header>
      <Loaded state={dashboard}>
        {(d) => {
          const total = Object.values(d.risk_level).reduce((sum, n) => sum + n, 0)
          return (
            <>
              <Section title="Trạng thái review">
                <Counts filter="review_status" counts={d.review_status} labels={reviewLabel} />
              </Section>
              <div className="grid gap-5 xl:grid-cols-2">
                <Section title="Mức rủi ro">
                  <p className="subtle">Theo assessment mới nhất · {total} đơn</p>
                  <div className="flex h-2.5 overflow-hidden rounded-full bg-gray-200" aria-hidden>
                    {Object.entries(d.risk_level).map(([key, n]) => (
                      <span
                        key={key}
                        style={{
                          width: `${total ? (n / total) * 100 : 0}%`,
                          background: riskColors[key],
                        }}
                      />
                    ))}
                  </div>
                  <div>
                    {Object.entries(d.risk_level).map(([key, n]) => (
                      <Link className="risk-row" key={key} to={`/seller/orders?risk_level=${key}`}>
                        <Badge map={riskLabel} value={key} />
                        <span className="risk-track">
                          <span
                            style={{
                              width: `${total ? (n / total) * 100 : 0}%`,
                              background: riskColors[key],
                            }}
                          />
                        </span>
                        <strong className="text-right font-medium">{n}</strong>
                        <span className="subtle text-right">
                          {total ? Math.round((n / total) * 100) : 0}%
                        </span>
                      </Link>
                    ))}
                  </div>
                  <p className="subtle">Điểm rủi ro hỗ trợ review, không phải xác suất gian lận.</p>
                </Section>
                <Section title="Trạng thái giao hàng">
                  <Counts
                    filter="fulfillment_status"
                    counts={d.fulfillment_status}
                    labels={fulfillmentLabel}
                  />
                </Section>
              </div>
            </>
          )
        }}
      </Loaded>
      <Section title="Đơn đang Hold · mới nhất">
        <div className="page-head">
          <p className="subtle">Chưa xử lý giao hàng, cần xem xét trước khi tiếp tục.</p>
          <Link
            to="/seller/orders?review_status=hold&fulfillment_status=unfulfilled"
            className="text-blue-700 text-sm"
          >
            Xem tất cả →
          </Link>
        </div>
        <Loaded state={held}>
          {(res) =>
            res.items.length ? (
              res.items.map((o) => (
                <Link className="held-order" key={o.order_id} to={`/seller/orders/${enc(o.order_id)}`}>
                  <Badge map={riskLabel} value={o.risk_level ?? 'unassessed'} />
                  <code>{o.order_id}</code>
                  <span className="subtle">{dateTime(o.placed_at)}</span>
                  <span>{money(o.order_total, o.currency)}</span>
                </Link>
              ))
            ) : (
              <p className="empty">Không có đơn đang Hold cần xử lý.</p>
            )
          }
        </Loaded>
      </Section>
    </>
  )
}
