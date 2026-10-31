import { test, expect, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import DemoPhone from '../src/pages/DemoPhone'
import NetworkPanel, { NetworkGraph } from '../src/pages/seller/NetworkPanel'
import SellerOrderDetail from '../src/pages/seller/OrderDetail'
import { PortalGuard } from '../src/layouts'
import { profiles, tokens } from '../src/api'
import { seller, buyer, order, shop, network, mount, mockApi, fail } from './helpers'

const verification = { verification_id: 'verification:1', assessment_id: 'assessment:1',
  status: 'pending', channel: 'demo', answered_at: null }
const phonePath = '/demo/phone?shop=shop%3A1&order=order%3A1'
const view = { verification, phone_masked: '***0001' }

test.each(['/demo/phone', '/demo/phone?shop=shop:1', '/demo/phone?order=order:1'])('phone requires both identifiers before requesting data: %s', (path) => {
  const fetch = mockApi(() => view)
  mount(<DemoPhone />, path)
  expect(screen.getByRole('alert').textContent).toMatch(/Mở cuộc gọi từ chi tiết/)
  expect(fetch).not.toHaveBeenCalled()
})

test('phone without verification shows no answer or speech controls', async () => {
  mockApi(() => ({ verification: null, phone_masked: '' }))
  mount(<DemoPhone />, phonePath)
  expect(await screen.findByText('Đơn chưa có yêu cầu xác minh.')).toBeTruthy()
  expect(screen.queryByRole('button')).toBeNull()
})

test.each([['1', 'confirmed', 'Khách xác nhận'], ['2', 'declined', 'Khách từ chối'], ['0', 'unanswered', 'Không trả lời']])(
  'phone records key %s against its verification, refreshes status and disables repeated responses', async (key, status, message) => {
    let result = structuredClone(view)
    const fetch = mockApi((path, options) => {
      if (options.method === 'POST') { result.verification.status = status; return result.verification }
      return result
    })
    const cancel = vi.fn()
    vi.stubGlobal('speechSynthesis', { cancel, speak: vi.fn() })
    const { unmount } = mount(<DemoPhone />, phonePath)
    await screen.findByText('Điện thoại: ***0001')
    await userEvent.click(screen.getByRole('button', { name: new RegExp(`^${key} ·`) }))
    expect(await screen.findByText(message)).toBeTruthy()
    const requests = fetch.mock.calls.filter(([, options]) => options.method === 'POST')
    expect(requests).toHaveLength(1)
    expect(requests[0][0]).toBe('/api/seller/shops/shop%3A1/orders/order%3A1/verification/respond')
    expect(JSON.parse(requests[0][1].body)).toEqual({ verification_id: 'verification:1', key })
    expect(screen.getByRole('button', { name: /^1 ·/ }).disabled).toBe(true)
    expect(screen.getByRole('button', { name: /^2 ·/ }).disabled).toBe(true)
    expect(screen.getByRole('button', { name: /^0 ·/ }).disabled).toBe(true)
    unmount()
    expect(cancel).toHaveBeenCalledTimes(2)
  },
)

test('phone rejects an API error without claiming success and preserves the current verification', async () => {
  const fetch = mockApi((path, options) => options.method === 'POST' ? fail(409, 'Verification expired') : view)
  mount(<DemoPhone />, phonePath)
  await screen.findByText('Chờ phản hồi')
  await userEvent.click(screen.getByRole('button', { name: /^1 ·/ }))
  expect(await screen.findByText('Verification expired')).toBeTruthy()
  expect(screen.getByText('Chờ phản hồi')).toBeTruthy()
  expect(fetch.mock.calls.filter(([, options]) => options.method === 'GET').length).toBe(2)
})

test('phone blocks answers for a suspended seller and presents missing-phone fallback', async () => {
  profiles.set({ buyer, seller: { ...seller, status: 'suspended' } })
  mockApi(() => ({ ...view, phone_masked: '' }))
  mount(<DemoPhone />, phonePath)
  await screen.findByText('Điện thoại: Chưa có số điện thoại')
  expect(screen.getByRole('button', { name: /^1 ·/ }).disabled).toBe(true)
})

test('speech has a readable fallback when the browser has no speech engine', async () => {
  vi.stubGlobal('speechSynthesis', undefined)
  mockApi(() => view)
  mount(<DemoPhone />, phonePath)
  await userEvent.click(await screen.findByRole('button', { name: 'Phát kịch bản' }))
  expect(screen.getByRole('alert').textContent).toMatch(/không hỗ trợ đọc/)
  expect(screen.getByText(/Xin chào, đây là cuộc gọi mô phỏng/)).toBeTruthy()
  expect(screen.getByRole('button', { name: /^1 ·/ }).disabled).toBe(false)
})

test('speech uses Vietnamese, stops prior speech and treats real playback errors separately from cancellation', async () => {
  const speak = vi.fn(), cancel = vi.fn()
  vi.stubGlobal('speechSynthesis', { speak, cancel })
  vi.stubGlobal('SpeechSynthesisUtterance', class { constructor(text) { this.text = text } })
  mockApi(() => view)
  mount(<DemoPhone />, phonePath)
  await userEvent.click(await screen.findByRole('button', { name: 'Phát kịch bản' }))
  const utterance = speak.mock.calls[0][0]
  expect(utterance.lang).toBe('vi-VN')
  expect(utterance.text).toContain('cuộc gọi mô phỏng')
  expect(cancel).toHaveBeenCalledOnce()
  act(() => utterance.onerror({ error: 'canceled' }))
  expect(screen.queryByRole('alert')).toBeNull()
  act(() => utterance.onerror({ error: 'interrupted' }))
  expect(screen.queryByRole('alert')).toBeNull()
  act(() => utterance.onerror({ error: 'audio-busy' }))
  expect(screen.getByRole('alert').textContent).toMatch(/Không phát được giọng đọc/)
})

test('phone route guard excludes buyers and anonymous users before fetching verification', async () => {
  profiles.set({ buyer, seller: null })
  tokens.set({ access_token: 'test' })
  const fetch = mockApi(() => view)
  const first = mount(<PortalGuard role="seller"><DemoPhone /></PortalGuard>, phonePath)
  expect(screen.getByText(/không có hồ sơ người bán/)).toBeTruthy()
  expect(fetch).not.toHaveBeenCalled()
  first.unmount()
  tokens.set(null)
  const { router } = mount(<PortalGuard role="seller"><DemoPhone /></PortalGuard>, phonePath)
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(fetch).not.toHaveBeenCalled()
})

test('network distinguishes identities and aggregate peers, ignores missing edge endpoints and keeps full labels accessible', async () => {
  const label = 'An identity label longer than twenty nine characters'
  const { container } = render(<NetworkGraph network={{ ...network, nodes: [
    { id: 'root', kind: 'checkout', label: 'Selected checkout', count: 1 },
    { id: 'identity', kind: 'identity', label, count: 2 },
    { id: 'peers', kind: 'aggregate', label: 'Other shops', count: 12 },
  ], edges: [{ source: 'root', target: 'identity' }, { source: 'identity', target: 'peers' },
    { source: 'missing', target: 'root' }, { source: 'root', target: 'missing' }], truncated: true }} />)
  expect(screen.getByRole('img')).toBeTruthy()
  const viewport = screen.getByRole('region', { name: 'Sơ đồ liên hệ có thể cuộn' })
  expect(viewport.tabIndex).toBe(0)
  await userEvent.tab()
  expect(document.activeElement).toBe(viewport)
  expect(container.querySelectorAll('line')).toHaveLength(2)
  expect(screen.getByText(`${label} · 2`, { selector: 'li' })).toBeTruthy()
  expect(container.querySelector('text').textContent).toBe('Selected checkout')
  expect(container.querySelectorAll('text')[1].textContent).toBe(`${label.slice(0, 28)}…`)
  expect(screen.getByText('Other shops (12)')).toBeTruthy()
  expect(screen.getByRole('status').textContent).toMatch(/giới hạn số nút/)
})

test('network loading errors can be retried and a single checkout has no fabricated links', async () => {
  let failing = true
  mockApi(() => failing ? fail(503, 'Network temporarily unavailable') : { ...network, nodes: [{ id: 'root', kind: 'checkout', label: 'Selected checkout', count: 1 }] })
  mount(<NetworkPanel path="/orders/order%3A1" />)
  expect(await screen.findByText('Network temporarily unavailable')).toBeTruthy()
  failing = false
  await userEvent.click(screen.getByRole('button', { name: 'Thử lại' }))
  expect(await screen.findByText('Chưa có định danh để hiển thị liên hệ.')).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
})

test('seller exposes only masked phone and an encoded demo link when explicitly enabled', async () => {
  mockApi((path) => path.endsWith('/network') ? network : { ...order, phone_masked: '***0001', demo_mode: true, verification })
  mount(<SellerOrderDetail />, '/seller/orders/order:1', '/seller/orders/:orderId')
  expect(await screen.findByText('Điện thoại: ***0001')).toBeTruthy()
  const link = screen.getByRole('link', { name: 'Mở điện thoại mô phỏng →' })
  expect(link.getAttribute('href')).toBe(`/demo/phone?shop=${encodeURIComponent(shop.shop_id)}&order=order%3A1`)
  expect(screen.getByText('Chờ phản hồi')).toBeTruthy()
  expect(screen.queryByText('0900000001')).toBeNull()
})
