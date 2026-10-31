import { test, expect } from 'vitest'
import { screen, waitFor, fireEvent, render, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ProductDetail from '../src/pages/ProductDetail'
import Shops from '../src/pages/Shops'
import Catalog from '../src/pages/Catalog'
import Login from '../src/pages/Login'
import Checkout from '../src/pages/buyer/Checkout'
import { ProductPlaceholder } from '../src/ui'
import { sampleProductImage } from '../src/product-images'
import { existsSync } from 'node:fs'
import { cart } from '../src/cart'
import { listing, shop, address, buyer, seller, page, mockApi, mount } from './helpers'

test('product quantity is integral and respects stock already in the cart', async () => {
  cart.set([{ listing, quantity: 2 }])
  mockApi(() => ({ ...listing, description: 'Fresh coffee', image_url: 'https://example.test/coffee.jpg' }))
  mount(<ProductDetail />, '/products/listing:1', '/products/:listingId')
  await screen.findByText('Fresh coffee')
  const qty = screen.getByRole('spinbutton', { name: 'Số lượng' })
  await userEvent.click(screen.getByRole('button', { name: 'Tăng số lượng' }))
  expect(qty.value).toBe('2')
  await userEvent.click(screen.getByRole('button', { name: 'Giảm số lượng' }))
  fireEvent.change(qty, { target: { value: '' } })
  expect(qty.value).toBe('1')
  fireEvent.change(qty, { target: { value: '3.9' } })
  expect(qty.value).toBe('3')
  await userEvent.click(screen.getByRole('button', { name: '＋ Thêm vào giỏ hàng' }))
  expect(cart.get()[0].quantity).toBe(5)
  expect(screen.getByText('Đã đạt số lượng tối đa trong giỏ hàng.')).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Mua ngay →' }).disabled).toBe(true)
})

test('buy now preserves other products and selects the requested listing by URL', async () => {
  cart.set([{ listing: { ...listing, listing_id: 'other' }, quantity: 1 }])
  mockApi(() => ({ ...listing, category_name_en: null, stock_quantity: null }))
  const { router } = mount(<ProductDetail />, '/products/listing:1', '/products/:listingId')
  await screen.findByText('Chưa cập nhật')
  expect(screen.getByText('Cửa hàng chưa bổ sung mô tả cho sản phẩm này.')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Mua ngay →' }))
  expect(router.state.location.pathname).toBe('/buyer/checkout')
  expect(new URLSearchParams(router.state.location.search).get('only')).toBe(listing.listing_id)
  expect(cart.get().map(l => l.listing.listing_id)).toEqual(['other', listing.listing_id])
})

test('sold-out detail prevents purchase', async () => {
  mockApi(() => ({ ...listing, stock_quantity: 0 }))
  mount(<ProductDetail />, '/products/sold', '/products/:listingId')
  await screen.findByText('Sản phẩm tạm hết hàng.')
  expect(screen.getByRole('button', { name: '＋ Thêm vào giỏ hàng' }).disabled).toBe(true)
})

test('product images reject insecure URLs and recover after a failed image is replaced', () => {
  const { rerender } = render(<ProductPlaceholder productId="test-product" src="http://example.test/image" />)
  expect(screen.getByRole('img').getAttribute('src')).toBe(sampleProductImage('test-product'))
  rerender(<ProductPlaceholder productId="test-product" src="https://example.test/image" alt="Coffee" />)
  fireEvent.error(screen.getByRole('img', { name: 'Coffee' }))
  expect(screen.getByRole('img').getAttribute('src')).toBe(sampleProductImage('test-product'))
  fireEvent.error(screen.getByRole('img'))
  expect(screen.queryByRole('img')).toBeNull()
  rerender(<ProductPlaceholder productId="test-product" src="https://example.test/replacement" />)
  expect(screen.getByRole('img').getAttribute('src')).toBe('https://example.test/replacement')
})

test.each([true, false])('shop directory supports links, pagination and empty results: %s', async populated => {
  const fetch = mockApi(() => page(populated ? [{ ...shop, listing_count: 3 }] : [], { total_pages: populated ? 2 : 1 }))
  const { router } = mount(<Shops />, '/shops')
  if (populated) {
    expect((await screen.findByRole('link', { name: /Test shop/ })).getAttribute('href')).toBe('/shops/shop%3A1')
    await userEvent.click(screen.getByRole('button', { name: 'Sau' }))
    expect(router.state.location.search).toBe('?page=2')
    await waitFor(() => expect(fetch.mock.calls.at(-1)[0]).toContain('page=2'))
  } else expect(await screen.findByText('Chưa có cửa hàng nào.')).toBeTruthy()
})

test('catalog preserves search filters and only sorts prices within a selected currency', async () => {
  const fetch = mockApi(path => path.includes('categories') ? [{ category_code: 'food', category_name_en: 'Food' }] : page([listing]))
  const { router } = mount(<Catalog />, '/?q=coffee&category=food&page=2')
  await screen.findByRole('heading', { name: 'Kết quả cho “coffee”' })
  await userEvent.click(screen.getByRole('combobox', { name: 'Tiền tệ' }))
  await userEvent.click(screen.getByRole('option', { name: 'Việt Nam · VND' }))
  await userEvent.click(screen.getByRole('combobox', { name: 'Sắp xếp theo' }))
  await userEvent.click(screen.getByRole('option', { name: 'Giá: thấp đến cao' }))
  await userEvent.click(screen.getByRole('checkbox', { name: 'Chỉ sản phẩm còn hàng' }))
  expect(router.state.location.search).toContain('q=coffee')
  expect(router.state.location.search).toContain('category=food')
  expect(router.state.location.search).not.toContain('page=')
  await waitFor(() => expect(fetch.mock.calls.at(-1)[0]).toContain('sort=price_asc'))
  await userEvent.click(screen.getByRole('checkbox', { name: 'Chỉ sản phẩm còn hàng' }))
  await userEvent.click(screen.getByRole('combobox', { name: 'Tiền tệ' }))
  await userEvent.click(screen.getByRole('option', { name: 'Tất cả tiền tệ' }))
  expect(router.state.location.search).not.toContain('sort=')
  await userEvent.click(screen.getByRole('button', { name: 'Xóa tất cả bộ lọc' }))
  expect(router.state.location.search).toBe('')
})

test('checkout sends only selected items and retains unpurchased products', async () => {
  const other = { ...listing, listing_id: 'other', product_name: 'Tea', currency: 'BRL' }
  cart.set([{ listing, quantity: 2 }, { listing: other, quantity: 1 }])
  const fetch = mockApi(path => path.endsWith('/addresses') ? [address] : { checkout_id: 'new', currency: 'VND', total_amount: '25', orders: [] })
  mount(<Checkout />, '/buyer/checkout?only=listing%3A1')
  await screen.findByRole('radio')
  expect(screen.getByRole('checkbox', { name: 'Chọn Tea' }).checked).toBe(false)
  await userEvent.click(screen.getByRole('checkbox', { name: /Chọn tất cả/ }))
  expect(screen.getByRole('alert').textContent).toMatch(/nhiều loại tiền tệ/)
  await userEvent.click(screen.getByRole('checkbox', { name: /Chọn tất cả/ }))
  expect(screen.getByRole('button', { name: 'Đặt hàng (0)' }).disabled).toBe(true)
  await userEvent.click(screen.getByRole('checkbox', { name: 'Chọn Coffee' }))
  await userEvent.type(screen.getByLabelText('Số điện thoại nhận hàng'), '0901234567')
  await userEvent.click(screen.getByRole('button', { name: 'Đặt hàng (1)' }))
  await screen.findByRole('heading', { name: 'Đặt hàng thành công' })
  expect(JSON.parse(fetch.mock.calls.find(([path]) => path.endsWith('/checkouts'))[1].body).items).toEqual([{ listing_id: listing.listing_id, quantity: 2 }])
  expect(cart.get()).toEqual([{ listing: other, quantity: 1 }])
})

test.each(['/buyer/checkout', '/buyer/checkout?only=listing%3A1'])(
  'checkout does not select products added by another tab: %s', async path => {
    const other = { ...listing, listing_id: 'other', product_name: 'Tea' }
    const incoming = { ...listing, listing_id: 'incoming', product_name: 'Cocoa' }
    cart.set([{ listing, quantity: 2 }, { listing: other, quantity: 1 }])
    const fetch = mockApi(url => url.endsWith('/addresses') ? [address] : {
      checkout_id: 'new', currency: 'VND', total_amount: '25', orders: [],
    })
    mount(<Checkout />, path)
    await screen.findByRole('radio')
    if (!path.includes('only=')) {
      await userEvent.click(screen.getByRole('checkbox', { name: 'Chọn Tea' }))
    }
    act(() => window.dispatchEvent(new StorageEvent('storage', {
      key: 'cart', newValue: JSON.stringify([...cart.get(), { listing: incoming, quantity: 1 }]),
    })))
    expect(screen.getByRole('checkbox', { name: 'Chọn Cocoa' }).checked).toBe(false)
    expect(screen.getByRole('checkbox', { name: /Chọn tất cả/ }).checked).toBe(false)
    await userEvent.type(screen.getByLabelText('Số điện thoại nhận hàng'), '0901234567')
    await userEvent.click(screen.getByRole('button', { name: 'Đặt hàng (1)' }))
    await screen.findByRole('heading', { name: 'Đặt hàng thành công' })
    expect(JSON.parse(fetch.mock.calls.find(([url]) => url.endsWith('/checkouts'))[1].body).items)
      .toEqual([{ listing_id: listing.listing_id, quantity: 2 }])
    expect(cart.get()).toEqual([{ listing: other, quantity: 1 }, { listing: incoming, quantity: 1 }])
  },
)

test.each([
  [{ buyer, seller }, '/buyer/checkout?only=listing%3A1', '/buyer/checkout'],
  [{ buyer: null, seller }, '/seller/orders', '/seller/orders'],
  [{ buyer, seller: null }, '//evil.test', '/buyer/orders'],
])('login honors authorized local destinations only', async (profile, from, expected) => {
  mockApi(path => path.endsWith('/login') ? { access_token: 'new' } : profile)
  const { router } = mount(<Login />, '/login')
  await act(() => router.navigate('/login', { state: { from } }))
  fireEvent.submit(screen.getByRole('button', { name: 'Đăng nhập' }).closest('form'))
  await waitFor(() => expect(router.state.location.pathname).toBe(expected))
})

test('sample image selection uses all five local assets and stays stable across mounts', () => {
  expect(sampleProductImage('olist:product-1')).toBe('/images/products/sample-3.webp')
  expect(sampleProductImage('\u{1F600}')).not.toBe(sampleProductImage('\u{1F601}'))
  const paths = new Set(Array.from({ length: 100 }, (_, n) => sampleProductImage(`olist:product-${n}`)))
  expect(paths.size).toBe(5)
  for (const path of paths) {
    expect(path).toMatch(/^\/images\/products\/sample-[1-5]\.webp$/)
    expect(existsSync(`public${path}`)).toBe(true)
  }
  const first = render(<ProductPlaceholder productId="olist:product-1" />)
  const src = screen.getByRole('img').getAttribute('src')
  first.unmount()
  render(<ProductPlaceholder productId="olist:product-1" alt="Renamed product" />)
  expect(screen.getByRole('img').getAttribute('src')).toBe(src)
})

test('imported product titles and categories are readable while filters retain the source code', async () => {
  const imported = { ...listing, product_name: null, category_code: 'bed_bath_table', category_name_en: 'bed_bath_table', product_id: '012345abcdef', image_url: null }
  mockApi(path => path.includes('categories') ? [{ category_code: imported.category_code, category_name_en: imported.category_name_en }] : page([imported]))
  const { router } = mount(<Catalog />)
  await screen.findByRole('link', { name: 'Bed bath table', exact: true })
  expect(screen.queryByText(/012345abcdef/)).toBeNull()
  expect(screen.queryByText('bed_bath_table')).toBeNull()
  expect(screen.getByRole('img').getAttribute('src')).toBe(sampleProductImage(imported.product_id))
  await userEvent.click(screen.getByRole('button', { name: 'Bed bath table', exact: true }))
  expect(router.state.location.search).toBe('?category=bed_bath_table')
  await screen.findByRole('heading', { name: 'Bed bath table', exact: true })
})
