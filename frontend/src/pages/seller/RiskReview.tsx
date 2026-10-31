import { dateTime } from '../../format'
import type { Assessment, OrderCase } from '../../types'
import { Section } from '../../ui'

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}
function display(value: unknown): string {
  if (value === null || value === undefined) return 'Chưa biết'
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

export function RiskFacts({ assessment: a }: { assessment: Assessment }) {
  const facts = record(a.feature_snapshot)
  const descriptions = new Map((a.rule_config?.rules ?? []).map((r) => [r.rule_id, r.description]))
  const reasons = Array.isArray(a.reasons) ? a.reasons : []
  return <div className="space-y-3">
    <p className="text-sm text-gray-600">Điểm rủi ro hỗ trợ review, không phải xác suất hay kết luận gian lận. Cutoff: {dateTime(a.input_cutoff_at)}.</p>
    {a.rule_config?.status === 'draft_not_calibrated' && <p className="text-sm text-amber-800">Rule demo chưa hiệu chỉnh bằng dữ liệu thực tế.</p>}
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <caption className="text-left font-semibold">Các rule đóng góp điểm</caption>
        <thead><tr><th className="p-2">Rule</th><th className="p-2">Lý do</th><th className="p-2">Điểm</th></tr></thead>
        <tbody>{reasons.map((value, i) => {
          const r = record(value)
          const id = typeof r.rule_id === 'string' ? r.rule_id : ''
          return <tr key={i} className="border-t border-gray-100"><td className="p-2">{id || '—'}</td><td className="p-2">{descriptions.get(id) ?? display(r.message ?? r.description ?? value)}</td><td className="p-2">{display(r.contribution)}</td></tr>
        })}</tbody>
      </table>
      {reasons.length === 0 && <p className="py-2 text-sm">Không có rule đóng góp điểm trong assessment này.</p>}
    </div>
    <details><summary className="cursor-pointer text-sm font-medium">Dữ kiện tại thời điểm chấm</summary>
      <dl className="mt-2 space-y-2">{Object.entries(facts).map(([id, value]) => <div key={id} className="rounded border border-gray-200 p-3">
        <dt className="text-sm font-medium">{id} · {descriptions.get(id) ?? 'Dữ kiện'}</dt>
        <dd><pre className="mt-1 overflow-x-auto text-xs">{JSON.stringify(value, null, 2)}</pre></dd>
      </div>)}</dl>
    </details>
  </div>
}

export function CaseHistory({ cases }: { cases: OrderCase[] }) {
  return <Section title="Hồ sơ tranh chấp của đơn">
    <p className="text-sm text-gray-600">Lời trình bày và bằng chứng tách khỏi kết luận đã xác minh. Hồ sơ dưới đây có thể được bổ sung sau cutoff của assessment.</p>
    {cases.length === 0 && <p className="text-sm">Đơn này chưa có hồ sơ tranh chấp.</p>}
    {cases.map((c) => <details key={c.case_id} className="rounded border border-gray-200 p-3">
      <summary className="cursor-pointer text-sm font-medium">{c.case_type} · {c.case_id} · {dateTime(c.opened_at)}</summary>
      <div className="mt-3 space-y-3 text-sm">
        {c.messages.map((m, i) => <p key={i}><strong>{m.speaker_role}</strong> · {dateTime(m.occurred_at)}: {m.body}</p>)}
        {c.evidence.map((e, i) => <p key={i}>Bằng chứng {e.media_type} ({e.submitted_by}): <code className="break-all">{e.content_ref}</code> · biết lúc {dateTime(e.available_at)}</p>)}
        {c.resolutions.length === 0 && <p>Chưa có kết luận.</p>}
        {c.resolutions.map((r, i) => <div key={i} className="rounded bg-gray-50 p-3"><p>{r.verification_status} · trách nhiệm: {r.responsible_party}</p><p>{r.rationale}</p><p className="text-xs text-gray-500">Quyết định {dateTime(r.decided_at)} · biết lúc {dateTime(r.available_at)}</p></div>)}
      </div>
    </details>)}
  </Section>
}
