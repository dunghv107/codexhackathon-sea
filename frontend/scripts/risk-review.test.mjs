import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { createServer } from 'vite'

test('review renders stored contributions, unknown facts and untrusted evidence as text', async () => {
  globalThis.window = new EventTarget()
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: { getItem: () => null } })
  const server = await createServer({ server: { middlewareMode: true }, appType: 'custom' })
  try {
    const { money } = await server.ssrLoadModule('/src/format.ts')
    const { maxQuantity } = await server.ssrLoadModule('/src/cart.ts')
    assert.equal(money(null, 'VND'), 'Chưa có giá')
    assert.equal(maxQuantity({ current_price: null, stock_quantity: 10 }), 0)
    const { RiskFacts, CaseHistory } = await server.ssrLoadModule('/src/pages/seller/RiskReview.tsx')
    const html = renderToStaticMarkup(createElement(RiskFacts, { assessment: {
      input_cutoff_at: '2026-01-01T00:00:00Z',
      rule_config: { status: 'draft_not_calibrated', rules: [{ rule_id: 'R3', description: 'Buyer failures', enabled: true }] },
      feature_snapshot: { R3: { value: 3 }, R9: { known: false } },
      reasons: [{ rule_id: 'R3', contribution: 40 }, null, '<script>bad()</script>'],
    } }))
    assert.match(html, /Buyer failures/)
    assert.match(html, />40</)
    assert.match(html, /Chưa biết/)
    assert.match(html, /chưa hiệu chỉnh/)
    assert.doesNotMatch(html, /<script>/)
    const empty = renderToStaticMarkup(createElement(RiskFacts, { assessment: { rule_config: {}, reasons: [], feature_snapshot: null, input_cutoff_at: null } }))
    assert.match(empty, /Không có rule đóng góp/)
    const cases = renderToStaticMarkup(createElement(CaseHistory, { cases: [{
      case_id: 'c', case_type: 'delivery', opened_at: '2026-01-01',
      messages: [], resolutions: [], evidence: [{ media_type: 'text', submitted_by: 'buyer', content_ref: 'javascript:alert(1)', available_at: '2026-01-01' }],
    }] }))
    assert.match(cases, /javascript:alert/)
    assert.doesNotMatch(cases, /href=/)
    assert.match(cases, /Chưa có kết luận/)
  } finally { await server.close(); delete globalThis.window; delete globalThis.localStorage }
})
