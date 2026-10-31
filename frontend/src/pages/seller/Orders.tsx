import { Link, useSearchParams } from 'react-router'
import { currentShop, enc, qs, useLoad } from '../../api'
import { dateTime, fulfillmentLabel, label, money, paymentMethodLabel, reviewLabel, riskLabel } from '../../format'
import { useStore } from '../../store'
import type { Page, SellerOrderSummary } from '../../types'
import { Badge, Loaded, Pager, SelectField } from '../../ui'

const filters = [
  ['review_status', 'Kiểm tra', reviewLabel],
  ['fulfillment_status', 'Giao hàng', fulfillmentLabel],
  ['risk_level', 'Mức rủi ro', riskLabel],
] as const

export default function Orders() {
  const shop = useStore(currentShop)!
  const [params, setParams] = useSearchParams()
  const page = Number(params.get('page')) || 1
  const query = qs({
    review_status: params.get('review_status'),
    fulfillment_status: params.get('fulfillment_status'),
    risk_level: params.get('risk_level'),
    page,
    limit: 20,
  })
  const orders = useLoad<Page<SellerOrderSummary>>(`/api/seller/shops/${enc(shop.shop_id)}/orders?${query}`)

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
      <h1 className="text-2xl font-semibold">Đơn hàng · {shop.display_name}</h1>
      <div className="grid gap-3 sm:grid-cols-3">
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
      <Loaded state={orders}>
        {(res) => (
          <>
            <p className="text-sm text-gray-500">{res.total} đơn</p>
            <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 text-left text-gray-500">
                  <tr>
                    <th className="p-2 font-medium">Đơn</th>
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
                        <Link to={`/seller/orders/${enc(o.order_id)}`} className="text-blue-700 hover:underline">
                          {o.order_id}
                        </Link>
                        <span className="block text-xs text-gray-500">{o.item_count} sản phẩm</span>
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
                        {o.risk_level ? `${label(riskLabel, o.risk_level)} (${o.score})` : label(riskLabel, 'unassessed')}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pager page={res.page} totalPages={res.total_pages} />
          </>
        )}
      </Loaded>
    </>
  )
}
