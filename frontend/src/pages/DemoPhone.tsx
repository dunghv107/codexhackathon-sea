import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { api, enc, errMsg, profiles, useLoad } from '../api'
import { useStore } from '../store'
import { verificationLabel } from '../format'
import type { VerificationView } from '../types'
import { Button, ErrorText, Loaded, Section } from '../ui'

const script = 'Xin chào, đây là cuộc gọi mô phỏng xác nhận đơn hàng. Bấm phím 1 để xác nhận nhận hàng. Bấm phím 2 để từ chối. Bấm phím 0 để mô phỏng không trả lời.'

export default function DemoPhone() {
  const [params] = useSearchParams()
  const shop = params.get('shop') ?? '', order = params.get('order') ?? ''
  const path = `/api/seller/shops/${enc(shop)}/orders/${enc(order)}/verification`
  const state = useLoad<VerificationView>(shop && order ? path : null)
  const suspended = useStore(profiles)?.seller?.status === 'suspended'
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => () => window.speechSynthesis?.cancel(), [])

  async function answer(key: string) {
    if (!state.data?.verification || busy) return
    setBusy(true)
    setError('')
    window.speechSynthesis?.cancel()
    try {
      await api(`${path}/respond`, 'POST', { verification_id: state.data.verification.verification_id, key })
    } catch (e) { setError(errMsg(e)) }
    finally { state.reload(); setBusy(false) }
  }
  function speak() {
    setError('')
    if (!window.speechSynthesis) { setError('Trình duyệt không hỗ trợ đọc. Hãy đọc kịch bản bên dưới và chọn phím.'); return }
    window.speechSynthesis.cancel()
    const speech = new SpeechSynthesisUtterance(script)
    speech.lang = 'vi-VN'
    speech.onerror = (event) => {
      if (event.error !== 'canceled' && event.error !== 'interrupted')
        setError('Không phát được giọng đọc. Bạn vẫn có thể chọn phím bên dưới.')
    }
    window.speechSynthesis.speak(speech)
  }
  return <main className="max-w-xl mx-auto p-6 space-y-5">
    <Link to="/seller/orders" className="underline">← Portal người bán</Link>
    <Section title="Điện thoại mô phỏng · DEMO">
      <p>Không gọi điện thật. Trang này dành cho người trình diễn, tách khỏi trang người mua.</p>
      {!shop || !order ? <ErrorText>Mở cuộc gọi từ chi tiết đơn hàng của shop.</ErrorText> :
        <Loaded state={state}>{(view) => view.verification ? <>
          <p>Điện thoại: {view.phone_masked || 'Chưa có số điện thoại'}</p>
          <p role="status">{verificationLabel[view.verification.status]}</p>
          <p className="card">{script}</p>
          <Button onClick={speak}>Phát kịch bản</Button>
          <div className="flex flex-wrap gap-3">
            {(['1', '2', '0'] as const).map((key) => <Button key={key}
              disabled={busy || suspended || state.loading || !!state.error || view.verification?.status !== 'pending'}
              onClick={() => answer(key)}>
              {key} · {key === '1' ? 'Xác nhận' : key === '2' ? 'Từ chối' : 'Không trả lời'}
            </Button>)}
          </div>
          <p className="subtle">Kết quả không tự Release đơn. Người bán quyết định trong portal.</p>
        </> : <p>Đơn chưa có yêu cầu xác minh.</p>}</Loaded>}
      {error && <ErrorText>{error}</ErrorText>}
    </Section>
  </main>
}
