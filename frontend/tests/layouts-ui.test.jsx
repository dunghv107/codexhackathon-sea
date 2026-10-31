import { test, expect, vi } from 'vitest'
import { screen, waitFor, fireEvent, render, act, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { RootLayout, BuyerLayout, SellerLayout } from '../src/layouts'
import { Button, Field, FormDialog, Loaded, Pager, AddressBlock, Badge } from '../src/ui'
import { tokens, profiles, currentShop } from '../src/api'
import { cart } from '../src/cart'
import { shop, seller, buyer, listing, mockApi, mount, fail } from './helpers'

function layout(element, path, children = [{ path: '*', element: <p>Private content</p> }]) {
  const router = createMemoryRouter([{ element, children }, { path: '/login', element: <p>Login destination</p> }], { initialEntries: [path] })
  return { ...render(<RouterProvider router={router} />), router }
}

test('root layout shows role links, cart quantities and logs out to login', async () => {
  tokens.set({ access_token: 'test' })
  cart.set([{ listing, quantity: 2 }])
  mockApi(() => ({}))
  const { router } = layout(<RootLayout />, '/buyer/orders')
  expect(screen.getByRole('link', { name: 'Giỏ hàng 2' })).toBeTruthy()
  expect(screen.getAllByRole('link', { name: /Kênh người bán/ })[0]).toBeTruthy()
  expect(screen.getAllByRole('link', { name: 'Đơn mua' })[0].className).toContain('active')
  await userEvent.click(screen.getByRole('button', { name: 'Đăng xuất' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(tokens.get()).toBeNull()
})

test.each(['/login', '/seller/shops', '/demo/phone'])('root bypasses buyer navigation for %s', (path) => {
  layout(<RootLayout />, path)
  expect(screen.queryByRole('navigation', { name: 'Cửa hàng' })).toBeNull()
})

test('anonymous root exposes login while buyer guard redirects', async () => {
  profiles.set(null)
  const root = layout(<RootLayout />, '/')
  expect(screen.getByRole('link', { name: 'Đăng nhập' })).toBeTruthy()
  root.unmount()
  const { router } = layout(<BuyerLayout />, '/buyer/orders')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
})

test.each(['buyer', 'seller'])('portal rejects authenticated accounts without %s profile', (role) => {
  tokens.set({ access_token: 'test' })
  profiles.set({ buyer: null, seller: null })
  layout(role === 'buyer' ? <BuyerLayout /> : <SellerLayout />, '/buyer/orders')
  expect(screen.getByText(/Tài khoản này không có hồ sơ/)).toBeTruthy()
})

test('suspended buyer receives readonly notice', () => {
  tokens.set({ access_token: 'test' })
  profiles.set({ buyer: { ...buyer, status: 'suspended' }, seller: null })
  layout(<BuyerLayout />, '/buyer/orders')
  expect(screen.getByRole('alert').textContent).toMatch(/tạm khóa/)
  expect(screen.getByText('Private content')).toBeTruthy()
})

test('seller without selected shop is sent to shop management', async () => {
  tokens.set({ access_token: 'test' })
  currentShop.set(null)
  mockApi(() => [])
  const { router } = layout(<SellerLayout />, '/seller/dashboard')
  await waitFor(() => expect(router.state.location.pathname).toBe('/seller/shops'))
  expect(screen.getByText('Chọn shop')).toBeTruthy()
})

test('seller desktop and mobile navigation switch shops, close drawer and log out', async () => {
  tokens.set({ access_token: 'test' })
  mockApi(() => [shop, { ...shop, shop_id: 'shop:2', display_name: 'Second shop' }])
  const { router } = layout(<SellerLayout />, '/seller/dashboard')
  await userEvent.click(screen.getByRole('button', { name: /CỬA HÀNG ĐANG CHỌN/ }))
  await userEvent.click(await screen.findByRole('menuitem', { name: /Second shop/ }))
  expect(currentShop.get().shop_id).toBe('shop:2')
  await userEvent.click(screen.getByRole('button', { name: 'Mở điều hướng' }))
  const dialog = screen.getByRole('dialog')
  await userEvent.click(within(dialog).getByRole('button', { name: /CỬA HÀNG ĐANG CHỌN/ }))
  await userEvent.click(await screen.findByRole('menuitem', { name: /Test shop/ }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(currentShop.get().shop_id).toBe(shop.shop_id)
  await userEvent.click(screen.getByRole('button', { name: 'Mở điều hướng' }))
  await userEvent.click(screen.getByRole('button', { name: 'Đóng điều hướng' }))
  await userEvent.click(screen.getByRole('button', { name: 'Đăng xuất' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
})

test('seller shop-switch errors remain visible without clearing current selection', async () => {
  tokens.set({ access_token: 'test' })
  profiles.set({ buyer: null, seller: { ...seller, status: 'suspended' } })
  mockApi(() => fail(503, 'Shop list unavailable'))
  layout(<SellerLayout />, '/seller/shops')
  await userEvent.click(screen.getByRole('button', { name: /CỬA HÀNG ĐANG CHỌN/ }))
  expect(await screen.findByText('Shop list unavailable')).toBeTruthy()
  expect(currentShop.get()).toEqual(shop)
})

test('Loaded distinguishes first-load errors, loading, stale data and retry', async () => {
  const reload = vi.fn()
  const { rerender } = render(<Loaded state={{ error: 'First failure', reload }}>{(data) => <p>{data}</p>}</Loaded>)
  await userEvent.click(screen.getByRole('button', { name: 'Thử lại' }))
  expect(reload).toHaveBeenCalledOnce()
  rerender(<Loaded state={{ error: 'No retry' }}>{(data) => <p>{data}</p>}</Loaded>)
  expect(screen.queryByRole('button')).toBeNull()
  rerender(<Loaded state={{}}>{(data) => <p>{data}</p>}</Loaded>)
  expect(screen.getByRole('status')).toBeTruthy()
  rerender(<Loaded state={{ data: 'Saved data', error: 'Refresh failed' }}>{(data) => <p>{data}</p>}</Loaded>)
  expect(screen.getByText('Saved data')).toBeTruthy()
  expect(screen.getByRole('alert')).toBeTruthy()
})

test('Pager preserves unrelated filters and supports previous page', async () => {
  const { router } = mount(<Pager page={2} totalPages={3} />, '/?category=food&page=2')
  await userEvent.click(screen.getByRole('button', { name: 'Trước' }))
  expect(router.state.location.search).toBe('?category=food&page=1')
})

test('FormDialog blocks duplicate submissions and closing until request settles', async () => {
  let resolve
  const submit = vi.fn(() => new Promise((done) => { resolve = done }))
  render(<FormDialog title="Create" trigger={<Button>Create</Button>} onSubmit={submit}><Field label="Name" name="name" /></FormDialog>)
  await userEvent.click(screen.getByRole('button', { name: 'Create' }))
  await userEvent.type(screen.getByLabelText('Name'), 'Alice')
  fireEvent.submit(screen.getByLabelText('Name').closest('form'))
  fireEvent.submit(screen.getByLabelText('Name').closest('form'))
  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
  expect(submit).toHaveBeenCalledOnce()
  expect(submit.mock.calls[0][0].get('name')).toBe('Alice')
  expect(screen.getByRole('dialog')).toBeTruthy()
  await act(async () => resolve())
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('disabled FormDialog rejects submission and optional address/badge values remain explicit', async () => {
  const submit = vi.fn()
  render(<><FormDialog title="Disabled" trigger={<Button>Open</Button>} submitDisabled onSubmit={submit}><Field label="Name" /></FormDialog><AddressBlock address={{ address_raw: null }} /><Badge map={{}} value={null} /></>)
  expect(screen.getByText('Chưa biết địa chỉ')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: 'Open' }))
  fireEvent.submit(screen.getByLabelText('Name').closest('form'))
  expect(submit).not.toHaveBeenCalled()
})
