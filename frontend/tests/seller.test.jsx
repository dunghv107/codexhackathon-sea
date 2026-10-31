import { test, expect, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { screen, waitFor, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Dashboard from '../src/pages/seller/Dashboard'
import Products from '../src/pages/seller/Products'
import Shops from '../src/pages/seller/Shops'
import Orders from '../src/pages/seller/Orders'
import OrderDetail from '../src/pages/seller/OrderDetail'
import { currentShop, profiles } from '../src/api'
import { listing, order, shop, seller, assessment, network, page, mockApi, mount, fail } from './helpers'

test.each([
  ['Bảng đơn hàng', Orders, order],
  ['Bảng sản phẩm', Products, listing],
])('seller table region remains keyboard reachable and scrollable: %s', async (name, Component, row) => {
  mockApi(path => path.includes('/categories') ? [] : page([row]))
  const style = document.createElement('style')
  style.textContent = readFileSync('src/seller.css', 'utf8')
  document.head.append(style)
  try {
    mount(<Component />)
    const viewport = await screen.findByRole('region', { name })
    expect(viewport.tagName).toBe('SECTION')
    expect(viewport.getAttribute('role')).toBeNull()
    expect(viewport.tabIndex).toBe(0)
    expect(getComputedStyle(viewport).overflow).toBe('auto')
    expect(Number.parseFloat(getComputedStyle(within(viewport).getByRole('table')).minWidth)).toBeGreaterThan(600)
    for (let step = 0; step < 20 && document.activeElement !== viewport; step++) {
      await userEvent.tab()
    }
    expect(document.activeElement).toBe(viewport)
  } finally {
    style.remove()
  }
})

test.each([true, false])('dashboard shows counts and held orders, populated=%s', async (populated) => {
  mockApi((path) => path.includes('/dashboard') ? {
    review_status: { pending: 1, hold: 2, released: 3 },
    fulfillment_status: { unfulfilled: 6, packing: 0 },
    risk_level: { high: populated ? 2 : 0, medium: 0, low: 0, unassessed: 0 },
  } : page(populated ? [order, { ...order, order_id: 'unscored', risk_level: null }] : []))
  mount(<Dashboard />)
  await screen.findByLabelText('Tổng hợp trạng thái kiểm tra')
  if (populated) expect(await screen.findByRole('link', { name: /order:1/ })).toBeTruthy()
  else expect(await screen.findByText('Không có đơn tạm giữ cần xử lý')).toBeTruthy()
  expect(screen.getByRole('link', { name: /Đối chiếu dữ kiện để đưa ra quyết định/ }).getAttribute('href')).toContain('review_status=pending')
})

test('shops create and select a shop without altering the current one until selected', async () => {
  let rows = []
  const fetch = mockApi((path, options) => {
    if (options.method === 'POST') { rows = [shop, { ...shop, shop_id: 'other', display_name: 'Other shop' }]; return {} }
    return rows
  })
  const { router } = mount(<Shops />)
  await screen.findByText('Bắt đầu với cửa hàng đầu tiên')
  await userEvent.click(screen.getByRole('button', { name: '＋ Tạo cửa hàng' }))
  await userEvent.type(screen.getByLabelText('Tên cửa hàng'), ' New shop ')
  await userEvent.click(screen.getByRole('button', { name: 'Lưu' }))
  await screen.findByText('Other shop')
  expect(JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'POST')[1].body)).toEqual({ display_name: 'New shop' })
  await userEvent.click(screen.getByRole('button', { name: 'Chọn cửa hàng →' }))
  expect(currentShop.get().shop_id).toBe('other')
  expect(router.state.location.pathname).toBe('/seller/dashboard')
})

test('current shop can be reopened', async () => {
  mockApi(() => [shop])
  const { router } = mount(<Shops />)
  await userEvent.click(await screen.findByRole('button', { name: 'Quản lý cửa hàng →' }))
  expect(router.state.location.pathname).toBe('/seller/dashboard')
})

