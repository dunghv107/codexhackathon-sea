import { useEffect, useRef, useState } from 'react'
import { api, errMsg } from '../api'
import { Button, ErrorText } from '../ui'

type Preview = { transcript: string; intent: 'confirmed' | 'declined' | 'unclear' }
const labels = { confirmed: 'Đồng ý nhận hàng', declined: 'Từ chối nhận hàng', unclear: 'Chưa rõ ý định — cần hỏi lại' }
const formats = ['audio/webm', 'audio/ogg', 'audio/mp4', 'audio/wav']

export default function VoiceReply({ path, disabled, practice, onConfirm }: {
  path: string; disabled: boolean; practice: boolean; onConfirm: (key: string) => Promise<void>
}) {
  const [recording, setRecording] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<Preview | null>(null)
  const recorder = useRef<MediaRecorder | null>(null)
  const stream = useRef<MediaStream | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
      clearTimeout(timer.current)
      if (recorder.current) { recorder.current.onstop = null; if (recorder.current.state !== 'inactive') recorder.current.stop() }
      stream.current?.getTracks().forEach(track => track.stop())
    }
  }, [])

  async function recognize(blob: Blob) {
    setPreview(null)
    setError('')
    if (!blob.size || blob.size > 614400) { setError('Bản ghi phải có âm thanh và không vượt quá 600 KB. Hãy ghi lại ngắn hơn.'); return }
    const mime = blob.type.split(';')[0]
    if (!formats.includes(mime)) { setError('Dùng tệp WebM, Ogg, MP4 hoặc WAV.'); return }
    setBusy(true)
    try {
      const audio = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => resolve(String(reader.result).split(',')[1])
        reader.onerror = () => reject(new Error('Không đọc được âm thanh.'))
        reader.readAsDataURL(blob)
      })
      const result = await api<Preview>(`${path}/voice-preview`, 'POST', { audio, mime_type: mime })
      if (alive.current) setPreview(result)
    } catch (e) { if (alive.current) setError(errMsg(e)) }
    finally { if (alive.current) setBusy(false) }
  }

  function stop() {
    clearTimeout(timer.current)
    if (recorder.current?.state === 'recording') recorder.current.stop()
    stream.current?.getTracks().forEach(track => track.stop())
    setRecording(false)
  }

  async function start() {
    setError(''); setPreview(null)
    if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) {
      setError('Micro cần trình duyệt hỗ trợ và HTTPS hoặc localhost. Bạn có thể tải tệp âm thanh thay thế.'); return
    }
    setBusy(true)
    try {
      const input = await navigator.mediaDevices.getUserMedia({ audio: true })
      if (!alive.current) { input.getTracks().forEach(track => track.stop()); return }
      stream.current = input
      const mimeType = formats.find(type => MediaRecorder.isTypeSupported(type))
      if (!mimeType) throw new Error('Trình duyệt không hỗ trợ định dạng thu âm. Hãy tải tệp âm thanh.')
      const active = new MediaRecorder(input, { mimeType, audioBitsPerSecond: 32000 })
      recorder.current = active
      const chunks: Blob[] = []
      active.ondataavailable = event => { if (event.data.size) chunks.push(event.data) }
      active.onstop = () => { if (alive.current) void recognize(new Blob(chunks, { type: mimeType })) }
      active.onerror = () => {
        active.onstop = null
        stop()
        setError('Thu âm thất bại. Hãy thử lại hoặc tải tệp âm thanh.')
      }
      window.speechSynthesis?.cancel()
      active.start()
      setRecording(true)
      timer.current = setTimeout(stop, 20000)
    } catch { stream.current?.getTracks().forEach(track => track.stop()); if (alive.current) setError('Không mở được micro. Kiểm tra quyền micro hoặc tải tệp âm thanh.') }
    finally { if (alive.current) setBusy(false) }
  }

  return <section className="rounded-lg border border-blue-200 bg-blue-50 p-4 space-y-3" aria-label="Trả lời bằng giọng nói">
    <h3 className="font-semibold">Trả lời bằng giọng nói · AI</h3>
    <p className="text-sm">Thu tối đa 20 giây hoặc tải tệp tối đa 600 KB. Âm thanh được gửi đến dịch vụ ASR đã cấu hình; ứng dụng không lưu bản ghi hay lời nói vào đơn.</p>
    <Button disabled={disabled || busy} onClick={recording ? stop : start}>{recording ? 'Dừng và nhận dạng' : 'Bắt đầu thu âm'}</Button>
    <label className="block text-sm">Hoặc tải câu trả lời đã thu
      <input className="block mt-2 max-w-full" type="file" accept="audio/webm,audio/ogg,audio/mp4,audio/wav" disabled={disabled || busy || recording}
        onChange={event => { const file = event.target.files?.[0]; event.target.value = ''; if (file) void recognize(file) }} />
    </label>
    {recording && <p role="status">Đang thu âm… Hãy nói câu trả lời của khách.</p>}
    {busy && <p role="status">Đang xử lý giọng nói… Chưa ghi kết quả vào đơn.</p>}
    {error && <ErrorText>{error}</ErrorText>}
    {preview && <div className="space-y-3" aria-live="polite">
      <p><strong>ASR nghe được:</strong> {preview.transcript || 'Không nhận được lời nói.'}</p>
      <p><strong>AI hiểu:</strong> {labels[preview.intent]}</p>
      <p className="text-sm">Kiểm tra nội dung trước khi xác nhận. Nếu nhận sai hoặc chưa rõ, hãy thu lại. Không có lời đồng ý rõ ràng thì không xác nhận nhận hàng.</p>
      {preview.intent !== 'unclear' && <Button disabled={disabled || busy} onClick={async () => { await onConfirm(preview.intent === 'confirmed' ? '1' : '2'); setPreview(null) }}>
        {preview.intent === 'confirmed' ? 'Đồng ý nhận hàng' : 'Từ chối nhận hàng'}{practice ? ' · Tập thử, không lưu' : ' · Xác nhận lưu'}
      </Button>}
    </div>}
  </section>
}
