import { Link, useParams } from 'react-router'
import { api, enc, useLoad } from '../../api'
import { dateTime, displayStatusLabel, eventLabel, label, money, paymentMethodLabel, paymentStatusLabel } from '../../format'
import type { BuyerOrderDetail } from '../../types'
import { AddressBlock, Badge, Button, FormDialog, Loaded, OrderItems, Section, TextArea } from '../../ui'

// Buyer chỉ thấy display_status trung tính, không có review_status/score/risk.
export default function OrderDetail() {
  const { orderId = '' } = useParams()
  const path = `/api/buyer/orders/${enc(orderId)}`
  const order = useLoad<BuyerOrderDetail>(path)

  return (
    <Loaded state={order}>
      {(o) => (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h1 className="text-2xl font-semibold">Đơn hàng</h1>
              <p className="text-sm text-gray-500">
                {o.order_id} · <Link to={`/shops/${enc(o.shop_id)}`} className="text-blue-700 hover:underline">{o.shop_name}</Link> ·{' '}
                {dateTime(o.placed_at)}
              </p>
            </div>
            <div className="flex items-center gap-3">
              <Badge map={displayStatusLabel} value={o.display_status} />
              {o.fulfillment_status === 'unfulfilled' && (
                <FormDialog
                  title="Hủy đơn hàng"
                  submitLabel="Hủy đơn"
                  trigger={<Button variant="danger">Hủy đơn</Button>}
                  onSubmit={(f) =>
                    api(`${path}/cancel`, 'POST', { reason: String(f.get('reason')).trim() }).then(order.reload)
                  }
                >
                  <TextArea label="Lý do hủy" name="reason" required />
                </FormDialog>
              )}
            </div>
          </div>

          <Section title="Sản phẩm">
            <OrderItems items={o.items} currency={o.currency} />
            <p className="text-right font-semibold">Tổng: {money(o.order_total, o.currency)}</p>
          </Section>
          <div className="grid gap-4 md:grid-cols-2">
            <Section title="Địa chỉ nhận hàng">
              <AddressBlock address={o.address} />
            </Section>
            <Section title="Thanh toán">
              <p className="text-sm">
                {label(paymentMethodLabel, o.payment.method)} · {label(paymentStatusLabel, o.payment.status)}
              </p>
            </Section>
          </div>
          <Section title="Tiến trình">
            {o.events.length === 0 ? (
              <p className="text-sm text-gray-500">Chưa có sự kiện.</p>
            ) : (
              <ol className="space-y-1 text-sm">
                {o.events.map((ev, i) => (
                  <li key={i}>
                    {dateTime(ev.occurred_at)} — {label(eventLabel, ev.event_type)}
                  </li>
                ))}
              </ol>
            )}
          </Section>
        </>
      )}
    </Loaded>
  )
}
