import { Link } from 'react-router'
import { currentShop, enc, useLoad } from '../../api'
import { dateTime, fulfillmentLabel, money, reviewLabel, riskLabel } from '../../format'
import { useStore } from '../../store'
import type { Dashboard as DashboardData, Page, SellerOrderSummary } from '../../types'
import { Badge, Loaded, Section } from '../../ui'

const riskColors: Record<string, string> = {
  high: '#c8372a',
  medium: '#d98a00',
  low: '#1f8a4c',
  unassessed: '#9ca3af',
}
const reviewHints: Record<string, string> = {
  pending: 'Đối chiếu dữ kiện để đưa ra quyết định',
  hold: 'Cần xác minh trước khi tiếp tục',
  released: 'Tiếp tục xử lý và giao hàng',
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
          <p className="eyebrow mb-2">KHÔNG GIAN VẬN HÀNH</p>
          <h1>Tổng quan cửa hàng</h1>
          <p className="text-gray-500 mt-2">
            Theo dõi đơn hàng, kiểm tra rủi ro và quản lý sản phẩm.
          </p>
        </div>
        <Link to="/seller/products" className="seller-secondary-link">
          Quản lý sản phẩm ↗
        </Link>
      </header>
      <section className="seller-welcome">
        <div>
          <span className="eyebrow">{shop.display_name}</span>
          <h2>Mỗi đơn hàng, một quyết định rõ ràng.</h2>
          <p>Xem những đơn cần kiểm tra và tiếp tục xử lý từ một nơi.</p>
        </div>
        <Link to="/seller/orders" className="seller-primary-link">
          Xem đơn hàng <span aria-hidden="true">→</span>
        </Link>
      </section>
      <Loaded state={dashboard}>
        {(d) => {
          const total = Object.values(d.risk_level).reduce((sum, n) => sum + n, 0)
          return (
            <>
              <div className="seller-kpis" aria-label="Tổng hợp trạng thái kiểm tra">
                <Link to="/seller/orders" className="seller-kpi">
                  <span>
                    Tổng đơn hàng <span aria-hidden="true">↗</span>
                  </span>
                  <strong>{total}</strong>
                  <small>Tất cả đơn của cửa hàng</small>
                </Link>
                {Object.entries(d.review_status).map(([key, n]) => (
                  <Link
                    key={key}
                    to={`/seller/orders?review_status=${key}`}
                    className="seller-kpi"
                    data-status={key}
                  >
                    <span>
                      <Badge map={reviewLabel} value={key} />
                      <span aria-hidden="true">↗</span>
                    </span>
                    <strong>{n}</strong>
                    <small>{reviewHints[key]}</small>
                  </Link>
                ))}
              </div>
              <div className="seller-dashboard-grid">
                <Section title="Phân bố rủi ro">
                  <p className="subtle">Theo đánh giá mới nhất · {total} đơn</p>
                  <div className="risk-distribution" aria-hidden="true">
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
                <Section title="Tiến trình giao hàng">
                  <p className="subtle">Chọn một trạng thái để xem các đơn tương ứng.</p>
                  <div className="seller-fulfillment">
                    {Object.entries(d.fulfillment_status).map(([key, n]) => (
                      <Link key={key} to={`/seller/orders?fulfillment_status=${key}`}>
                        <Badge map={fulfillmentLabel} value={key} />
                        <strong>{n}</strong>
                        <span aria-hidden="true">→</span>
                      </Link>
                    ))}
                  </div>
                </Section>
              </div>
            </>
          )
        }}
      </Loaded>
      <Section title="Đơn tạm giữ cần xử lý">
        <div className="page-head">
          <p className="subtle">Các đơn mới nhất đang Hold và chưa xử lý giao hàng.</p>
          <Link
            to="/seller/orders?review_status=hold&fulfillment_status=unfulfilled"
            className="seller-text-link"
          >
            Xem tất cả →
          </Link>
        </div>
        <Loaded state={held}>
          {(res) =>
            res.items.length ? (
              res.items.map((o) => (
                <Link
                  className="held-order"
                  key={o.order_id}
                  to={`/seller/orders/${enc(o.order_id)}`}
                >
                  <Badge map={riskLabel} value={o.risk_level ?? 'unassessed'} />
                  <code>{o.order_id}</code>
                  <span className="subtle">{dateTime(o.placed_at)}</span>
                  <strong>{money(o.order_total, o.currency)}</strong>
                  <span aria-hidden="true">→</span>
                </Link>
              ))
            ) : (
              <div className="seller-empty">
                <span aria-hidden="true">✓</span>
                <h3>Không có đơn tạm giữ cần xử lý</h3>
                <p>Các đơn đang Hold sẽ xuất hiện tại đây.</p>
              </div>
            )
          }
        </Loaded>
      </Section>
    </>
  )
}
