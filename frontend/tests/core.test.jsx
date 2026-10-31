import { test, expect, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { api, ApiError, tokens, profiles, currentShop, logout, useLoad, qs, enc, errMsg } from '../src/api'
import { cart, setQuantity, addToCart, maxQuantity } from '../src/cart'
import { persisted, useStore } from '../src/store'
import { money, dateTime, toCents, fromCents, categoryLabel, productLabel, label, topReasons } from '../src/format'
import { listing, mockApi, ok, fail } from './helpers'

test('formatters retain unknown values and use deterministic currency arithmetic', () => {
  expect(toCents('12.5')).toBe(1250)
  expect(toCents('12')).toBe(1200)
  expect(toCents('0.129')).toBe(12)
  expect(fromCents(1250)).toBe('12.50')
  expect(money(null, 'VND')).toBe('Chưa có giá')
  expect(money('12', 'invalid')).toBe('12 invalid')
  expect(money('12', 'VND')).toContain('12')
  expect(dateTime(null)).toBe('—')
  expect(dateTime('2026-01-01')).toContain('2026')
  expect(productLabel('Name', 'Food')).toBe('Name')
  expect(productLabel(null, null)).toBe('Sản phẩm')
  expect(productLabel(null, 'Food')).toBe('Food')
  expect(productLabel('  ', 'bed_bath_table')).toBe('Bed bath table')
  expect(productLabel('Cà phê rang mộc', 'food')).toBe('Cà phê rang mộc')
  expect(categoryLabel('  home__and_living  ')).toBe('Home and living')
  expect(categoryLabel('Food')).toBe('Food')
  expect(categoryLabel(null)).toBe('Chưa phân loại')
  expect(categoryLabel('')).toBe('Chưa phân loại')
  expect(label({ yes: 'Yes' }, 'yes')).toBe('Yes')
  expect(label({}, 'unknown')).toBe('unknown')
  expect(label({}, null)).toBe('—')
  expect(topReasons({ reasons: [null, 'bad', { rule_id: 'R1', contribution: Infinity }, { rule_id: 'R2', points: 2 }] })).toEqual([{ rule_id: 'R2', points: 2, contribution: 2 }])
  expect(qs({ a: null, b: undefined, c: '', d: 0, e: 'x y' })).toBe('d=0&e=x+y')
  expect(enc('a:b/c')).toBe('a%3Ab%2Fc')
  expect(errMsg('failure')).toBe('failure')
  expect(errMsg(new Error('failure'))).toBe('failure')
})

test('cart caps quantities to stock, updates the selected listing and removes invalid quantities', () => {
  expect(maxQuantity({ ...listing, stock_quantity: null })).toBe(99)
  expect(maxQuantity({ ...listing, current_price: null })).toBe(0)
  setQuantity(listing, 100)
  expect(cart.get()[0].quantity).toBe(5)
  const other = { ...listing, listing_id: 'other' }
  addToCart(other)
  addToCart(other)
  expect(cart.get().map((line) => line.quantity)).toEqual([5, 2])
  setQuantity(listing, 2.9)
  expect(cart.get()[0].quantity).toBe(2)
  setQuantity(listing, NaN)
  expect(cart.get()).toEqual([{ listing: other, quantity: 2 }])
  setQuantity(other, -1)
  expect(cart.get()).toEqual([])
})

test('persisted stores recover from malformed or unavailable storage and synchronize tabs', () => {
  localStorage.setItem('saved', '{"count":2}')
  const state = persisted('saved', { count: 0 })
  const listener = vi.fn()
  const unsubscribe = state.subscribe(listener)
  expect(state.get()).toEqual({ count: 2 })
  state.set({ count: 3 })
  expect(JSON.parse(localStorage.getItem('saved'))).toEqual({ count: 3 })
  window.dispatchEvent(new StorageEvent('storage', { key: 'other', newValue: '1' }))
  window.dispatchEvent(new StorageEvent('storage', { key: 'saved', newValue: 'bad json' }))
  expect(state.get()).toEqual({ count: 3 })
  window.dispatchEvent(new StorageEvent('storage', { key: 'saved', newValue: '{"count":4}' }))
  expect(state.get()).toEqual({ count: 4 })
  window.dispatchEvent(new StorageEvent('storage', { key: 'saved', newValue: null }))
  expect(state.get()).toEqual({ count: 0 })
  expect(listener).toHaveBeenCalledTimes(3)
  unsubscribe()
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked') })
  state.set({ count: 9 })
  expect(state.get()).toEqual({ count: 9 })
  expect(listener).toHaveBeenCalledTimes(3)
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
  expect(persisted('blocked', 7).get()).toBe(7)
})

test('useStore rerenders subscribers and detaches on unmount', () => {
  const state = persisted('subscription', 0)
  const { result, unmount } = renderHook(() => useStore(state))
  act(() => state.set(2))
  expect(result.current).toBe(2)
  unmount()
  state.set(3)
})

test('API sends JSON and bearer auth, unwraps data and accepts empty responses', async () => {
  tokens.set({ access_token: 'test-access' })
  const fetch = mockApi((path) => path === '/empty' ? new Response(null, { status: 204 }) : { id: 1 })
  expect(await api('/items', 'POST', { name: 'Coffee' })).toEqual({ id: 1 })
  expect(fetch.mock.calls[0][1]).toEqual({ method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer test-access' }, body: '{"name":"Coffee"}' })
  expect(await api('/empty')).toBeUndefined()
})

test('concurrent unauthorized requests refresh once and retry with rotated tokens', async () => {
  tokens.set({ access_token: 'old', refresh_token: 'refresh' })
  let resolveRefresh
  const fetch = mockApi((path, options) => {
    if (path === '/api/auth/refresh') return new Promise((resolve) => { resolveRefresh = resolve })
    return options.headers.Authorization === 'Bearer old' ? fail(401) : { id: path }
  })
  const a = api('/a'), b = api('/b')
  await waitFor(() => expect(resolveRefresh).toBeTypeOf('function'))
  resolveRefresh(ok({ access_token: 'new', refresh_token: 'rotated' }))
  expect(await Promise.all([a, b])).toEqual([{ id: '/a' }, { id: '/b' }])
  expect(fetch.mock.calls.filter(([path]) => path === '/api/auth/refresh')).toHaveLength(1)
  expect(tokens.get()).toEqual({ access_token: 'new', refresh_token: 'rotated' })
})

test.each(['absent', 'expired', 'retry-unauthorized'])('invalid sessions clear tokens: %s', async (scenario) => {
  tokens.set({ access_token: 'old', ...(scenario !== 'absent' ? { refresh_token: 'refresh' } : {}) })
  mockApi((path) => path === '/api/auth/refresh' && scenario === 'retry-unauthorized' ? { access_token: 'new' } : fail(401, 'Expired'))
  await expect(api('/private')).rejects.toMatchObject({ status: 401, message: 'Expired' })
  expect(tokens.get()).toBeNull()
})

test.each([429, 500])('temporary refresh failure preserves the session: %s', async (status) => {
  tokens.set({ access_token: 'old', refresh_token: 'refresh' })
  mockApi((path) => path === '/api/auth/refresh' ? new Response('not json', { status }) : fail(401))
  await expect(api('/private')).rejects.toMatchObject({ status })
  expect(tokens.get().refresh_token).toBe('refresh')
})

test('refresh retains old refresh token when provider returns only access token', async () => {
  tokens.set({ access_token: 'old', refresh_token: 'refresh' })
  mockApi((path, options) => path === '/api/auth/refresh' ? { access_token: 'new' } : options.headers.Authorization === 'Bearer old' ? fail(401) : 'ok')
  expect(await api('/private')).toBe('ok')
  expect(tokens.get()).toEqual({ access_token: 'new', refresh_token: 'refresh' })
})

test('API rejects malformed/error envelopes and logout clears all account state despite network failure', async () => {
  mockApi(() => new Response('not json', { status: 502 }))
  await expect(api('/bad')).rejects.toBeInstanceOf(ApiError)
  await expect(api('/bad')).rejects.toThrow('502')
  mockApi(() => fail(400, 'Invalid input'))
  await expect(api('/bad')).rejects.toMatchObject({ code: 'test_error', message: 'Invalid input' })
  tokens.set({ access_token: 'test' })
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
  await logout()
  expect(tokens.get()).toBeNull()
  expect(profiles.get()).toBeNull()
  expect(currentShop.get()).toBeNull()
})

test('useLoad ignores stale requests, preserves data on refresh errors, resets when path changes', async () => {
  let resolveOld
  const fetch = mockApi((path) => path === '/old' ? new Promise((resolve) => { resolveOld = resolve }) : { id: path })
  const { result, rerender, unmount } = renderHook(({ path }) => useLoad(path), { initialProps: { path: null } })
  expect(fetch).not.toHaveBeenCalled()
  rerender({ path: '/old' })
  await waitFor(() => expect(resolveOld).toBeTypeOf('function'))
  rerender({ path: '/new' })
  await waitFor(() => expect(result.current.data).toEqual({ id: '/new' }))
  await act(async () => resolveOld(ok({ id: 'stale' })))
  expect(result.current.data.id).toBe('/new')
  mockApi(() => fail(503, 'Retry later'))
  act(() => result.current.reload())
  await waitFor(() => expect(result.current.error).toBe('Retry later'))
  expect(result.current.data).toEqual({ id: '/new' })
  rerender({ path: '/failure' })
  await waitFor(() => expect(result.current.loading).toBe(false))
  expect(result.current.data).toBeUndefined()
  unmount()
})
