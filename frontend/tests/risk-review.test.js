import assert from 'node:assert/strict'
import { test } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

test('adaptive signals show measured baselines and nonblocking coverage separately from score', async () => {
  const { RiskFacts } = await import('../src/pages/seller/RiskReview.tsx')
  const assessment = {
    rule_config: { rules: [{ rule_id: 'SFD-A08', mode: 'shadow', enabled: true }] }, reasons: [], input_cutoff_at: '2026-01-01T00:00:00Z',
    feature_snapshot: { adaptive: {
      mode: 'shadow', baseline_version: 'demo-adaptive-baseline-0.1',
      input_cutoff_at: '2026-01-01T00:00:00Z',
      coverage: { status: 'insufficient_data', blocking: false, missing_features: { buyer_failure_rate: 'Cần thêm lịch sử giao' } },
      signals: [
        { rule_id: 'SFD-A08', status: 'fired', description: 'Số lượng bất thường', action: 'confirm_quantity',
          evidence: [{ label: 'Số lượng mặt hàng 1', observed: '20', comparison_threshold: '9.1',
            baseline: { cohort: 'shop_listing', sample_count: 10, threshold: '9.1' } }] },
        { rule_id: 'SFD-A01', status: 'unknown', description: 'Lịch sử giao', missing_reason: 'Cần thêm lịch sử giao',
          evidence: [{ label: 'Tỷ lệ lỗi giao', observed: null, missing_reason: 'Cần thêm lịch sử giao' }] },
        { rule_id: 'SFD-A11', status: 'clear', description: '<script>bad()</script>', evidence: null },
      ],
    } },
  }
  const html = renderToStaticMarkup(createElement(RiskFacts, { assessment }))
  assert.match(html, /Rule thử nghiệm \(shadow\)/)
  assert.match(html, /không cộng điểm hoặc thay đổi Hold\/Release/)
  assert.match(html, /A17 chỉ cảnh báo/)
  assert.match(html, /Có tín hiệu/)
  assert.match(html, /Các tín hiệu còn lại · 0/)
  assert.match(html, /Không kích hoạt/)
  assert.match(html, /Chưa đủ dữ liệu/)
  assert.match(html, /Ngưỡng so sánh: 9,1/)
  assert.match(html, /Mặt hàng tại cửa hàng/)
  assert.match(html, /Số mẫu: 10/)
  assert.match(html, /Xác nhận số lượng/)
  assert.match(html, /Cần thêm lịch sử giao/)
  assert.doesNotMatch(html, /<script>/)
  assessment.feature_snapshot.adaptive.signals = null
  const malformed = renderToStaticMarkup(createElement(RiskFacts, { assessment }))
  assert.doesNotMatch(malformed, /Rule thử nghiệm \(shadow\)/)
})

test('risk evidence uses stored facts and versioned parameters, distinguishes missing and disabled rules', async () => {
  const { RiskFacts } = await import('../src/pages/seller/RiskEvidence.tsx')
  const html = renderToStaticMarkup(createElement(RiskFacts, { assessment: {
    rule_config: { rules: [
      { rule_id: 'R1', enabled: true, params: { window_days: 120, alpha: 2, prior: 0.1, tiers: [[0.5, 33]] } },
      { rule_id: 'R2', enabled: true }, { rule_id: 'R9', enabled: false },
    ] },
    feature_snapshot: { R1: { resolved: '2', failed: '2', smoothed_rate: '0.55' },
      R2: { resolved: null }, unknown_reasons: { R2: 'Unknown resolved' } },
    reasons: [{ rule_id: 'R1', contribution: 33 }],
  } }))
  assert.match(html, /2 \/ 2 lần mua có kết quả giao trong 120 ngày/)
  assert.match(html, /0,55/)
  assert.match(html, /33 điểm/)
  assert.match(html, /Chưa đủ dữ liệu/)
  assert.match(html, /Chưa áp dụng/)
  assert.doesNotMatch(html, /180 ngày/)
})

