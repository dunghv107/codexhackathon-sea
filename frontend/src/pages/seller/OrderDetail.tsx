import { useState } from 'react'
import { CaseHistory, RiskFacts } from './RiskReview'
import { useParams } from 'react-router'
import { api, ApiError, currentShop, enc, errMsg, useLoad } from '../../api'
import {
  actorLabel,
  dateTime,
  decisionLabel,
  eventLabel,
  fulfillmentLabel,
  label,
  money,
  paymentMethodLabel,
  paymentStatusLabel,
  reviewLabel,
  riskLabel,
} from '../../format'
import { useStore } from '../../store'
import type { SellerOrderDetail } from '../../types'
import { AddressBlock, Badge, Button, ErrorText, FormDialog, Loaded, OrderItems, Section, TextArea } from '../../ui'

// Chuyển trạng thái hợp lệ theo contract; server vẫn kiểm lại (PAYMENT_NOT_READY, INVALID_TRANSITION).
const nextFulfillment: Record<string, string[]> = {
  unfulfilled: ['packing', 'canceled'],
  packing: ['shipped', 'canceled'],
  shipped: ['delivered'],
  delivered: ['returned'],
}

export default function OrderDetail() {
  const shop = useStore(currentShop)!
  const { orderId = '' } = useParams()
  const path = `/api/seller/shops/${enc(shop.shop_id)}/orders/${enc(orderId)}`
  const order = useLoad<SellerOrderDetail>(path)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // 409 = đơn hoặc đánh giá đã đổi ở nơi khác: tải lại để nút và assessment_id không bị cũ.
  const reloadOnConflict = (e: unknown) => {
    if (e instanceof ApiError && e.status === 409) order.reload()
    throw e
  }

  async function fulfill(status: string) {
    if (status === 'canceled' && !confirm('Hủy đơn này?')) return
    setError('')
    setBusy(true)
    try {
      await api(`${path}/fulfillment`, 'POST', { status }).catch(reloadOnConflict)
      order.reload()
    } catch (e) {
      setError(errMsg(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Loaded state={order}>
      {(o) => {
        const a = o.latest_assessment
        const canDecide = a !== null && o.fulfillment_status === 'unfulfilled'
        const decide = (action: 'hold' | 'release' | 'keep_hold', title: string) => (
          <FormDialog
            title={title}
            submitLabel={title}
            trigger={<Button variant={action === 'release' ? 'primary' : 'secondary'}>{title}</Button>}
            onSubmit={(f) =>
              api(`${path}/decision`, 'POST', {
                assessment_id: a!.assessment_id,
                action,
                reason: String(f.get('reason')).trim(),
              }).then(order.reload, reloadOnConflict)
            }
          >
            <p className="text-sm text-gray-500">
              Áp dụng cho đánh giá {a?.assessment_id} ({a?.rule_version}).
            </p>
            <TextArea label="Lý do" name="reason" required />
          </FormDialog>
        )

        return (
          <>
            <div className="space-y-1">
              <h1 className="text-2xl font-semibold break-all">Đơn {o.order_id}</h1>
              <p className="text-sm text-gray-500">
                Checkout {o.checkout_id} · {dateTime(o.placed_at)} · {o.item_count} sản phẩm
              </p>
              <p className="text-sm text-gray-600">{o.batch_id} · {o.source_system} · {o.is_synthetic ? 'Dữ liệu mô phỏng' : 'Dữ liệu không gắn nhãn mô phỏng'}</p>
              <div className="flex flex-wrap gap-2">
                <Badge map={reviewLabel} value={o.review_status} />
                <Badge map={fulfillmentLabel} value={o.fulfillment_status} />
              </div>
            </div>

            <Button onClick={order.reload}>Tải lại đánh giá</Button>
            <Section title="Kiểm tra rủi ro">
              {a ? (
                <div className="space-y-2 text-sm">
                  <p>
                    <span className="font-semibold">
                      {label(riskLabel, a.risk_level)} · điểm {a.score}
                    </span>{' '}
                    · rule {a.rule_version} · đánh giá lúc {dateTime(a.assessed_at)}
                    {a.input_cutoff_at && ` · dữ liệu tới ${dateTime(a.input_cutoff_at)}`}
                  </p>
                  <RiskFacts assessment={a} />
                </div>
              ) : (
                <p className="text-sm text-gray-500">Chưa có kết quả đánh giá. Cần assessment từ hệ thống chấm điểm trước khi Hold/Release; trang này không tự tạo điểm.</p>
              )}
              {canDecide && (
                <div className="flex gap-2">
                  {decide('release', 'Release · Cho xử lý')}
                  {o.review_status === 'hold' ? decide('keep_hold', 'Keep Hold · Giữ kiểm tra') : decide('hold', 'Hold · Tạm giữ')}
                </div>
              )}
              {o.decisions.length > 0 && (
                <table className="w-full text-sm">
                  <thead className="text-left text-gray-500">
                    <tr>
                      <th className="py-1 font-medium">Thời điểm</th>
                      <th className="py-1 font-medium">Bởi</th>
                      <th className="py-1 font-medium">Quyết định</th>
                      <th className="py-1 font-medium">Lý do</th>
                    </tr>
                  </thead>
                  <tbody>
                    {o.decisions.map((d) => (
                      <tr key={d.decision_id} className="border-t border-gray-100">
                        <td className="py-1 whitespace-nowrap">{dateTime(d.created_at)}</td>
                        <td className="py-1">{label(actorLabel, d.actor_type)}</td>
                        <td className="py-1">{label(decisionLabel, d.action)}</td>
                        <td className="py-1">{d.reason}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </Section>

            <Section title="Giao hàng">
              <div className="flex flex-wrap gap-2">
                {(nextFulfillment[o.fulfillment_status] ?? []).map((s) => (
                  <Button
                    key={s}
                    variant={s === 'canceled' || s === 'returned' ? 'danger' : 'primary'}
                    disabled={busy || (s === 'packing' && (o.review_status !== 'released' || (o.payment.method !== 'cod' && o.payment.status !== 'succeeded')))}
                    onClick={() => fulfill(s)}
                  >
                    {label(fulfillmentLabel, s)}
                  </Button>
                ))}
              </div>
              {o.fulfillment_status === 'unfulfilled' && o.review_status !== 'released' && (
                <p className="text-sm text-gray-500">Đơn cần được Release trước khi đóng gói.</p>
              )}
              {o.fulfillment_status === 'unfulfilled' && o.review_status === 'released' && o.payment.method !== 'cod' && o.payment.status !== 'succeeded' && (
                <p className="text-sm text-gray-500">Cần thanh toán thành công trước khi đóng gói.</p>
              )}
              {error && <ErrorText>{error}</ErrorText>}
            </Section>

            <Section title="Sản phẩm">
              <OrderItems items={o.items} currency={o.currency} />
              <p className="text-right font-semibold">Tổng đơn: {money(o.order_total, o.currency)}</p>
            </Section>
            <div className="grid gap-4 md:grid-cols-2">
              <Section title="Địa chỉ giao hàng">
                <AddressBlock address={o.address} />
                {(o.address.ward_code || o.address.house_no) && <p className="text-xs text-gray-500">Mã phường/xã: {o.address.ward_code ?? 'Chưa biết'} · Số nhà: {o.address.house_no ?? 'Chưa biết'}</p>}
                {o.address.parse_ok !== null && <p className="text-xs text-gray-500">Cấu trúc địa chỉ: {o.address.parse_ok ? 'Đọc được' : 'Chưa đọc được'}; không phải kết quả xác minh địa chỉ.</p>}
              </Section>
              <Section title="Thanh toán">
                <p className="text-sm">
                  {label(paymentMethodLabel, o.payment.method)} · {label(paymentStatusLabel, o.payment.status)}
                </p>
              </Section>
            </div>
            <CaseHistory cases={o.cases} />
            <Section title="Tiến trình">
              {o.events.length === 0 ? (
                <p className="text-sm text-gray-500">Chưa có sự kiện.</p>
              ) : (
                <ol className="space-y-1 text-sm">
                  {o.events.map((ev, i) => (
                    <li key={i}>
                      {dateTime(ev.occurred_at)} — {label(eventLabel, ev.event_type)} ({label(actorLabel, ev.actor_type)})
                      {ev.reason && `: ${ev.reason}`}
                      <span className="block text-xs text-gray-500">Thời điểm biết: {dateTime(ev.available_at)}</span>
                    </li>
                  ))}
                </ol>
              )}
            </Section>
          </>
        )
      }}
    </Loaded>
  )
}
