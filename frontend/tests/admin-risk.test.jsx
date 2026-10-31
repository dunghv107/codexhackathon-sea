import { test, expect, vi } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import AdminRisk from '../src/pages/admin/Risk'
import Login from '../src/pages/Login'
import { tokens } from '../src/api'
import { page, mockApi, mount, fail, buyer, seller } from './helpers'

async function select(label, option) {
  await userEvent.click(screen.getByRole('combobox', { name: label }))
  await userEvent.click(screen.getByRole('option', { name: option }))
}

test('admin risk redirects guests and hides evidence form on forbidden responses', async () => {
  const guest = mount(<AdminRisk />, '/admin/risk')
  await waitFor(() => expect(guest.router.state.location.pathname).toBe('/login'))
  guest.unmount()
  tokens.set({ access_token: 'access' })
  mockApi(() => fail(403, 'Không có quyền'))
  mount(<AdminRisk />, '/admin/risk')
  await screen.findByText('Không có quyền')
  expect(screen.queryByText('Ghi nhận dữ liệu kiểm chứng')).toBeNull()
})

test('platform observations filter, paginate, refresh and sign out', async () => {
  tokens.set({ access_token: 'access' })
  const fetch = mockApi(() => page([{
    assessment_id: 'private', order_id: 'order', shop_name: 'Shop cần kiểm tra', baseline_version: 'demo-v2',
    assessed_at: '2026-01-01', input_cutoff_at: '2026-01-01', signals: [{
      rule_id: 'SFD-A16', status: 'fired', description: 'Hồ sơ lỗi người bán tăng', action: 'platform_seller_review',
      evidence: [{ feature: 'seller_case_change', label: 'Tỷ lệ lỗi', observed: '0.5', observed_sample_count: 12 }],
    }],
  }], { total: 21, total_pages: 2 }))
  const { router } = mount(<AdminRisk />, '/admin/risk')
  await screen.findByText('Shop cần kiểm tra')
  expect(screen.getByText(/SFD-A16 ·/)).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Trang sau' }))
  await waitFor(() => expect(fetch.mock.calls.at(-1)[0]).toContain('page=2'))
  await userEvent.click(screen.getByRole('button', { name: 'Trang trước' }))
  await select('Trạng thái tín hiệu', 'Có tín hiệu')
  await waitFor(() => expect(fetch.mock.calls.at(-1)[0]).toContain('status=fired'))
  const count = fetch.mock.calls.length
  await userEvent.click(screen.getByRole('button', { name: 'Làm mới' }))
  await waitFor(() => expect(fetch.mock.calls.length).toBeGreaterThan(count))
  await userEvent.click(screen.getByRole('button', { name: 'Đăng xuất' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
})

test.each([
  ['Kết luận hồ sơ', 'case_verdict', { 'Mã đơn hàng': 'order', 'Căn cứ kết luận': 'Ticket verified' }, { order_id: 'order', outcome_code: 'legitimate', verification_status: 'unresolved' }],
  ['Liên kết danh tính đã kiểm chứng', 'identity_link', { 'Mã người mua thứ nhất': 'buyer-a', 'Mã người mua thứ hai': 'buyer-b' }, { buyer_id: 'buyer-a', related_buyer_id: 'buyer-b', edge_type: 'verified_document' }],
  ['Quyết định xử lý vi phạm', 'enforcement', { 'Mã người mua đã được xử lý': 'buyer-a' }, { buyer_id: 'buyer-a', action: 'restricted' }],
  ['Đối chiếu bằng chứng', 'evidence_review', { 'Mã bằng chứng trong hồ sơ': 'proof' }, { evidence_id: 'proof', content_base64: 'aGVsbG8=', verdict: 'unresolved' }],
])('admin submits %s with server-owned availability and retry-stable record ID', async (label, kind, fields, expected) => {
  tokens.set({ access_token: 'access' })
  let reject = true
  const bodies = []
  mockApi((path, options) => {
    if (options.method === 'POST') { bodies.push(JSON.parse(options.body)); return reject ? fail(400, 'Kiểm tra lại nguồn') : {} }
    return page()
  })
  mount(<AdminRisk />, '/admin/risk')
  await userEvent.click(await screen.findByText('Ghi nhận dữ liệu kiểm chứng'))
  await select('Loại dữ liệu', label)
  await userEvent.type(screen.getByLabelText('Nguồn kiểm chứng / mã hồ sơ đối chiếu'), 'ticket:1')
  fireEvent.change(screen.getByLabelText('Thời điểm sự việc hoặc kết luận'), { target: { value: '2026-01-01T10:00' } })
  for (const [name, value] of Object.entries(fields)) await userEvent.type(screen.getByLabelText(name), value)
  if (kind === 'evidence_review') {
    const file = new File(['hello'], 'proof.txt')
    await userEvent.upload(screen.getByLabelText(/Tệp nội dung/), file)
    // jsdom's native form validation/FormData do not recognize user-event's virtual FileList.
    const get = FormData.prototype.get
    vi.spyOn(FormData.prototype, 'get').mockImplementation(function(name) { return name === 'content_file' ? file : get.call(this, name) })
  }
  if (kind === 'case_verdict') await select('Loại hồ sơ', 'Hủy đơn')
  async function submit() {
    const button = screen.getByRole('button', { name: 'Lưu ghi nhận' })
    if (kind === 'evidence_review') fireEvent.submit(button.closest('form'))
    else await userEvent.click(button)
  }
  await submit()
  await screen.findByText('Kiểm tra lại nguồn')
  reject = false
  await submit()
  await screen.findByRole('status')
  expect(bodies).toHaveLength(2)
  expect(bodies[0].record_id).toBe(bodies[1].record_id)
  expect(bodies[1]).toMatchObject({ kind, source_ref: 'ticket:1', data: expected })
  expect(bodies[1]).not.toHaveProperty('available_at')
  expect(bodies[1].data).not.toHaveProperty('content_file')
  expect(bodies[1].data).not.toHaveProperty('source_ref')
})

test('oversized uploaded evidence is rejected before sending', async () => {
  tokens.set({ access_token: 'access' })
  const fetch = mockApi(() => page())
  mount(<AdminRisk />, '/admin/risk')
  await userEvent.click(await screen.findByText('Ghi nhận dữ liệu kiểm chứng'))
  await select('Loại dữ liệu', 'Đối chiếu bằng chứng')
  await userEvent.upload(screen.getByLabelText(/Tệp nội dung/), new File(['x'.repeat(600001)], 'large.txt'))
  fireEvent.submit(screen.getByRole('button', { name: 'Lưu ghi nhận' }).closest('form'))
  await screen.findByText('Chọn tệp bằng chứng tối đa 600 KB.')
  expect(fetch.mock.calls.some(([, options]) => options.method === 'POST')).toBe(false)
})

test.each([false, true])('admin login supports platform access with marketplace profiles=%s', async (hasProfiles) => {
  mockApi(path => path === '/api/auth/login' ? { access_token: 'admin-token', user: { role: 'ADMIN' } } :
    { buyer: hasProfiles ? buyer : null, seller: hasProfiles ? seller : null })
  const { router } = mount(<Login />, '/login')
  await userEvent.type(screen.getByLabelText('Tên đăng nhập'), 'admin')
  await userEvent.type(screen.getByLabelText('Mật khẩu'), 'password')
  await userEvent.click(screen.getByRole('button', { name: 'Đăng nhập' }))
  if (hasProfiles) await userEvent.click(await screen.findByRole('button', { name: 'Quản trị rủi ro sàn →' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/admin/risk'))
})