test('review renders stored contributions, unknown facts and untrusted evidence as text', async () => {
  const { money, topReasons } = await import('../src/format.ts')
  const { maxQuantity } = await import('../src/cart.ts')
  assert.equal(money(null, 'VND'), 'Chưa có giá')
  const reasons = [
    { rule_id: 'R1', contribution: 10 },
    null,
    { rule_id: 'R2', contribution: 40 },
    { rule_id: 'R3', contribution: 30 },
    { rule_id: 'R4', contribution: 20 },
    { rule_id: 'bad', contribution: '90' },
  ]
  assert.deepEqual(
    topReasons({ reasons }).map((r) => r.rule_id),
    ['R2', 'R3', 'R4'],
  )
  assert.equal(reasons[0].rule_id, 'R1')
  assert.deepEqual(topReasons({ reasons: null }), [])
  assert.deepEqual(
    topReasons({
      reasons: [
        { rule_id: 'R7', points: 15 },
        { rule_id: 'R9', contribution: 0, points: 9 },
      ],
    }).map((r) => r.contribution),
    [15, 0],
  )
  assert.equal(maxQuantity({ current_price: null, stock_quantity: 10 }), 0)
  const { Loaded } = await import('../src/ui.tsx')
  const retry = renderToStaticMarkup(
    createElement(
      Loaded,
      {
        state: { data: { id: 'old-assessment' }, error: 'Refresh failed', reload: () => {} },
      },
      (data) => createElement('p', null, data.id),
    ),
  )
  assert.match(retry, /old-assessment/)
  assert.match(retry, /Refresh failed/)
  assert.match(retry, /Thử lại/)
  const { RiskFacts, CaseHistory } = await import('../src/pages/seller/RiskReview.tsx')
  const { NetworkGraph } = await import('../src/pages/seller/NetworkPanel.tsx')
  const network = renderToStaticMarkup(createElement(NetworkGraph, { network: {
    input_cutoff_at: '2026-01-01T00:00:00Z', truncated: true,
    nodes: [{ id: 'root', kind: 'checkout', label: '<script>bad()</script>', count: 1 },
      { id: 'other', kind: 'aggregate', label: 'Other shops', count: 12 }],
    edges: [{ source: 'root', target: 'other' }],
  } }))
  assert.ok(!network.includes('<script>'))
  assert.match(network, /&lt;script&gt;/)
  assert.match(network, /role="img"/)
  assert.match(network, /Other shops/)
  assert.match(network, /12/)
  assert.match(network, /giới hạn/)
  const html = renderToStaticMarkup(
    createElement(RiskFacts, {
      assessment: {
        input_cutoff_at: '2026-01-01T00:00:00Z',
        rule_config: {
          status: 'draft_not_calibrated',
          rules: [{ rule_id: 'R3', description: 'Buyer failures', enabled: true }],
        },
        feature_snapshot: { R3: { value: 3 }, R9: { known: false } },
        reasons: [
          { rule_id: 'R3', contribution: 40 },
          { rule_id: 'R7', points: 15 },
          { rule_id: 'R9', contribution: 0 },
          null,
          '<script>bad()</script>',
        ],
      },
    }),
  )
  assert.match(html, /Buyer failures/)
  assert.match(html, />40</)
  assert.match(html, />15</)
  assert.match(html, />0</)
  assert.match(html, /Chưa biết/)
  assert.match(html, /chưa hiệu chỉnh/)
  assert.doesNotMatch(html, /<script>/)
  const empty = renderToStaticMarkup(
    createElement(RiskFacts, {
      assessment: {
        rule_config: {},
        reasons: [],
        feature_snapshot: null,
        input_cutoff_at: null,
      },
    }),
  )
  assert.match(empty, /Không có rule đóng góp/)
  const cases = renderToStaticMarkup(
    createElement(CaseHistory, {
      cases: [
        {
          case_id: 'c',
          case_type: 'delivery',
          opened_at: '2026-01-01',
          messages: [],
          resolutions: [],
          evidence: [
            {
              media_type: 'text',
              submitted_by: 'buyer',
              content_ref: 'javascript:alert(1)',
              available_at: '2026-01-01',
            },
          ],
        },
      ],
    }),
  )
  assert.match(cases, /javascript:alert/)
  assert.doesNotMatch(cases, /href=/)
  assert.match(cases, /Chưa có kết luận/)
})
