import { useState } from 'react'
import { AlertDialog, Tabs } from 'radix-ui'
import { Link, useParams } from 'react-router'
import { CaseHistory, RiskFacts } from './RiskReview'
import NetworkPanel from './NetworkPanel'
import { evidenceSummary, record } from './risk-evidence'
import { api, ApiError, currentShop, enc, errMsg, profiles, useLoad } from '../../api'
import {
  verificationLabel,
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
  const [explanation, setExplanation] = useState<{ assessment_id: string; source?: string; fallback_reason?: string | null; sentences: string[] } | null>(null)
  const [explaining, setExplaining] = useState(false)

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
    setExplaining(true)
    setBusy(true)
    setError('')
    try {
      setExplanation(await api(`${path}/explain`, 'POST'))
    } catch (e) {
      setError(errMsg(e))
    } finally {
      setBusy(false)
      setExplaining(false)
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
        const reasons = (Array.isArray(a?.reasons) ? a.reasons : []).map(record).filter(r => typeof r.rule_id === 'string')
        const first = reasons[0]
        const firstRule = a?.rule_config?.rules?.find(r => r.rule_id === first?.rule_id)
        const summary = first ? evidenceSummary(String(first.rule_id), record(record(a?.feature_snapshot)[String(first.rule_id)]), firstRule?.params ?? {}) : null
        const thresholds = a?.rule_config?.thresholds
        const callStatus = o.verification?.status
        const callMessage = callStatus === 'confirmed' ? 'Khách đã xác nhận trong mô phỏng. Bạn vẫn cần tự quyết định cho xử lý đơn.'
          : callStatus === 'declined' ? 'Khách đã từ chối trong mô phỏng. Kiểm tra lại với khách trước khi cho xử lý.'
          : callStatus === 'unanswered' ? 'Chưa liên lạc được trong mô phỏng. Không trả lời không đồng nghĩa với từ chối; cân nhắc liên hệ lại.'
          : callStatus === 'pending' ? 'Yêu cầu đang chờ phản hồi. Mở điện thoại mô phỏng để đóng vai khách và chọn kết quả.'
          : !o.demo_mode ? 'Mô phỏng cuộc gọi chưa được bật trên hệ thống này.'
          : 'Chưa có yêu cầu xác minh. Hệ thống tạo yêu cầu sau khi chấm đơn COD mức Trung bình, chưa giao và đang chờ duyệt hoặc tạm giữ.'
        const nextStep = o.fulfillment_status !== 'unfulfilled' ? 'Đơn đã chuyển sang tiến trình giao hàng. Theo dõi ở tab Giao hàng.'
          : o.review_status === 'released' ? 'Đã cho phép xử lý. Kiểm tra thanh toán và chuyển sang đóng gói ở tab Giao hàng.'
          : !a ? 'Chấm điểm đơn để có dữ kiện trước khi quyết định.'
          : 'Đối chiếu bằng chứng, kết quả xác minh và chọn cho xử lý hoặc tạm giữ đơn.'
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
            <header className="page-head seller-detail-head">
              <div className="min-w-0">
                <p className="eyebrow mb-2">
                  Chi tiết đơn hàng · {o.batch_id}
                  {o.is_synthetic ? ' · Mô phỏng' : ''}
                </p>
                <h1>Đánh giá đơn hàng</h1>
                <p className="subtle mt-2 break-all">Mã đơn: {o.order_id}</p>
                <p className="subtle mt-2 break-all">
                  Người mua: {o.buyer_display_name} · {o.buyer_id}
                </p>
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
            <div className="seller-order-status">
              <span className="subtle">Trạng thái đơn</span>
              <Badge map={reviewLabel} value={o.review_status} />
              <Badge map={fulfillmentLabel} value={o.fulfillment_status} />
              <Badge map={paymentMethodLabel} value={o.payment.method} />
            </div>
            {error && <ErrorText>{error}</ErrorText>}
            <section className="assessment-hero" data-risk={a?.risk_level ?? 'unassessed'} aria-label="Kết quả đánh giá KBS">
              <div className="assessment-score">
                <p className="eyebrow">KBS · Điểm rủi ro</p>
                <div className="assessment-score-value">{a?.score ?? '—'}<span> / {a?.rule_config?.score_cap ?? 100}</span></div>
                <Badge map={riskLabel} value={a?.risk_level ?? 'unassessed'} />
                <p className="subtle">Điểm rủi ro, không phải % gian lận.</p>
              </div>
              <div className="assessment-overview">
                <h2>{!a ? 'Đơn chưa được đánh giá' : a.risk_level === 'high' ? 'Có tín hiệu rủi ro cao cần kiểm tra' : a.risk_level === 'medium' ? 'Cần xác minh thêm trước khi xử lý' : 'Chưa có nhiều tín hiệu làm tăng điểm'}</h2>
                <p>{summary ?? (a ? `${reasons.length} tín hiệu đã kích hoạt trong lần chấm này. Xem dữ kiện và điều kiện cụ thể bên dưới.` : 'Chưa có căn cứ để hiển thị mức rủi ro. Hãy chấm điểm hoặc thử lại khi KBS sẵn sàng.')}</p>
                {a && <p className="subtle">KBS tính từ lịch sử đã biết trước {dateTime(a.input_cutoff_at)}. Kết quả cuộc gọi sau đó không tự thay đổi điểm này.</p>}
                {thresholds && <div className="assessment-thresholds"><span>Thấp: dưới {thresholds.medium_min}</span><span>Trung bình: {thresholds.medium_min} đến dưới {thresholds.high_min}</span><span>Cao: từ {thresholds.high_min}</span></div>}
                <p className="assessment-next">{nextStep}</p>
              </div>
            </section>
            <div className="seller-review-summary">
              <Section title="Xác minh qua cuộc gọi">
                <p>Điện thoại: {o.phone_masked || 'Chưa có số điện thoại'}</p>
                <p>{o.verification ? verificationLabel[o.verification.status] : 'Chưa có yêu cầu xác minh'}</p>
                <p>{callMessage}</p>
                <p className="subtle">Mô phỏng bằng giọng nói: xem transcript rồi xác nhận ý định đã nhận dạng. Chưa gọi điện thật; kết quả không tự đổi điểm, duyệt hoặc hủy đơn.</p>
                {o.verification?.answered_at && <p className="subtle">Ghi nhận lúc {dateTime(o.verification.answered_at)}</p>}
                {o.demo_mode && o.verification && <Link className="underline" to={`/demo/phone?shop=${enc(shop.shop_id)}&order=${enc(o.order_id)}`}>{callStatus === 'pending' ? 'Mở điện thoại mô phỏng →' : 'Xem kết quả mô phỏng →'}</Link>}
              </Section>
              <Section title="Quyết định review">
                <Badge map={reviewLabel} value={o.review_status} />
                <p className="text-sm text-gray-500">Cho xử lý để tiếp tục chuẩn bị hàng; tạm giữ để kiểm tra thêm. Quyết định và lý do của bạn được lưu vào lịch sử.</p>
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
                  <Section title="Hiểu đánh giá và biết cần hỏi khách điều gì">
                  <p>Yêu cầu AI giải thích lịch sử liên quan đến đơn bằng ngôn ngữ dễ hiểu và gợi ý thông tin cần xác nhận với khách.</p>
                  <div className="flex flex-wrap gap-2">
                    {!a && <Button disabled={busy || suspended || order.loading} onClick={() => assess(false)}>Chấm điểm</Button>}
                    {a && (
                      <Button disabled={busy || suspended || order.loading} onClick={explain}>
                        {explaining ? 'Đang lấy giải thích…' : 'Giải thích đánh giá'}
                      </Button>
                    )}
                  </div>
                  <p className="subtle">AI đọc dữ kiện lịch sử của lần chấm, không tính lại điểm. Kết quả cuộc gọi hiện tại được hiển thị riêng ở phần xác minh phía trên.</p>
                  {explaining && <p role="status">AI đang viết giải thích cho đơn này…</p>}
                {a && explanation?.assessment_id === a.assessment_id && (
                  <div className="explanation-result" aria-live="polite">
                    <strong>{explanation.source === 'llm' ? 'Diễn giải bằng AI' : 'Diễn giải từ dữ kiện đã lưu'}</strong>
                    {explanation.source === 'template' && <p>{({
                      disabled: 'AI chưa được bật. Đang hiển thị giải thích có sẵn từ KBS.',
                      no_fired_rules: 'Chưa có tín hiệu làm tăng điểm. Hệ thống giải thích trực tiếp từ kết quả đã lưu.',
                      provider_timeout: 'AI phản hồi quá thời gian chờ. Bạn có thể thử lại; điểm KBS vẫn giữ nguyên.',
                      provider_unavailable: 'Dịch vụ AI chưa phản hồi thành công. Đang dùng giải thích dự phòng.',
                      invalid_output: 'Phản hồi AI chưa hợp lệ. Đang dùng giải thích dự phòng đã kiểm soát.',
                      provider_refusal: 'AI không trả lời yêu cầu này. Đang dùng giải thích dự phòng.',
                    } as Record<string,string>)[explanation.fallback_reason ?? ''] ?? 'Đang hiển thị bản giải thích dự phòng từ KBS.'}</p>}
                    <div className="space-y-4 leading-7">
                      {explanation.sentences.map((sentence, index) => (
                        <p key={index}>{sentence}</p>
                      ))}
                    </div>
                    <p className="subtle">Lịch sử có liên kết không chứng minh người đặt đơn hiện tại đã thực hiện các lần mua trước. Đây là thông tin hỗ trợ kiểm tra; bạn vẫn quyết định xử lý đơn.</p>
                  </div>
                )}
                <details className="assessment-network"><summary>Xem bằng chứng và cách tính điểm</summary>
                  {a ? <RiskFacts assessment={a} /> : <p className="empty">Đơn chưa có kết quả chấm điểm.</p>}
                  {a && o.batch_id === 'olist_vn_sim' && <div className="space-y-3 mt-4">
                    <Button disabled={busy || suspended || order.loading} onClick={() => assess(a !== null)}>{a ? 'Chấm lại' : 'Chấm điểm'}</Button>
                    <p className="subtle">Chấm lại vẫn dùng mốc đặt hàng của đơn, không đưa dữ kiện phát sinh sau đó vào điểm.</p>
                  </div>}
                </details>
                  </Section>
                )}
                {o.batch_id === 'olist_vn_sim' && <details className="assessment-network"><summary>Khám phá các mối liên hệ · Network</summary><p className="subtle">Dùng để đối chiếu các lần đặt hàng có liên kết. Có liên kết không tự động có nghĩa là gian lận.</p><NetworkPanel key={a?.assessment_id ?? o.order_id} path={path} /></details>}
                {a && <details className="assessment-network"><summary>Nguồn dữ liệu và phiên bản đánh giá</summary>
                  <dl className="assessment-metadata"><div><dt>Nguồn</dt><dd>{o.source_system || 'Chưa có thông tin'} · {o.is_synthetic ? 'Dữ liệu mô phỏng' : 'Dữ liệu đã lưu'}</dd></div>
                    <div><dt>Phiên bản rule</dt><dd>{a.rule_version}</dd></div><div><dt>Mốc lịch sử được xét</dt><dd>{dateTime(a.input_cutoff_at)}</dd></div>
                    <div><dt>Chấm lúc</dt><dd>{dateTime(a.assessed_at)}</dd></div><div><dt>Mã assessment</dt><dd>{a.assessment_id}</dd></div></dl>
                </details>}
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