test('product creation normalizes money and currency; edit supports unlimited stock', async () => {
  let rows = []
  const fetch = mockApi((path, options) => {
    if (path.includes('/categories')) return [{ category_code: 'food', category_name_en: 'Food' }, { category_code: 'other', category_name_en: null }]
    if (options.method !== 'GET') { rows = [listing]; return {} }
    return page(rows)
  })
  mount(<Products />)
  await screen.findByText('Cửa hàng chưa có sản phẩm')
  await userEvent.click(screen.getByRole('button', { name: '＋ Thêm sản phẩm' }))
  await userEvent.type(screen.getByLabelText('Tên sản phẩm'), ' Coffee ')
  await userEvent.type(screen.getByLabelText('Giá'), '12.5')
  await userEvent.type(screen.getByLabelText('Tồn kho (để trống nếu không giới hạn)'), '5')
  await userEvent.click(screen.getByRole('combobox', { name: 'Danh mục' }))
  await userEvent.click(screen.getByRole('option', { name: 'Food' }))
  await userEvent.click(screen.getByRole('button', { name: 'Lưu' }))
  await screen.findByText('Coffee')
  expect(JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'POST')[1].body)).toEqual({ product_name: 'Coffee', description: '', image_url: '', category_code: 'food', current_price: '12.50', currency: 'VND', stock_quantity: 5 })
  await userEvent.click(screen.getByRole('button', { name: 'Sửa' }))
  await userEvent.clear(screen.getByLabelText('Tồn kho'))
  await userEvent.click(screen.getByRole('button', { name: 'Lưu' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'PATCH')[1].body)).toEqual({ product_name: 'Coffee', description: '', image_url: '', current_price: '12.50' })
})

test('products display nullable imported fields and create uncategorized listings', async () => {
  const fetch = mockApi((path, options) => path.includes('categories') ? [] : options.method !== 'GET' ? {} : page([{ ...listing, category_name_en: null, current_price: null, stock_quantity: null }]))
  mount(<Products />)
  await screen.findByText('Không giới hạn')
  await userEvent.click(screen.getByRole('button', { name: 'Sửa' }))
  expect(screen.getByLabelText('Giá (VND)').value).toBe('')
  await userEvent.click(screen.getByRole('button', { name: 'Hủy' }))
  await userEvent.click(screen.getByRole('button', { name: '＋ Thêm sản phẩm' }))
  await userEvent.type(screen.getByLabelText('Tên sản phẩm'), 'Book')
  await userEvent.type(screen.getByLabelText('Giá'), '1')
  await userEvent.click(screen.getByRole('button', { name: 'Lưu' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'POST')[1].body).category_code).toBeUndefined()
})

