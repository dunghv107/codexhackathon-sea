import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router'
import { api, currentShop, enc, errMsg, qs, useLoad } from '../../api'
import {
  dateTime,
  fulfillmentLabel,
  label,
  money,
  paymentMethodLabel,
  reviewLabel,
  riskLabel,
} from '../../format'
import { useStore } from '../../store'
import type { Page, SellerOrderSummary } from '../../types'
import { Badge, Button, ErrorText, Field, Loaded, Pager, SelectField } from '../../ui'

const filters = [
  ['fulfillment_status', 'Giao hàng', fulfillmentLabel],
  ['risk_level', 'Mức rủi ro', riskLabel],
] as const

export default function Orders() {
  const navigate = useNavigate()
  const [searchError, setSearchError] = useState('')
  const [searching, setSearching] = useState(false)
  const shop = useStore(currentShop)!
  const [params, setParams] = useSearchParams()
  const page = Number(params.get('page')) || 1
  const query = qs({
    batch_id: params.get('batch_id'),
    order_id: params.get('order_id'),
    review_status: params.get('review_status'),
    fulfillment_status: params.get('fulfillment_status'),
    risk_level: params.get('risk_level'),
    page,
    limit: 20,
  })
  const orders = useLoad<Page<SellerOrderSummary>>(
    `/api/seller/shops/${enc(shop.shop_id)}/orders?${query}`,
  )

  // Đổi bộ lọc thì quay về trang 1.
  const setFilter = (key: string, value: string) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value === 'all') next.delete(key)
      else next.set(key, value)
      next.delete('page')
      return next
    })

  return (
    <>
      <header className="page-head">
        <div>
          <p className="eyebrow mb-2">VẬN HÀNH & KIỂM TRA</p>
          <h1>Đơn hàng & review</h1>
          <p className="text-gray-500 mt-1">
            {shop.display_name} · Xem rủi ro, đối chiếu dữ kiện và đưa ra quyết định.
          </p>
        </div>
        <Button onClick={orders.reload}>↻ Làm mới</Button>
      </header>
      <section className="card seller-order-tools" aria-label="Tìm và lọc đơn hàng">
        <form
          className="seller-order-search"
          onSubmit={async (e) => {
            e.preventDefault()
            const value = String(new FormData(e.currentTarget).get('order_id') ?? '').trim()
            if (!value) return
            setSearching(true)
            setSearchError('')
            try {
              await api(`/api/seller/shops/${enc(shop.shop_id)}/orders/${enc(value)}`)
              navigate(`/seller/orders/${enc(value)}`)
            } catch (error) {
              setSearchError(errMsg(error))
            } finally {
              setSearching(false)
            }
          }}
        >
          <Field
            label="Tìm chính xác mã đơn hàng"
            name="order_id"
            defaultValue={params.get('order_id') ?? ''}
            placeholder="Nhập mã đơn hàng…"
            required
          />
          <Button type="submit" variant="primary" disabled={searching}>
            {searching ? 'Đang tìm…' : 'Tìm đơn →'}
          </Button>
          {params.get('order_id') && (
            <Button type="button" onClick={() => setFilter('order_id', 'all')}>
              Xóa lọc mã
            </Button>
          )}
        </form>
        {searchError && <ErrorText>{searchError}</ErrorText>}
        <div className="seller-order-filters">
          <SelectField
            label="Nguồn dữ liệu"
            value={params.get('batch_id') ?? 'all'}
            onValueChange={(v) => setFilter('batch_id', v)}
            options={[
              ['all', 'Tất cả'],
              ['olist_vn_sim', 'Olist VN mô phỏng'],
              ['olist_raw', 'Olist gốc'],
              ['app', 'Đơn tạo trên app'],
            ]}
          />
          {filters.map(([key, text, labels]) => (
            <SelectField
              key={key}
              label={text}
              value={params.get(key) ?? 'all'}
              onValueChange={(v) => setFilter(key, v)}
              options={[['all', 'Tất cả'], ...Object.entries(labels)]}
            />
          ))}
        </div>
      </section>
      <section className="seller-collection" aria-label="Danh sách đơn hàng">
        <nav className="seller-status-tabs" aria-label="Lọc trạng thái review">
          {[['all', 'Tất cả đơn'], ...Object.entries(reviewLabel)].map(([key, text]) => (
            <button
              key={key}
              aria-pressed={(params.get('review_status') ?? 'all') === key}
              onClick={() => setFilter('review_status', key)}
            >
              {text}
            </button>
          ))}
        </nav>
        <Loaded state={orders}>
          {(res) => (
            <>
              <div className="seller-list-heading">
                <p>
                  <strong>{res.total}</strong> đơn hàng
                </p>
                <span>Kiểm tra dữ kiện trước khi xử lý</span>
              </div>
              {res.items.length === 0 && (
                <div className="seller-empty">
                  <span aria-hidden="true">⌕</span>
                  <h3>Chưa có đơn hàng phù hợp</h3>
                  <p>Thử thay đổi bộ lọc để xem các đơn khác.</p>
                  <Button className="mt-3" onClick={() => setParams({})}>
                    Xóa bộ lọc
                  </Button>
                </div>
              )}
              {res.items.length > 0 && (
                <section
                  className="seller-table-scroll"
                  aria-label="Bảng đơn hàng"
                  tabIndex={0 /* NOSONAR: S6845 - native keyboard scrolling requires focus in browsers without focusable scroll containers. */}
                >
                  <table className="seller-table seller-orders-table">
                    <thead className="bg-gray-50 text-left text-gray-500">
                      <tr>
                        <th className="p-2 font-medium">Đơn</th>
                        <th className="p-2 font-medium">Người mua</th>
                        <th className="p-2 font-medium">Đặt lúc</th>
                        <th className="p-2 text-right font-medium">Tổng</th>
                        <th className="p-2 font-medium">Thanh toán</th>
                        <th className="p-2 font-medium">Kiểm tra</th>
                        <th className="p-2 font-medium">Giao hàng</th>
                        <th className="p-2 font-medium">Rủi ro</th>
                      </tr>
                    </thead>
                    <tbody>
                      {res.items.map((o) => (
                        <tr key={o.order_id} className="border-t border-gray-100">
                          <td className="p-2">
                            <Link
                              to={`/seller/orders/${enc(o.order_id)}`}
                              className="seller-order-id"
                            >
                              {o.order_id}
                            </Link>
                            <span className="block text-xs text-gray-500">
                              {o.item_count} sản phẩm · {o.batch_id}
                              {o.is_synthetic ? ' · Mô phỏng' : ''}
                            </span>
                          </td>
                          <td className="p-2">
                            <strong className="block font-medium">{o.buyer_display_name}</strong>
                            <span className="block text-xs text-gray-500">{o.buyer_id}</span>
                          </td>
                          <td className="p-2 whitespace-nowrap">{dateTime(o.placed_at)}</td>
                          <td className="p-2 text-right">{money(o.order_total, o.currency)}</td>
                          <td className="p-2">{label(paymentMethodLabel, o.payment_method)}</td>
                          <td className="p-2">
                            <Badge map={reviewLabel} value={o.review_status} />
                          </td>
                          <td className="p-2">
                            <Badge map={fulfillmentLabel} value={o.fulfillment_status} />
                          </td>
                          <td className="p-2 whitespace-nowrap">
                            <Badge map={riskLabel} value={o.risk_level ?? 'unassessed'} />{' '}
                            <span className="font-medium">{o.score ?? '—'}</span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </section>
              )}
              <Pager page={res.page} totalPages={res.total_pages} />
            </>
          )}
        </Loaded>
      </section>
    </>
  )
}
