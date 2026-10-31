import { test, expect, vi } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import VoiceReply from '../src/pages/VoiceReply'
import { mount, mockApi, fail } from './helpers'

test.each([['confirmed', '1'], ['declined', '2'], ['unclear', null]])('voice preview %s requires explicit confirmation', async (intent, key) => {
  const fetch = mockApi(() => ({ transcript: 'Câu trả lời của khách', intent }))
  const confirm = vi.fn()
  mount(<VoiceReply path="/verification" disabled={false} practice={false} onConfirm={confirm} />)
  await userEvent.upload(screen.getByLabelText('Hoặc tải câu trả lời đã thu'), new File(['sound'], 'reply.wav', { type: 'audio/wav' }))
  await screen.findByText('Câu trả lời của khách', { exact: false })
  expect(confirm).not.toHaveBeenCalled()
  expect(fetch.mock.calls[0][0]).toBe('/verification/voice-preview')
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ audio: 'c291bmQ=', mime_type: 'audio/wav' })
  if (key) {
    await userEvent.click(screen.getByRole('button', { name: /nhận hàng · Xác nhận lưu/ }))
    expect(confirm).toHaveBeenCalledWith(key)
  } else expect(screen.queryByRole('button', { name: /nhận hàng · Xác nhận lưu/ })).toBeNull()
})

test('practice has a distinct non-saving confirmation', async () => {
  mockApi(() => ({ transcript: 'Đồng ý', intent: 'confirmed' }))
  const confirm = vi.fn()
  mount(<VoiceReply path="/verification" disabled={false} practice onConfirm={confirm} />)
  await userEvent.upload(screen.getByLabelText('Hoặc tải câu trả lời đã thu'), new File(['sound'], 'reply.wav', { type: 'audio/wav' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Đồng ý nhận hàng · Tập thử, không lưu' }))
  expect(confirm).toHaveBeenCalledWith('1')
})

test.each([0, 614401])('rejects invalid audio size %i before provider request', async size => {
  const fetch = mockApi(() => fail())
  mount(<VoiceReply path="/verification" disabled={false} practice={false} onConfirm={vi.fn()} />)
  await userEvent.upload(screen.getByLabelText('Hoặc tải câu trả lời đã thu'), new File([new Uint8Array(size)], 'reply.wav', { type: 'audio/wav' }))
  expect(screen.getByRole('alert').textContent).toMatch(/600 KB/)
  expect(fetch).not.toHaveBeenCalled()
})

test('provider failure never offers confirmation', async () => {
  mockApi(() => fail(503, 'ASR unavailable'))
  mount(<VoiceReply path="/verification" disabled={false} practice={false} onConfirm={vi.fn()} />)
  await userEvent.upload(screen.getByLabelText('Hoặc tải câu trả lời đã thu'), new File(['sound'], 'reply.wav', { type: 'audio/wav' }))
  expect(await screen.findByText('ASR unavailable')).toBeTruthy()
  expect(screen.queryByText('Xác nhận lưu kết quả này')).toBeNull()
})

function recorderMock() {
  const track = { stop: vi.fn() }
  const input = { getTracks: () => [track] }
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia: vi.fn().mockResolvedValue(input) } })
  let active
  vi.stubGlobal('MediaRecorder', class {
    static isTypeSupported() { return true }
    state = 'inactive'
    constructor() { active = this }
    start() { this.state = 'recording' }
    stop() { this.state = 'inactive'; this.ondataavailable?.({ data: new Blob(['sound']) }); this.onstop?.() }
  })
  return { track, getActive: () => active }
}

test('microphone recording stops tracks and creates only a preview', async () => {
  const { track } = recorderMock()
  mockApi(() => ({ transcript: '', intent: 'unclear' }))
  mount(<VoiceReply path="/verification" disabled={false} practice={false} onConfirm={vi.fn()} />)
  await userEvent.click(screen.getByRole('button', { name: 'Bắt đầu thu âm' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Dừng và nhận dạng' }))
  expect(await screen.findByText('Không nhận được lời nói.', { exact: false })).toBeTruthy()
  expect(track.stop).toHaveBeenCalled()
})

test('leaving while recording closes microphone without uploading', async () => {
  const { track } = recorderMock()
  const fetch = mockApi(() => fail())
  const { unmount } = mount(<VoiceReply path="/verification" disabled={false} practice={false} onConfirm={vi.fn()} />)
  await userEvent.click(screen.getByRole('button', { name: 'Bắt đầu thu âm' }))
  await screen.findByRole('button', { name: 'Dừng và nhận dạng' })
  unmount()
  expect(track.stop).toHaveBeenCalled()
  expect(fetch).not.toHaveBeenCalled()
})

test('denied microphone and recording errors give actionable feedback', async () => {
  const { getActive } = recorderMock()
  navigator.mediaDevices.getUserMedia.mockRejectedValueOnce(new Error('denied'))
  mount(<VoiceReply path="/verification" disabled={false} practice={false} onConfirm={vi.fn()} />)
  await userEvent.click(screen.getByRole('button', { name: 'Bắt đầu thu âm' }))
  expect(await screen.findByText(/Không mở được micro/)).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Bắt đầu thu âm' }))
  await waitFor(() => expect(getActive()).toBeTruthy())
  act(() => getActive().onerror())
  expect(screen.getByRole('alert').textContent).toMatch(/Thu âm thất bại/)
})

test('unsupported browser offers upload and keypad instead', async () => {
  vi.stubGlobal('MediaRecorder', undefined)
  mount(<VoiceReply path="/verification" disabled={false} practice={false} onConfirm={vi.fn()} />)
  await userEvent.click(screen.getByRole('button', { name: 'Bắt đầu thu âm' }))
  expect(screen.getByRole('alert').textContent).toMatch(/HTTPS hoặc localhost/)
})
