import { beforeEach, vi } from 'vitest'
import { render } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { tokens, profiles, currentShop } from '../src/api'
import { cart } from '../src/cart'

export const shop = { shop_id: 'shop:1', display_name: 'Test shop', batch_id: 'olist_vn_sim', assessed_orders: 2 }
export const buyer = { buyer_id: 'buyer:1', display_name: 'Buyer', status: 'active' }
export const seller = { seller_id: 'seller:1', display_name: 'Seller', status: 'active' }
export const listing = { listing_id: 'listing:1', shop_id: shop.shop_id, shop_name: shop.display_name,
  product_id: 'product:1', product_name: 'Coffee', category_code: 'food', category_name_en: 'Food',
  current_price: '12.50', currency: 'VND', stock_quantity: 5 }
export const address = { saved_address_id: 'address:1', label: 'Home', address_raw: '12 Main Street',
  region: 'City', country_code: 'VN', is_default: true, created_at: '2026-01-01' }
export const assessment = { assessment_id: 'assessment:1', assessed_at: '2026-01-01',
  input_cutoff_at: '2026-01-01', rule_version: 'v1', score: 65, risk_level: 'high',
  rule_config: { status: 'draft_not_calibrated', rules: [{ rule_id: 'R3', description: 'Buyer failures', enabled: true }] },
  feature_snapshot: { R3: { value: 3 } }, reasons: [{ rule_id: 'R3', contribution: 65 }] }
export const order = { order_id: 'order:1', checkout_id: 'checkout:1', placed_at: '2026-01-01',
  buyer_id: 'buyer:1', buyer_display_name: 'Buyer demo',
  shop_id: shop.shop_id, shop_name: shop.display_name, item_count: 2, order_total: '25.00', currency: 'VND',
  display_status: 'confirming', review_status: 'pending', fulfillment_status: 'unfulfilled',
  batch_id: 'olist_vn_sim', is_synthetic: true, source_system: 'test', payment_method: 'cod',
  risk_level: 'high', score: 65, latest_assessment: assessment,
  phone_masked: '', demo_mode: false, verification: null,
  address: { address_raw: '12 Main Street', city: 'City', region: 'City', country_code: 'VN',
    ward_code: '1', house_no: '12', parse_ok: true, postal_prefix: '100' },
  items: [{ item_id: 'item:1', listing_id: listing.listing_id, product_name: 'Coffee', category_name_en: 'Food', quantity: 2, unit_price: '12.50', freight_amount: '0.00' }],
  payment: { method: 'cod', status: 'pending' }, events: [], cases: [], decisions: [] }
export const network = { order_id: order.order_id, input_cutoff_at: '2026-01-01', nodes: [], edges: [], truncated: false }
export const page = (items = [], overrides = {}) => ({ items, total: items.length, page: 1, limit: 20, total_pages: 1, ...overrides })
export const ok = (data) => new Response(JSON.stringify({ is_success: true, data }), { status: 200 })
export const fail = (status = 500, message = 'Request failed') => new Response(JSON.stringify({ is_success: false, error: { message, code: 'test_error' } }), { status })

export function mockApi(handler) {
  const fetch = vi.fn(async (path, options) => {
    const result = await handler(String(path), options ?? {})
    if (result === undefined) throw new Error(`Unexpected request: ${path}`)
    return result instanceof Response ? result : ok(result)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

export function mount(element, path = '/', pattern = path.split('?')[0]) {
  const router = createMemoryRouter([
    { path: pattern, element },
    { path: '*', element: <p>Destination</p> },
  ], { initialEntries: [path] })
  return { ...render(<RouterProvider router={router} />), router }
}

beforeEach(() => {
  tokens.set(null)
  profiles.set({ buyer, seller })
  currentShop.set(shop)
  cart.set([])
})
