import { test, expect } from 'vitest'
import { screen, waitFor, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Catalog from '../src/pages/Catalog'
import Login from '../src/pages/Login'
import Checkout from '../src/pages/buyer/Checkout'
import Addresses from '../src/pages/buyer/Addresses'
import Orders from '../src/pages/buyer/Orders'
import OrderDetail from '../src/pages/buyer/OrderDetail'
import { tokens, profiles } from '../src/api'
import { cart } from '../src/cart'
import { listing, address, order, buyer, seller, page, mockApi, mount, fail } from './helpers'

test('catalog loads priced, missing-price and out-of-stock listings and adds stock-limited cart items', async () => {
  mockApi((path) => path.includes('categories') ? [{ category_code: 'food', category_name_en: 'Food' }, { category_code: 'other', category_name_en: null }] : page([
    listing, { ...listing, listing_id: 'no-price', product_name: 'Unknown', current_price: null, stock_quantity: null },
    { ...listing, listing_id: 'sold-out', product_name: null, category_name_en: null, stock_quantity: 0 },
  ], { total_pages: 2 }))
  const { router } = mount(<Catalog />)
  const add = await screen.findByRole('button', { name: 'Thêm vào giỏ' })
  await userEvent.click(add)
  expect(cart.get()[0].quantity).toBe(1)
  expect(screen.getByRole('button', { name: 'Thêm vào giỏ (1)' })).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Chưa có giá' }).disabled).toBe(true)
  expect(screen.getByRole('button', { name: 'Hết hàng' }).disabled).toBe(true)
  await userEvent.click(screen.getByRole('button', { name: 'Sau' }))
  expect(router.state.location.search).toBe('?page=2')
  await userEvent.click(screen.getByRole('combobox', { name: 'Danh mục' }))
  await userEvent.click(screen.getByRole('option', { name: 'Food' }))
  expect(router.state.location.search).toBe('?category=food')
  await userEvent.click(screen.getByRole('combobox', { name: 'Danh mục' }))
  await userEvent.click(screen.getByRole('option', { name: 'Tất cả danh mục' }))
  expect(router.state.location.search).toBe('')
})

test('shop catalog shows its title and empty state', async () => {
  mockApi((path) => path.includes('categories') ? [] : path.includes('/shops/') ? { display_name: 'My shop', listing_count: 0 } : page())
  mount(<Catalog />, '/shops/shop:1', '/shops/:shopId')
  expect(await screen.findByRole('heading', { name: 'My shop' })).toBeTruthy()
  expect(await screen.findByText('Không có sản phẩm.')).toBeTruthy()
})

test.each([
  [{ buyer, seller: null }, '/buyer/orders'],
  [{ buyer: null, seller }, '/seller/shops'],
])('login routes a single-role profile and clears the previous session', async (profile, destination) => {
  tokens.set({ access_token: 'old' })
  const fetch = mockApi((path) => path === '/api/auth/login' ? { access_token: 'new', refresh_token: 'new-refresh' } : profile)
  const { router } = mount(<Login />, '/login')
  await userEvent.type(screen.getByLabelText('Tên đăng nhập'), 'user')
  await userEvent.type(screen.getByLabelText('Mật khẩu'), 'test-password')
  await userEvent.click(screen.getByRole('button', { name: 'Đăng nhập' }))
  await waitFor(() => expect(router.state.location.pathname).toBe(destination))
  expect(fetch.mock.calls[0][1].headers.Authorization).toBeUndefined()
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ username: 'user', password: 'test-password' })
  expect(tokens.get().access_token).toBe('new')
  expect(profiles.get()).toEqual(profile)
})

test.each(['buyer', 'seller'])('dual-role login lets the user choose %s portal', async (role) => {
  mockApi((path) => path === '/api/auth/login' ? { access_token: 'new' } : { buyer, seller })
  const { router } = mount(<Login />, '/login')
  fireEvent.submit(screen.getByRole('button', { name: 'Đăng nhập' }).closest('form'))
  await screen.findByRole('heading', { name: 'Chọn không gian làm việc' })
  await userEvent.click(screen.getByRole('button', { name: role === 'buyer' ? /Người mua/ : /Người bán/ }))
  expect(router.state.location.pathname).toBe(role === 'buyer' ? '/buyer/orders' : '/seller/shops')
})

