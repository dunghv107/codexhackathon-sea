import { useState } from 'react'
import { AlertDialog, Tabs } from 'radix-ui'
import { Link, useParams } from 'react-router'
import { CaseHistory, RiskFacts } from './RiskReview'
import { api, ApiError, currentShop, enc, errMsg, profiles, useLoad } from '../../api'
import {
  topReasons,
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
import {
  AddressBlock,
  Badge,
  Button,
  ErrorText,
  FormDialog,
  Loaded,
  OrderItems,
  Section,
  TextArea,
} from '../../ui'

const nextFulfillment: Record<string, string[]> = {
  unfulfilled: ['packing', 'canceled'],
  packing: ['shipped', 'canceled'],
  shipped: ['delivered'],
  delivered: ['returned'],
}

export default function OrderDetail() {
  const shop = useStore(currentShop)!
  const suspended = useStore(profiles)?.seller?.status === 'suspended'
  const { orderId = '' } = useParams()
  const path = `/api/seller/shops/${enc(shop.shop_id)}/orders/${enc(orderId)}`
  const order = useLoad<SellerOrderDetail>(path)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [cancelOpen, setCancelOpen] = useState(false)
  const [stale, setStale] = useState(false)
  const [explanation, setExplanation] = useState<{ assessment_id: string; sentences: string[] } | null>(null)

  async function assess(force: boolean) {
    setBusy(true)
    setError('')
    setExplanation(null)
    try {
      await api(`${path}/score?force=${force}`, 'POST')
      order.reload()
    } catch (e) {
      setError(errMsg(e))
    } finally {
      setBusy(false)
    }
  }

  async function explain() {
    setBusy(true)
    setError('')
    try {
      setExplanation(await api(`${path}/explain`, 'POST'))
    } catch (e) {
      setError(errMsg(e))
    } finally {
      setBusy(false)
    }
  }

  const reloadOnConflict = (e: unknown) => {
    if (e instanceof ApiError && e.status === 409) {
      setStale(true)
      order.reload()
    }
    throw e
  }
  async function fulfill(status: string) {
    setError('')
    setBusy(true)
    try {
      await api(`${path}/fulfillment`, 'POST', { status }).catch(reloadOnConflict)
      order.reload()
      setCancelOpen(false)
      setNotice('Đã cập nhật trạng thái giao hàng.')
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
        const canDecide =
          a !== null && o.fulfillment_status === 'unfulfilled' && !suspended && !busy && !order.loading
        const decide = (action: 'hold' | 'release' | 'keep_hold', title: string) => (
          <FormDialog
            title={title}
            submitDisabled={busy || order.loading || !!order.error}
            submitLabel="Xác nhận quyết định"
            trigger={
              <Button disabled={!canDecide} variant={action === 'release' ? 'primary' : 'secondary'}>
                {title}
              </Button>
            }
            onSubmit={async (f) => {
              if (!a || !canDecide)
                throw new Error('Đơn không còn đủ điều kiện review. Vui lòng kiểm tra lại.')
              const reason = String(f.get('reason') ?? '').trim()
              if (!reason) throw new Error('Vui lòng nhập lý do quyết định.')
              await api(`${path}/decision`, 'POST', {
                assessment_id: a.assessment_id,
                action,
                reason,
              }).catch(reloadOnConflict)
              order.reload()
              setStale(false)
              setNotice('Đã lưu quyết định review.')
            }}
          >
            <p className="text-sm text-gray-500">
              Đơn <code className="break-all">{o.order_id}</code>
            </p>
            {stale && (
              <p role="alert" className="rounded-lg bg-amber-50 p-3 text-sm text-amber-800">
                Đơn hoặc assessment đã thay đổi. Đang tải dữ liệu mới; hãy kiểm tra assessment bên dưới trước
                khi xác nhận lại. Lý do của bạn vẫn được giữ.
              </p>
            )}
            <p className="text-sm">
              Assessment <code className="break-all">{a?.assessment_id}</code> · {a?.rule_version} ·{' '}
              {a?.score}/100
            </p>
            {order.error && (
              <div className="space-y-2">
                <ErrorText>{order.error}</ErrorText>
                <Button type="button" disabled={order.loading} onClick={order.reload}>
                  {order.loading ? 'Đang tải…' : 'Tải lại assessment'}
                </Button>
              </div>
            )}
            <TextArea
              label="Lý do quyết định"
              name="reason"
              required
              maxLength={2000}
              placeholder="Ghi rõ dữ kiện đã kiểm tra và lý do…"
            />
          </FormDialog>
        )
        return (
          <>
            <Link to="/seller/orders" className="text-blue-700 text-sm">
              ← Hàng đợi review
            </Link>
            <header className="page-head">
              <div className="min-w-0">
                <p className="eyebrow mb-2">
                  Chi tiết đơn hàng · {o.batch_id}
                  {o.is_synthetic ? ' · Mô phỏng' : ''}
                </p>
                <h1 className="break-all">{o.order_id}</h1>
                <p className="subtle mt-2 break-all">
                  Checkout {o.checkout_id} · {dateTime(o.placed_at)} · {o.item_count} sản phẩm
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(o.order_id)
                      setNotice('Đã sao chép mã đơn.')
                    } catch {
                      setError('Không thể sao chép. Hãy chọn mã đơn để sao chép thủ công.')
                    }
                  }}
                >
                  Sao chép mã
                </Button>
                <Button onClick={order.reload}>↻ Làm mới</Button>
              </div>
            </header>
            <div className="flex flex-wrap gap-2">
              <Badge map={reviewLabel} value={o.review_status} />
              <Badge map={fulfillmentLabel} value={o.fulfillment_status} />
              <Badge map={paymentMethodLabel} value={o.payment.method} />
            </div>
            {error && <ErrorText>{error}</ErrorText>}
            <div className="summary-grid">
              <Section title="Điểm rủi ro">
                <div className="flex items-end gap-3">
                  <span className="score">
                    {a?.score ?? '—'}
                    <span className="text-lg text-gray-500"> /100</span>
                  </span>
                  <Badge map={riskLabel} value={a?.risk_level ?? 'unassessed'} />
                </div>
                <div className="risk-track">
                  <span
                    style={{
                      width: `${a ? Math.min(100, Math.max(0, a.score)) : 0}%`,
                    }}
                  />
                </div>
                <p className="subtle">
                  {a ? `${a.rule_version} · ${dateTime(a.assessed_at)}` : 'Chưa có kết quả đánh giá.'}
                </p>
                <p className="subtle">Cutoff: {dateTime(a?.input_cutoff_at ?? null)}</p>
              </Section>
              <Section title="Tín hiệu đóng góp chính">
                {a && topReasons(a).length ? (
                  topReasons(a).map((r) => (
                    <div className="flex gap-3 items-start border-b border-gray-100 pb-3" key={r.rule_id}>
                      <code className="badge">{r.rule_id}</code>
                      <p className="flex-1 text-sm">
                        {a.rule_config?.rules?.find((rule) => rule.rule_id === r.rule_id)?.description ??
                          'Xem dữ kiện trong tab Rủi ro'}
                      </p>
                      <strong className="text-red-700">+{r.contribution}</strong>
                    </div>
                  ))
                ) : (
                  <p className="subtle">Chưa có tín hiệu đóng góp.</p>
                )}
                <p className="subtle">Điểm hỗ trợ review, không phải kết luận gian lận.</p>
              </Section>
              <Section title="Quyết định review">
                <Badge map={reviewLabel} value={o.review_status} />
                <p className="text-sm text-gray-500">Đối chiếu dữ kiện trước khi cho phép xử lý đơn hàng.</p>
                <div className="flex flex-wrap gap-2">
                  {decide('release', 'Release · Cho xử lý')}
                  {o.review_status === 'hold'
                    ? decide('keep_hold', 'Keep Hold')
                    : decide('hold', 'Hold · Tạm giữ')}
                </div>
                {!canDecide && (
                  <p className="subtle">
                    {suspended
                      ? 'Tài khoản tạm khóa, chỉ được xem.'
                      : !a
                        ? 'Cần assessment trước khi quyết định.'
                        : 'Đơn đã chuyển sang xử lý giao hàng.'}
                  </p>
                )}
              </Section>
            </div>
            <Tabs.Root defaultValue="risk">
              <Tabs.List className="review-tabs" aria-label="Chi tiết đơn">
                {[
                  ['risk', 'Rủi ro'],
                  ['order', 'Đơn hàng'],
                  ['shipping', 'Giao hàng'],
                  ['history', 'Lịch sử'],
                  ['cases', `Tranh chấp (${o.cases.length})`],
                ].map(([value, title]) => (
                  <Tabs.Trigger key={value} value={value}>
                    {title}
                  </Tabs.Trigger>
                ))}
              </Tabs.List>
              <Tabs.Content value="risk" className="review-panel">
                {o.batch_id === 'olist_vn_sim' && (
                  <div className="flex flex-wrap gap-2">
                    <Button disabled={busy || suspended || order.loading} onClick={() => assess(a !== null)}>
                      {a ? 'Chấm lại' : 'Chấm điểm'}
                    </Button>
                    {a && (
                      <Button disabled={busy || suspended || order.loading} onClick={explain}>
                        Giải thích đánh giá
                      </Button>
                    )}
                  </div>
                )}
                {a && explanation?.assessment_id === a.assessment_id && (
                  <Section title="Giải thích từ KBS">
                    <ul className="list-disc pl-5 space-y-2 text-sm">
                      {explanation.sentences.map((sentence, index) => (
                        <li key={index}>{sentence}</li>
                      ))}
                    </ul>
                  </Section>
                )}
                <Section title="Giải thích đánh giá">
                  {a ? (
                    <RiskFacts assessment={a} />
                  ) : (
                    <p className="empty">Chưa có assessment cho đơn này.</p>
                  )}
                </Section>
              </Tabs.Content>
              <Tabs.Content value="order" className="review-panel">
                <Section title="Sản phẩm">
                  <div className="overflow-x-auto">
                    <OrderItems items={o.items} currency={o.currency} />
                  </div>
                  <p className="text-right font-semibold">Tổng đơn: {money(o.order_total, o.currency)}</p>
                </Section>
                <Section title="Thanh toán">
                  <p>
                    {label(paymentMethodLabel, o.payment.method)} ·{' '}
                    <Badge map={paymentStatusLabel} value={o.payment.status} />
                  </p>
                </Section>
              </Tabs.Content>
              <Tabs.Content value="shipping" className="review-panel">
                <Section title="Giao hàng">
                  <Badge map={fulfillmentLabel} value={o.fulfillment_status} />
                  <div className="flex flex-wrap gap-2">
                    {(nextFulfillment[o.fulfillment_status] ?? []).map((s) =>
                      s === 'canceled' ? (
                        <AlertDialog.Root
                          key={s}
                          open={cancelOpen}
                          onOpenChange={(v) => {
                            if (!busy) setCancelOpen(v)
                          }}
                        >
                          <AlertDialog.Trigger asChild>
                            <Button variant="danger" disabled={busy || suspended}>
                              Hủy đơn
                            </Button>
                          </AlertDialog.Trigger>
                          <AlertDialog.Portal>
                            <AlertDialog.Overlay className="dialog-overlay" />
                            <AlertDialog.Content className="dialog-content space-y-4">
                              <AlertDialog.Title className="text-lg font-medium">
                                Hủy đơn hàng?
                              </AlertDialog.Title>
                              <AlertDialog.Description>
                                Đơn sẽ chuyển sang trạng thái đã hủy. Thao tác này không thể hoàn tác.
                              </AlertDialog.Description>
                              {error && <ErrorText>{error}</ErrorText>}
                              <div className="flex justify-end gap-2">
                                <AlertDialog.Cancel asChild>
                                  <Button disabled={busy}>Quay lại</Button>
                                </AlertDialog.Cancel>
                                <Button variant="danger" disabled={busy} onClick={() => fulfill('canceled')}>
                                  {busy ? 'Đang hủy…' : 'Xác nhận hủy'}
                                </Button>
                              </div>
                            </AlertDialog.Content>
                          </AlertDialog.Portal>
                        </AlertDialog.Root>
                      ) : (
                        <Button
                          key={s}
                          variant={s === 'returned' ? 'danger' : 'primary'}
                          disabled={
                            busy ||
                            suspended ||
                            (s === 'packing' &&
                              (o.review_status !== 'released' ||
                                (o.payment.method !== 'cod' && o.payment.status !== 'succeeded')))
                          }
                          onClick={() => fulfill(s)}
                        >
                          {label(fulfillmentLabel, s)}
                        </Button>
                      ),
                    )}
                  </div>
                  {o.fulfillment_status === 'unfulfilled' && (
                    <p className="subtle">
                      {o.review_status !== 'released'
                        ? 'Đơn cần được Release trước khi đóng gói.'
                        : o.payment.method !== 'cod' && o.payment.status !== 'succeeded'
                          ? 'Cần thanh toán thành công trước khi đóng gói.'
                          : 'Đơn đã đủ điều kiện đóng gói.'}
                    </p>
                  )}
                </Section>
                <Section title="Địa chỉ nhận hàng">
                  <AddressBlock address={o.address} />
                  <p className="subtle">
                    Mã phường/xã: {o.address.ward_code ?? 'Chưa biết'} · Số nhà:{' '}
                    {o.address.house_no ?? 'Chưa biết'}
                  </p>
                  <p className="subtle">
                    Cấu trúc địa chỉ:{' '}
                    {o.address.parse_ok === null
                      ? 'Chưa biết'
                      : o.address.parse_ok
                        ? 'Đọc được'
                        : 'Chưa đọc được'}{' '}
                    · không phải kết quả xác minh.
                  </p>
                </Section>
              </Tabs.Content>
              <Tabs.Content value="history" className="review-panel">
                <Section title="Lịch sử quyết định">
                  {o.decisions.length ? (
                    o.decisions.map((d) => (
                      <article key={d.decision_id} className="border-b border-gray-100 pb-4">
                        <div className="page-head">
                          <strong className="font-medium">{label(decisionLabel, d.action)}</strong>
                          <span className="subtle">
                            {dateTime(d.created_at)} · {label(actorLabel, d.actor_type)}
                          </span>
                        </div>
                        <p className="my-2">{d.reason}</p>
                        <p className="subtle break-all">
                          Assessment {d.assessment_id}
                          {d.assessment_id !== a?.assessment_id && ' · Assessment cũ'}
                        </p>
                      </article>
                    ))
                  ) : (
                    <p className="empty">Chưa có quyết định review.</p>
                  )}
                </Section>
                <Section title="Tiến trình đơn hàng">
                  {o.events.length ? (
                    <ol className="space-y-4">
                      {o.events.map((ev, i) => (
                        <li key={i} className="border-l-2 border-amber-300 pl-4">
                          <p>
                            {label(eventLabel, ev.event_type)} · {label(actorLabel, ev.actor_type)}
                          </p>
                          {ev.reason && <p className="text-sm">{ev.reason}</p>}
                          <p className="subtle">
                            {dateTime(ev.occurred_at)} · Biết lúc {dateTime(ev.available_at)}
                          </p>
                        </li>
                      ))}
                    </ol>
                  ) : (
                    <p className="empty">Chưa có sự kiện.</p>
                  )}
                </Section>
              </Tabs.Content>
              <Tabs.Content value="cases" className="review-panel">
                <CaseHistory cases={o.cases} />
              </Tabs.Content>
            </Tabs.Root>
            {notice && (
              <div role="status" className="toast">
                {notice}
                <button className="ml-4" onClick={() => setNotice('')} aria-label="Đóng thông báo">
                  ×
                </button>
              </div>
            )}
          </>
        )
      }}
    </Loaded>
  )
}
