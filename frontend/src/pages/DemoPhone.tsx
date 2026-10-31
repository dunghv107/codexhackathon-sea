import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { api, enc, errMsg, profiles, useLoad } from '../api'
import { useStore } from '../store'
import { verificationLabel } from '../format'
import type { VerificationView } from '../types'
import { Button, ErrorText, Loaded, Section } from '../ui'
import VoiceReply from './VoiceReply'

const script = 'Xin chào, đây là cuộc gọi mô phỏng xác nhận đơn hàng. Anh chị có đồng ý nhận đơn hàng này không? Vui lòng trả lời rõ đồng ý nhận hàng hoặc từ chối nhận hàng, và cho biết thời gian nhận thuận tiện.'

export default function DemoPhone() {
  const [params] = useSearchParams()
  const shop = params.get('shop') ?? '', order = params.get('order') ?? ''
  return <PhoneCall key={`${shop}/${order}`} shop={shop} order={order} />
}

function PhoneCall({ shop, order }: { shop: string, order: string }) {
  const path = `/api/seller/shops/${enc(shop)}/orders/${enc(order)}/verification`
  const state = useLoad<VerificationView>(shop && order ? path : null)
  const suspended = useStore(profiles)?.seller?.status === 'suspended'
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [playing, setPlaying] = useState(false)
  const practice = !!state.data?.verification && state.data.verification.status !== 'pending'
  const [practiceResult, setPracticeResult] = useState<string | null>(null)
  useEffect(() => () => window.speechSynthesis?.cancel(), [])

  async function answer(key: string) {
    if (!state.data?.verification || busy) return
    if (practice) {
      window.speechSynthesis?.cancel()
      setPlaying(false)
      setPracticeResult(key === '1' ? 'Khách xác nhận' : key === '2' ? 'Khách từ chối' : 'Không trả lời')
      return
    }
    setBusy(true)
    setError('')
    window.speechSynthesis?.cancel()
    setPlaying(false)
    try {
      await api(`${path}/respond`, 'POST', { verification_id: state.data.verification.verification_id, key })
    } catch (e) { setError(errMsg(e)) }
    finally { state.reload(); setBusy(false) }
  }
  function speak() {
    setError('')
    if (!window.speechSynthesis) { setError('Trình duyệt không hỗ trợ đọc. Hãy đọc kịch bản bên dưới rồi thu âm câu trả lời.'); return }
    window.speechSynthesis.cancel()
    const speech = new SpeechSynthesisUtterance(script)
    speech.lang = 'vi-VN'
    speech.onstart = () => setPlaying(true)
    speech.onend = () => setPlaying(false)
    speech.onerror = (event) => {
      setPlaying(false)
      if (event.error !== 'canceled' && event.error !== 'interrupted')
        setError('Không phát được giọng đọc. Bạn vẫn có thể thu âm câu trả lời bên dưới.')
    }
    window.speechSynthesis.speak(speech)
  }
  return <main className="max-w-xl mx-auto p-6 space-y-5">
    <Link to={order ? `/seller/orders/${enc(order)}` : '/seller/orders'} className="underline">← Quay về đánh giá đơn</Link>
    <Section title="Điện thoại mô phỏng · DEMO">
      <p>Không gọi điện thật. Trang này dành cho người trình diễn, tách khỏi trang người mua.</p>
      <ol className="list-decimal pl-5 space-y-2 text-sm">
        <li>Phát kịch bản để nghe lời xác nhận; nếu máy không có giọng đọc, đọc nội dung bên dưới.</li>
        <li>Bấm Bắt đầu thu âm, nói câu trả lời rồi bấm Dừng và nhận dạng.</li>
        <li>Đọc lại transcript. Chỉ khi AI hiểu rõ ý định, nút Đồng ý hoặc Từ chối tương ứng mới xuất hiện để bạn xác nhận.</li>
        <li>Quay về đơn để xem kết quả và quyết định cho xử lý hoặc tạm giữ.</li>
      </ol>
      {!shop || !order ? <ErrorText>Mở cuộc gọi từ chi tiết đơn hàng của shop.</ErrorText> :
        <Loaded state={state}>{(view) => view.verification ? <>
          <p>Điện thoại: {view.phone_masked || 'Chưa có số điện thoại'}</p>
          <p role="status">{verificationLabel[view.verification.status]}</p>
          {view.verification.status !== 'pending' && <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 space-y-3">
            <p>Lượt xác minh đã kết thúc. Thu âm bên dưới đang ở chế độ tập thử, không ghi đè kết quả đã lưu.</p>
            <p className="text-sm">Muốn lưu phản hồi mới, mở đơn COD mức Trung bình đang Chờ phản hồi.</p>
          </div>}
          {practice && <p className="rounded bg-blue-50 p-3 font-medium">Chế độ tập thử — không ghi vào đơn hàng.</p>}
          {practiceResult && <p role="status">Kết quả tập thử: {practiceResult}. Kết quả đã lưu của đơn không đổi.</p>}
          <p className="card">{script}</p>
          <Button onClick={speak}>Phát kịch bản</Button>
          {playing && <p aria-live="polite">Đang phát kịch bản… Bấm thu âm để trả lời.</p>}
          {suspended && <p>Tài khoản người bán đang tạm khóa, chỉ có thể xem kết quả.</p>}
          <VoiceReply key={String(practice)} path={path}
            disabled={busy || !!suspended || state.loading || !!state.error} practice={practice} onConfirm={answer} />
          <p className="subtle">Kết quả không tự Release đơn. Người bán quyết định trong portal.</p>
          {view.verification.status !== 'pending' && <Link className="underline font-medium" to={`/seller/orders/${enc(order)}`}>Xem kết quả trên đơn và quyết định →</Link>}
        </> : <p>Đơn chưa có yêu cầu xác minh.</p>}</Loaded>}
      {error && <ErrorText>{error}</ErrorText>}
    </Section>
  </main>
}