test.each(['missing-profile', 'unauthorized'])('login displays actionable failure: %s', async (scenario) => {
  mockApi((path) => scenario === 'unauthorized' ? fail(401, 'Invalid credentials') : path === '/api/auth/login' ? { access_token: 'new' } : { buyer: null, seller: null })
  mount(<Login />, '/login')
  fireEvent.submit(screen.getByRole('button', { name: 'Đăng nhập' }).closest('form'))
  expect((await screen.findByRole('alert')).textContent).toMatch(scenario === 'unauthorized' ? /Invalid credentials/ : /chưa có hồ sơ/)
})

test('checkout adjusts quantities, changes address and submits server-priced order', async () => {
  cart.set([{ listing, quantity: 1 }])
  const fetch = mockApi((path) => path.endsWith('/addresses') ? [address, { ...address, saved_address_id: 'office', label: 'Office', is_default: false }] : {
    checkout_id: 'checkout-new', total_amount: '40.00', currency: 'VND', orders: [{ order_id: 'new:1', shop_id: listing.shop_id, shop_name: 'Test shop', order_total: '40.00' }],
  })
  mount(<Checkout />)
  await screen.findByText('Office')
  expect(screen.getByRole('button', { name: 'Đặt hàng' }).disabled).toBe(true)
  await userEvent.type(screen.getByLabelText('Số điện thoại nhận hàng'), '0900000001')
  await userEvent.click(screen.getByRole('button', { name: 'Tăng số lượng Coffee' }))
  expect(cart.get()[0].quantity).toBe(2)
  await userEvent.click(screen.getByRole('button', { name: 'Giảm số lượng Coffee' }))
  fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '3' } })
  expect(cart.get()[0].quantity).toBe(3)
  await userEvent.click(screen.getAllByRole('radio')[1])
  await userEvent.click(screen.getByRole('combobox', { name: 'Phương thức thanh toán' }))
  await userEvent.click(screen.getByRole('option', { name: 'Thẻ tín dụng' }))
  await userEvent.click(screen.getByRole('button', { name: 'Đặt hàng' }))
  expect(await screen.findByRole('heading', { name: 'Đặt hàng thành công' })).toBeTruthy()
  const request = fetch.mock.calls.find(([path]) => path.endsWith('/checkouts'))
  expect(JSON.parse(request[1].body)).toEqual({ saved_address_id: 'office', payment_method: 'credit_card', phone: '0900000001', items: [{ listing_id: listing.listing_id, quantity: 3 }] })
  expect(cart.get()).toEqual([])
  expect(screen.getByRole('link').getAttribute('href')).toBe('/buyer/orders/new%3A1')
})

test('checkout preserves cart after API failure and allows removal', async () => {
  cart.set([{ listing: { ...listing, product_name: null }, quantity: 1 }])
  mockApi((path) => path.endsWith('/addresses') ? [{ ...address, is_default: false }] : fail(409, 'Stock changed'))
  mount(<Checkout />)
  await screen.findByText('Home')
  await userEvent.type(screen.getByLabelText('Số điện thoại nhận hàng'), '0900000001')
  fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '' } })
  await userEvent.click(screen.getByRole('button', { name: 'Đặt hàng' }))
  expect(await screen.findByText('Stock changed')).toBeTruthy()
  expect(cart.get()).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: 'Xóa' }))
  expect(screen.getByText(/Giỏ hàng trống/)).toBeTruthy()
})

test('checkout prevents mixed-currency or missing-address submission', async () => {
  cart.set([{ listing, quantity: 1 }, { listing: { ...listing, listing_id: 'usd', shop_id: 'other', currency: 'USD', current_price: null }, quantity: 1 }])
  mockApi(() => [])
  mount(<Checkout />)
  expect(await screen.findByText(/Chưa có địa chỉ\./)).toBeTruthy()
  expect(screen.getByRole('alert').textContent).toMatch(/nhiều loại tiền tệ/)
  expect(screen.getByRole('button', { name: 'Đặt hàng' }).disabled).toBe(true)
})

