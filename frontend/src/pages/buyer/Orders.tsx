import { Link, useSearchParams } from 'react-router'
import { enc, qs, useLoad } from '../../api'
import { dateTime, displayStatusLabel, money } from '../../format'
import type { BuyerOrderSummary, Page } from '../../types'
import { Badge, Loaded, Pager } from '../../ui'

export default function Orders() {
  const [params] = useSearchParams()
  const page = Number(params.get('page')) || 1
  const orders = useLoad<Page<BuyerOrderSummary>>(`/api/buyer/orders?${qs({ page, limit: 20 })}`)

  return (
    <>
      <h1 className="text-2xl font-semibold">Đơn hàng của tôi</h1>
      <Loaded state={orders}>
        {(res) => (
          <>
            {res.items.length === 0 && <p className="text-sm text-gray-500">Chưa có đơn hàng.</p>}
            <ul className="space-y-3">
              {res.items.map((o) => (
                <li key={o.order_id}>
                  <Link
                    to={`/buyer/orders/${enc(o.order_id)}`}
                    className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-gray-200 bg-white p-4 hover:border-gray-400 focus-visible:outline-2 focus-visible:outline-blue-500"
                  >
                    <span>
                      <span className="font-medium">{o.shop_name}</span>
                      <span className="block text-xs text-gray-500">
                        {dateTime(o.placed_at)} · {o.item_count} sản phẩm
                      </span>
                    </span>
                    <span className="flex items-center gap-3">
                      <Badge map={displayStatusLabel} value={o.display_status} />
                      <span className="font-semibold">{money(o.order_total, o.currency)}</span>
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
            <Pager page={res.page} totalPages={res.total_pages} />
          </>
        )}
      </Loaded>
    </>
  )
}
