import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation } from 'react-router'
import { api, errMsg, logout, qs, useAuthed, useLoad } from '../../api'
import { dateTime } from '../../format'
import type { Page } from '../../types'
import { Button, ErrorText, Field, Loaded, SelectField, TextArea } from '../../ui'
import { AdaptiveFacts } from '../seller/AdaptiveFacts'

type Observation = {
  assessment_id: string; order_id: string; shop_name: string; rule_version: string;
  baseline_version: string; assessed_at: string; input_cutoff_at: string; signals: unknown[];
}

function EvidenceForm() {
  const [kind, setKind] = useState('case_verdict')
  const [caseType, setCaseType] = useState('delivery')
  const [recordID, setRecordID] = useState(() => `admin:${crypto.randomUUID()}`)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const values = new FormData(form)
    setBusy(true); setError(''); setMessage('')
    try {
      const data: Record<string, unknown> = Object.fromEntries(values)
      delete data.source_ref; delete data.occurred_at; delete data.content_file
      if (kind === 'evidence_review') {
        const file = values.get('content_file')
        if (!(file instanceof File) || file.size === 0 || file.size > 600_000) throw new Error('Chọn tệp bằng chứng tối đa 600 KB.')
        const encoded = await new Promise<string>((resolve, reject) => {
          const reader = new FileReader()
          reader.onload = () => resolve(String(reader.result).split(',')[1])
          reader.onerror = () => reject(new Error('Không đọc được tệp.'))
          reader.readAsDataURL(file)
        })
        data.content_base64 = encoded
      }
      await api('/api/admin/risk/evidence', 'POST', {
        record_id: recordID, kind, source_ref: values.get('source_ref'),
        occurred_at: new Date(String(values.get('occurred_at'))).toISOString(), data,
      })
      setRecordID(`admin:${crypto.randomUUID()}`)
      setMessage('Đã lưu. Dữ liệu chỉ được dùng cho các lần mua sau thời điểm hệ thống nhận bản ghi; assessment cũ giữ nguyên.')
    } catch (e) { setError(errMsg(e)) } finally { setBusy(false) }
  }
  const outcome = { delivery: ['delivery_failure', 'Lỗi giao hàng'], cancellation: ['cancellation', 'Hủy đơn'],
    return: ['return_abuse', 'Lạm dụng trả hàng đã xác minh'], refund: ['refund_abuse', 'Lạm dụng hoàn tiền đã xác minh'] }[caseType] as [string, string]
  return <details className="card">
    <summary className="cursor-pointer font-semibold">Ghi nhận dữ liệu kiểm chứng</summary>
    <p className="subtle my-3">Dành cho quản trị viên. Lưu nguồn đối chiếu và người ghi nhận; không tự khóa tài khoản hoặc từ chối hoàn tiền.</p>
    <SelectField label="Loại dữ liệu" value={kind} onValueChange={(value) => {
      setKind(value); setRecordID(`admin:${crypto.randomUUID()}`); setError(''); setMessage('')
    }} options={[
      ['case_verdict', 'Kết luận hồ sơ'], ['identity_link', 'Liên kết danh tính đã kiểm chứng'],
      ['enforcement', 'Quyết định xử lý vi phạm'], ['evidence_review', 'Đối chiếu bằng chứng'],
    ]} />
    <form key={kind} onSubmit={submit} className="mt-4 space-y-4">
      <Field label="Nguồn kiểm chứng / mã hồ sơ đối chiếu" name="source_ref" required maxLength={500} />
      <Field label="Thời điểm sự việc hoặc kết luận" name="occurred_at" type="datetime-local" required />
      {kind === 'identity_link' && <>
        <Field label="Mã người mua thứ nhất" name="buyer_id" required />
        <Field label="Mã người mua thứ hai" name="related_buyer_id" required />
        <SelectField label="Nguồn xác minh liên kết" name="edge_type" defaultValue="verified_document" options={[
          ['verified_document', 'Giấy tờ đã xác minh'], ['verified_payment', 'Thông tin thanh toán đã xác minh'], ['verified_account', 'Quyền sở hữu tài khoản đã xác minh'],
        ]} />
        <SelectField label="Trạng thái liên kết" name="status" defaultValue="verified" options={[
          ['verified', 'Đã xác minh'], ['revoked', 'Thu hồi xác minh từ nguồn này'],
        ]} />
        <p className="subtle">Rule yêu cầu ít nhất hai loại và hai nguồn bằng chứng độc lập. Số điện thoại nhập tay và cuộc gọi demo không đáp ứng điều kiện này.</p>
      </>}
      {kind === 'enforcement' && <>
        <Field label="Mã người mua đã được xử lý" name="buyer_id" required />
        <SelectField label="Quyết định đã thực hiện" name="action" defaultValue="restricted" options={[
          ['restricted', 'Hạn chế do vi phạm'], ['reinstated', 'Khôi phục sau xử lý'],
        ]} />
        <SelectField label="Nhóm hành vi đã xác minh" name="abuse_code" defaultValue="delivery" options={[
          ['delivery', 'Giao hàng'], ['cancellation', 'Hủy đơn'], ['after_sale', 'Hoàn trả / hoàn tiền'],
        ]} />
      </>}
      {kind === 'case_verdict' && <>
        <Field label="Mã đơn hàng" name="order_id" required />
        <Field label="Mã hồ sơ hiện có (bỏ trống để tạo hồ sơ)" name="case_id" />
        <SelectField label="Loại hồ sơ" name="case_type" value={caseType} onValueChange={setCaseType} options={[
          ['delivery', 'Giao hàng'], ['cancellation', 'Hủy đơn'], ['return', 'Trả hàng'], ['refund', 'Hoàn tiền'],
        ]} />
        <SelectField label="Mức xác minh" name="verification_status" defaultValue="unresolved" options={[
          ['unresolved', 'Chưa kết luận'], ['insufficient_evidence', 'Chưa đủ bằng chứng'], ['verified', 'Đã xác minh'],
        ]} />
        <SelectField label="Bên chịu trách nhiệm" name="responsible_party" defaultValue="unknown" options={[
          ['unknown', 'Chưa rõ'], ['buyer', 'Người mua'], ['seller', 'Người bán'], ['carrier', 'Đơn vị vận chuyển'], ['shared', 'Nhiều bên'],
        ]} />
        <SelectField key={caseType} label="Kết quả đối chiếu" name="outcome_code" defaultValue="legitimate" options={[
          ['legitimate', 'Không xác minh hành vi lạm dụng'], outcome,
        ]} />
        <TextArea label="Căn cứ kết luận" name="rationale" required maxLength={500} />
      </>}
      {kind === 'evidence_review' && <>
        <Field label="Mã bằng chứng trong hồ sơ" name="evidence_id" required />
        <Field label="Tệp nội dung đã đối chiếu (tối đa 600 KB)" name="content_file" type="file" required />
        <SelectField label="Kết luận đối chiếu" name="verdict" defaultValue="unresolved" options={[
          ['unresolved', 'Chưa kết luận'], ['no_contradiction', 'Không phát hiện mâu thuẫn'], ['contradiction_verified', 'Mâu thuẫn đã được xác minh'],
        ]} />
        <p className="subtle">Hệ thống tính dấu vân tay nội dung. Hai tệp giống nhau chỉ tạo cảnh báo khi có hồ sơ độc lập và mâu thuẫn được xác minh.</p>
      </>}
      {error && <ErrorText>{error}</ErrorText>}
      {message && <p role="status" className="text-sm text-green-800">{message}</p>}
      <Button variant="primary" disabled={busy}>{busy ? 'Đang lưu…' : 'Lưu ghi nhận'}</Button>
    </form>
  </details>
}