test('addresses create, update, choose default and delete through the API', async () => {
  let rows = []
  const fetch = mockApi((path, options) => {
    if (options.method === 'GET') return rows
    if (options.method === 'DELETE') rows = []
    else rows = [{ ...address, ...JSON.parse(options.body), is_default: false }]
    return {}
  })
  mount(<Addresses />)
  await screen.findByText('Chưa có địa chỉ nào.')
  await userEvent.click(screen.getByRole('button', { name: 'Thêm địa chỉ' }))
  await userEvent.type(screen.getByLabelText('Tên gợi nhớ'), ' Home ')
  await userEvent.type(screen.getByLabelText('Địa chỉ'), '12 Main Street')
  await userEvent.type(screen.getByLabelText('Tỉnh/bang'), 'City')
  await userEvent.clear(screen.getByLabelText('Mã quốc gia (2 ký tự)'))
  await userEvent.type(screen.getByLabelText('Mã quốc gia (2 ký tự)'), 'vn')
  await userEvent.click(screen.getByRole('checkbox'))
  await userEvent.click(screen.getByRole('button', { name: 'Lưu' }))
  await screen.findByText('Home')
  expect(JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'POST')[1].body)).toEqual({ label: 'Home', address_raw: '12 Main Street', region: 'City', country_code: 'VN', is_default: true })
  await userEvent.click(screen.getByRole('button', { name: 'Sửa' }))
  await userEvent.clear(screen.getByLabelText('Tên gợi nhớ'))
  await userEvent.type(screen.getByLabelText('Tên gợi nhớ'), 'Office')
  await userEvent.click(screen.getByRole('button', { name: 'Lưu' }))
  await screen.findByText('Office')
  await userEvent.click(screen.getByRole('button', { name: 'Đặt mặc định' }))
  expect(fetch.mock.calls.filter(([, options]) => options.method === 'PUT')).toHaveLength(2)
  await userEvent.click(screen.getByRole('button', { name: 'Xóa' }))
  await userEvent.click(screen.getByRole('button', { name: 'Quay lại' }))
  await userEvent.click(screen.getByRole('button', { name: 'Xóa' }))
  await userEvent.click(screen.getByRole('button', { name: 'Xóa địa chỉ' }))
  await screen.findByText('Chưa có địa chỉ nào.')
})

test('address mutation errors retain the entry and remain visible', async () => {
  mockApi((path, options) => options.method === 'GET' ? [{ ...address, is_default: false }] : fail(403, 'Read only'))
  mount(<Addresses />)
  await screen.findByText('Home')
  await userEvent.click(screen.getByRole('button', { name: 'Đặt mặc định' }))
  await screen.findByText('Read only')
  await userEvent.click(screen.getByRole('button', { name: 'Xóa' }))
  await userEvent.click(screen.getByRole('button', { name: 'Xóa địa chỉ' }))
  expect(await within(screen.getByRole('alertdialog')).findByText('Read only')).toBeTruthy()
  expect(screen.getByRole('alertdialog')).toBeTruthy()
})

test.each([[[]], [[order]]])('buyer order list supports empty and populated pages', async (rows) => {
  mockApi(() => page(rows))
  mount(<Orders />)
  if (rows.length) expect(await screen.findByRole('link', { name: /Test shop/ })).toBeTruthy()
  else expect(await screen.findByText('Chưa có đơn hàng.')).toBeTruthy()
})

test('buyer can cancel an unfulfilled order and sees its neutral status and event history', async () => {
  let result = structuredClone(order)
  const fetch = mockApi((path, options) => {
    if (options.method === 'POST') {
      result = { ...result, fulfillment_status: 'canceled', display_status: 'canceled', events: [{ event_type: 'canceled', occurred_at: '2026-01-02' }] }
      return {}
    }
    return result
  })
  mount(<OrderDetail />, '/buyer/orders/order:1', '/buyer/orders/:orderId')
  await screen.findByText('Chưa có sự kiện.')
  await userEvent.click(screen.getByRole('button', { name: 'Hủy đơn' }))
  await userEvent.type(screen.getByLabelText('Lý do hủy'), ' Ordered twice ')
  await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Hủy đơn' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(fetch.mock.calls.find(([, options]) => options.method === 'POST')[1].body).toBe('{"reason":"Ordered twice"}')
  expect(screen.queryByRole('button', { name: 'Hủy đơn' })).toBeNull()
  expect(screen.queryByText(/65\/100/)).toBeNull()
  expect(screen.getAllByText('Đã hủy').length).toBeGreaterThan(0)
})