test('seller order filters reset pagination, clear and search using encoded IDs', async () => {
  const fetch = mockApi((path) => path.includes('?') ? page([order, { ...order, order_id: 'unscored', score: null, risk_level: null, is_synthetic: false }]) : order)
  const { router } = mount(<Orders />, '/seller/orders?page=2&order_id=old')
  await screen.findByRole('link', { name: 'order:1' })
  await userEvent.click(screen.getByRole('button', { name: 'Xóa lọc mã' }))
  expect(router.state.location.search).toBe('')
  await userEvent.click(screen.getByRole('combobox', { name: 'Nguồn dữ liệu' }))
  await userEvent.click(screen.getByRole('option', { name: 'Olist VN mô phỏng' }))
  expect(router.state.location.search).toBe('?batch_id=olist_vn_sim')
  await userEvent.click(screen.getByRole('combobox', { name: 'Mức rủi ro' }))
  await userEvent.click(screen.getByRole('option', { name: 'Cao' }))
  expect(router.state.location.search).toContain('risk_level=high')
  await userEvent.click(screen.getByRole('button', { name: '↻ Làm mới' }))
  const input = screen.getByLabelText('Tìm chính xác mã đơn hàng')
  await userEvent.clear(input)
  await userEvent.click(screen.getByRole('button', { name: 'Tìm đơn →' }))
  expect(router.state.location.pathname).toBe('/seller/orders')
  await userEvent.type(input, 'order:1')
  await userEvent.click(screen.getByRole('button', { name: 'Tìm đơn →' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/seller/orders/order%3A1'))
  expect(fetch.mock.calls.some(([path]) => path.endsWith('/orders/order%3A1'))).toBe(true)
})

test('seller order empty results allow reset and missing order reports API failure', async () => {
  mockApi((path) => path.includes('?') ? page() : fail(404, 'Order missing'))
  const { router } = mount(<Orders />, '/seller/orders?risk_level=high')
  await userEvent.click(await screen.findByRole('button', { name: 'Xóa bộ lọc' }))
  expect(router.state.location.search).toBe('')
  await userEvent.type(screen.getByLabelText('Tìm chính xác mã đơn hàng'), 'unknown')
  await userEvent.click(screen.getByRole('button', { name: 'Tìm đơn →' }))
  expect(await screen.findByText('Order missing')).toBeTruthy()
})

test.each(['llm', 'template'])('seller reviews evidence with %s explanation, records a decision, and updates fulfillment', async (source) => {
  let data = structuredClone(order)
  const fetch = mockApi((path, options) => {
    if (path.endsWith('/network')) return network
    if (path.endsWith('/explain')) return { assessment_id: assessment.assessment_id, source, fallback_reason: source === 'template' ? 'provider_timeout' : null, sentences: ['Known contribution only.'] }
    if (path.endsWith('/decision')) { data.review_status = 'released'; return {} }
    if (path.endsWith('/fulfillment')) { data.fulfillment_status = JSON.parse(options.body).status; return {} }
    if (options.method === 'POST') return {}
    return data
  })
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } })
  mount(<OrderDetail />, '/seller/orders/order:1', '/seller/orders/:orderId')
  await screen.findByRole('heading', { level: 1 })
  await userEvent.click(screen.getByRole('button', { name: 'Sao chép mã' }))
  expect(navigator.clipboard.writeText).toHaveBeenCalledWith('order:1')
  await userEvent.click(screen.getByRole('button', { name: 'Đóng thông báo' }))
  await userEvent.click(screen.getByRole('button', { name: '↻ Làm mới' }))
  await userEvent.click(screen.getByRole('button', { name: 'Giải thích đánh giá' }))
  expect(await screen.findByText('Known contribution only.')).toBeTruthy()
  expect(screen.getByText(source === 'llm' ? 'Diễn giải bằng AI' : 'AI phản hồi quá thời gian chờ. Bạn có thể thử lại; điểm KBS vẫn giữ nguyên.')).toBeTruthy()
  await userEvent.click(screen.getByText('Xem bằng chứng và cách tính điểm'))
  await userEvent.click(screen.getByRole('button', { name: 'Chấm lại' }))
  await waitFor(() => expect(screen.queryByText('Known contribution only.')).toBeNull())
  await userEvent.click(screen.getByRole('button', { name: 'Release · Cho xử lý' }))
  await userEvent.type(screen.getByLabelText('Lý do quyết định'), '   ')
  fireEvent.submit(screen.getByLabelText('Lý do quyết định').closest('form'))
  expect(await screen.findByText('Vui lòng nhập lý do quyết định.')).toBeTruthy()
  await userEvent.clear(screen.getByLabelText('Lý do quyết định'))
  await userEvent.type(screen.getByLabelText('Lý do quyết định'), ' Reviewed facts ')
  await userEvent.click(screen.getByRole('button', { name: 'Xác nhận quyết định' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(JSON.parse(fetch.mock.calls.find(([path]) => path.endsWith('/decision'))[1].body)).toEqual({ assessment_id: assessment.assessment_id, action: 'release', reason: 'Reviewed facts' })
  await userEvent.click(screen.getByRole('tab', { name: 'Đơn hàng' }))
  expect(await screen.findByText('Coffee')).toBeTruthy()
  await userEvent.click(screen.getByRole('tab', { name: 'Giao hàng' }))
  expect(screen.getByText('Đơn đã đủ điều kiện đóng gói.')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Đang đóng gói' }))
  await screen.findByRole('button', { name: 'Đã gửi hàng' })
  await userEvent.click(screen.getByRole('button', { name: 'Đã gửi hàng' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Đã giao' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Đã trả hàng' }))
  await screen.findByText('Đã cập nhật trạng thái giao hàng.')
  await userEvent.click(screen.getByRole('tab', { name: 'Lịch sử' }))
  expect(screen.getByText('Chưa có quyết định review.')).toBeTruthy()
  expect(screen.getByText('Chưa có sự kiện.')).toBeTruthy()
  await userEvent.click(screen.getByRole('tab', { name: 'Tranh chấp (0)' }))
  expect(screen.getByText('Đơn này chưa có hồ sơ tranh chấp.')).toBeTruthy()
})

test('seller errors preserve state, conflict refreshes assessment, cancel remains explicit', async () => {
  let conflict = true
  const fetch = mockApi((path, options) => {
    if (path.endsWith('/network')) return network
    if (options.method === 'GET') return { ...order, review_status: 'hold', payment: { method: 'credit_card', status: 'pending' }, address: { ...order.address, parse_ok: null, ward_code: null, house_no: null } }
    if (path.endsWith('/decision')) return conflict ? fail(409, 'Assessment changed') : {}
    if (path.endsWith('/fulfillment')) return conflict ? fail(400, 'Cannot cancel') : {}
    return fail(503, 'Service unavailable')
  })
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
  mount(<OrderDetail />, '/seller/orders/order:1', '/seller/orders/:orderId')
  await screen.findByRole('heading', { level: 1 })
  await userEvent.click(screen.getByRole('button', { name: 'Sao chép mã' }))
  await screen.findByText(/Không thể sao chép/)
  await userEvent.click(screen.getByText('Xem bằng chứng và cách tính điểm'))
  await userEvent.click(screen.getByRole('button', { name: 'Chấm lại' }))
  await screen.findByText('Service unavailable')
  await userEvent.click(screen.getByRole('button', { name: 'Giải thích đánh giá' }))
  await screen.findByText('Service unavailable')
  await userEvent.click(screen.getByRole('button', { name: 'Keep Hold' }))
  await userEvent.type(screen.getByLabelText('Lý do quyết định'), 'Need more evidence')
  await userEvent.click(screen.getByRole('button', { name: 'Xác nhận quyết định' }))
  await screen.findByText('Assessment changed')
  expect(screen.getByLabelText('Lý do quyết định').value).toBe('Need more evidence')
  expect(screen.getByText(/Đơn hoặc assessment đã thay đổi/)).toBeTruthy()
  conflict = false
  await userEvent.click(screen.getByRole('button', { name: 'Xác nhận quyết định' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await userEvent.click(screen.getByRole('tab', { name: 'Giao hàng' }))
  expect(screen.getByText('Đơn cần được Release trước khi đóng gói.')).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Đang đóng gói' }).disabled).toBe(true)
  await userEvent.click(screen.getByRole('button', { name: 'Hủy đơn' }))
  await userEvent.click(screen.getByRole('button', { name: 'Quay lại' }))
  await userEvent.click(screen.getByRole('button', { name: 'Hủy đơn' }))
  conflict = true
  await userEvent.click(screen.getByRole('button', { name: 'Xác nhận hủy' }))
  expect(await within(screen.getByRole('alertdialog')).findByText('Cannot cancel')).toBeTruthy()
  conflict = false
  await userEvent.click(screen.getByRole('button', { name: 'Xác nhận hủy' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(fetch.mock.calls.filter(([path]) => path.endsWith('/fulfillment'))).toHaveLength(2)
})

test.each(['unassessed', 'suspended', 'unpaid', 'paid', 'historical'])('seller detail handles %s without unsafe actions', async (scenario) => {
  if (scenario === 'suspended') profiles.set({ buyer: null, seller: { ...seller, status: 'suspended' } })
  const data = { ...order, review_status: 'released', latest_assessment: scenario === 'unassessed' ? null : { ...assessment, reasons: [], rule_config: {} },
    batch_id: scenario === 'historical' ? 'olist_raw' : 'olist_vn_sim', is_synthetic: false,
    payment: { method: 'credit_card', status: scenario === 'paid' ? 'succeeded' : 'pending' },
    address: { ...order.address, parse_ok: false },
    events: [{ event_type: 'created', actor_type: 'buyer', reason: 'Created by buyer', occurred_at: '2026-01-01', available_at: null }, { event_type: 'confirmed', actor_type: 'seller', reason: null, occurred_at: '2026-01-02', available_at: '2026-01-02' }],
    decisions: [{ decision_id: 'd1', assessment_id: 'old', action: 'release', actor_type: 'seller', reason: 'Reviewed', created_at: '2026-01-01' }],
    cases: [{ case_id: 'case', case_type: 'delivery', opened_at: '2026-01-01', messages: [{ speaker_role: 'buyer', body: 'Not received', occurred_at: '2026-01-01' }], evidence: [], resolutions: [
      { verification_status: 'verified', responsible_party: 'carrier', rationale: 'Carrier error', decided_at: '2026-01-02', available_at: '2026-01-02' },
      { verification_status: 'pending', responsible_party: 'unknown', rationale: 'Investigating', decided_at: '2026-01-01', available_at: '2026-01-01' },
    ] }],
  }
  mockApi((path, options) => path.endsWith('/network') ? network : options.method === 'POST' ? {} : data)
  mount(<OrderDetail />, '/seller/orders/order:1', '/seller/orders/:orderId')
  await screen.findByRole('heading', { level: 1 })
  if (scenario === 'unassessed') {
    expect(screen.getByRole('button', { name: 'Release · Cho xử lý' }).disabled).toBe(true)
    await userEvent.click(screen.getByText('Xem bằng chứng và cách tính điểm'))
    await userEvent.click(screen.getByRole('button', { name: 'Chấm điểm' }))
  }
  if (scenario === 'historical') expect(screen.queryByRole('button', { name: 'Chấm lại' })).toBeNull()
  await userEvent.click(screen.getByRole('tab', { name: 'Giao hàng' }))
  expect(screen.getByRole('button', { name: 'Đang đóng gói' }).disabled).toBe(scenario !== 'paid')
  if (scenario === 'unpaid') expect(screen.getByText('Cần thanh toán thành công trước khi đóng gói.')).toBeTruthy()
  await userEvent.click(screen.getByRole('tab', { name: 'Lịch sử' }))
  expect(screen.getByText('Reviewed')).toBeTruthy()
  expect(screen.getByText(/Assessment cũ/)).toBeTruthy()
  expect(screen.getByText('Created by buyer')).toBeTruthy()
  await userEvent.click(screen.getByRole('tab', { name: 'Tranh chấp (1)' }))
  expect(screen.getByText(/Not received/)).toBeTruthy()
  expect(screen.getByText('Carrier error')).toBeTruthy()
})