export default function AdminRisk() {
  const authed = useAuthed()
  const location = useLocation()
  const [status, setStatus] = useState('all')
  const [page, setPage] = useState(1)
  const observations = useLoad<Page<Observation>>(authed ? `/api/admin/risk/observations?${qs({ status, page, limit: 20 })}` : null)
  if (!authed) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <main className="mx-auto max-w-6xl space-y-6 p-6">
    <header className="flex flex-wrap items-center justify-between gap-3">
      <div><Link to="/" className="brand-mark">afd.</Link><h1 className="mt-3">Quản trị rủi ro sàn</h1></div>
      <div className="flex gap-3"><Link to="/seller/shops">Kênh người bán</Link><Button onClick={() => logout()}>Đăng xuất</Button></div>
    </header>
    <p className="subtle">Theo dõi nhóm tài khoản, lịch sử xử lý và vấn đề của người bán (A12, A13, A16). Tất cả đang chạy shadow, không tự xử phạt.</p>
    <SelectField label="Trạng thái tín hiệu" value={status} onValueChange={(value) => { setStatus(value); setPage(1) }} options={[
      ['all', 'Tất cả'], ['fired', 'Có tín hiệu'], ['unknown', 'Chưa đủ dữ liệu'], ['clear', 'Không kích hoạt'],
    ]} />
    <Loaded state={observations}>{data => <>
      <EvidenceForm />
      <div className="flex items-center justify-between"><p>{data.total} assessment phù hợp</p><Button onClick={observations.reload}>Làm mới</Button></div>
      {data.items.length === 0 && <p className="card">Chưa có kết quả phù hợp. Các đơn được chấm bằng phiên bản đủ 18 rule sẽ xuất hiện tại đây.</p>}
      {data.items.map(item => <article key={item.assessment_id} className="card space-y-3">
        <h2>{item.shop_name}</h2>
        <p className="subtle break-all">Đơn {item.order_id} · Chấm lúc {dateTime(item.assessed_at)}</p>
        <AdaptiveFacts value={{ mode: 'shadow', baseline_version: item.baseline_version, input_cutoff_at: item.input_cutoff_at, signals: item.signals }} />
      </article>)}
      <div className="flex gap-3">
        <Button disabled={page <= 1} onClick={() => setPage(page - 1)}>Trang trước</Button>
        <span>Trang {page}</span>
        <Button disabled={page >= data.total_pages} onClick={() => setPage(page + 1)}>Trang sau</Button>
      </div>
    </>}</Loaded>
  </main>
}
