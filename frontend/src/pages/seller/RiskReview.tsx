import { dateTime } from '../../format'
import type { OrderCase } from '../../types'
import { Section } from '../../ui'
export { RiskFacts } from './RiskEvidence'

export function CaseHistory({ cases }: { cases: OrderCase[] }) {
  return (
    <Section title="Hồ sơ tranh chấp của đơn">
      <p className="text-sm text-gray-600">
        Lời trình bày và bằng chứng tách khỏi kết luận đã xác minh. Hồ sơ dưới đây có thể được bổ sung sau
        cutoff của assessment.
      </p>
      {cases.length === 0 && <p className="text-sm">Đơn này chưa có hồ sơ tranh chấp.</p>}
      {cases.map((c) => (
        <details key={c.case_id} className="rounded border border-gray-200 p-3">
          <summary className="cursor-pointer text-sm font-medium">
            {c.case_type} · {c.case_id} · {dateTime(c.opened_at)}
          </summary>
          <div className="mt-3 space-y-3 text-sm">
            {c.messages.map((m, i) => (
              <p key={i}>
                <strong>{m.speaker_role}</strong> · {dateTime(m.occurred_at)}: {m.body}
              </p>
            ))}
            {c.evidence.map((e, i) => (
              <p key={i}>
                Bằng chứng {e.media_type} ({e.submitted_by}):{' '}
                <code className="break-all">{e.content_ref}</code> · biết lúc {dateTime(e.available_at)}
              </p>
            ))}
            {c.resolutions.length === 0 && <p>Chưa có kết luận.</p>}
            {[...c.resolutions]
              .sort((a, b) => Date.parse(b.decided_at) - Date.parse(a.decided_at))
              .map((r, i) => (
                <div key={i} className="rounded bg-gray-50 p-3">
                  <p>
                    {r.verification_status} · trách nhiệm: {r.responsible_party}
                  </p>
                  <p>{r.rationale}</p>
                  <p className="text-xs text-gray-500">
                    Quyết định {dateTime(r.decided_at)} · biết lúc {dateTime(r.available_at)}
                  </p>
                </div>
              ))}
          </div>
        </details>
      ))}
    </Section>
  )
}
