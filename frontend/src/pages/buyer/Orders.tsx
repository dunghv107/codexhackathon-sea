import { Link, useSearchParams } from 'react-router'
import { enc, qs, useLoad } from '../../api'
import { dateTime, displayStatusLabel, money } from '../../format'
import type { BuyerOrderSummary, Page } from '../../types'
import { Badge, Loaded, Pager } from '../../ui'

export default function Orders() {
  const [params] = useSearchParams()
  const status = params.get('status') ?? ''
  const orders = useLoad<Page<BuyerOrderSummary>>(
    `/api/buyer/orders?${qs({ page: Number(params.get('page')) || 1, limit: 20, status })}`,
  )
  return (
    <>
      <header className="page-head">
        <div>
          <p className="eyebrow">TÀI KHOẢN CỦA BẠN</p>
          <h1 className="mt-2">Đơn mua</h1>
          <p className="text-gray-500 mt-2">Tất cả đơn hàng và hành trình giao hàng của bạn.</p>
        </div>
        <Link to="/" className="hero-secondary">
          Tiếp tục mua sắm →
        </Link>
      </header>
      <nav className="order-filters" aria-label="Lọc trạng thái đơn mua">
        {[['', 'Tất cả'], ...Object.entries(displayStatusLabel)].map(([value, text]) => (
          <Link
            key={value}
            className={status === value ? 'active' : ''}
            aria-current={status === value ? 'page' : undefined}
            to={value ? `?status=${value}` : '?'}
          >
            {text}
          </Link>
        ))}
      </nav>
      <Loaded state={orders}>
        {(res) => (
          <>
            {res.items.length === 0 ? (
              <div className="empty">
                <p className="text-lg mb-3">Chưa có đơn hàng {status ? 'ở trạng thái này' : ''}</p>
                <Link to="/" className="hero-cta">
                  Khám phá sản phẩm
                </Link>
              </div>
            ) : (
              <ul className="space-y-4">
                {res.items.map((o) => (
                  <li key={o.order_id} className="buyer-order-card">
                    <header>
                      <Link to={`/shops/${enc(o.shop_id)}`} className="font-medium">
                        ◇ {o.shop_name} ↗
                      </Link>
                      <Badge map={displayStatusLabel} value={o.display_status} />
                    </header>
                    <div>
                      <code>{o.order_id}</code>
                      <p className="subtle mt-2">
                        {dateTime(o.placed_at)} · {o.item_count} sản phẩm
                      </p>
                    </div>
                    <footer>
                      <span className="text-sm">
                        Thành tiền:{' '}
                        <strong className="text-lg ml-2">{money(o.order_total, o.currency)}</strong>
                      </span>
                      <Link className="hero-cta mt-0!" to={`/buyer/orders/${enc(o.order_id)}`}>
                        Xem chi tiết →
                      </Link>
                    </footer>
                  </li>
                ))}
              </ul>
            )}
            <Pager page={res.page} totalPages={res.total_pages} />
          </>
        )}
      </Loaded>
    </>
  )
}
