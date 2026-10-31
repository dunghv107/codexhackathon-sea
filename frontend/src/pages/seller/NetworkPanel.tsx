import { useLoad } from '../../api'
import { dateTime } from '../../format'
import type { RiskNetwork } from '../../types'
import { Loaded, Section } from '../../ui'

export function NetworkGraph({ network }: { network: RiskNetwork }) {
  const columns = [0, 0, 0]
  const nodes = network.nodes.map((node, i) => {
    const col = i === 0 ? 0 : node.kind === 'identity' ? 1 : 2
    return { ...node, x: 145 + col * 290, y: 35 + columns[col]++ * 60 }
  })
  const positions = new Map(nodes.map((n) => [n.id, n]))
  return <>
    <p className="subtle">Dữ liệu biết tại {dateTime(network.input_cutoff_at)}. Liên hệ chung không phải kết luận gian lận.</p>
    {nodes.length <= 1 ? <p>Chưa có định danh để hiển thị liên hệ.</p> :
      <section className="overflow-auto max-h-96 border rounded-lg" aria-label="Sơ đồ liên hệ có thể cuộn"
        tabIndex={0 /* NOSONAR: S6845 - native keyboard scrolling requires focus in browsers without focusable scroll containers. */}>
        <svg width="870" height={Math.max(120, Math.max(...columns) * 60 + 10)} role="img" aria-label="Sơ đồ checkout và định danh; chi tiết trong danh sách bên dưới">
          {network.edges.map((edge, i) => {
            const a = positions.get(edge.source), b = positions.get(edge.target)
            return a && b ? <line key={i} x1={a.x} y1={a.y} x2={b.x} y2={b.y} stroke="#94a3b8" /> : null
          })}
          {nodes.map((n) => <g key={n.id}>
            <title>{`${n.label} · ${n.count}`}</title>
            <rect x={n.x - 135} y={n.y - 20} width="270" height="40" rx="8" fill={n.kind === 'identity' ? '#fef3c7' : '#f1f5f9'} stroke="#64748b" />
            <text x={n.x} y={n.y + 5} textAnchor="middle" fontSize="12" fill="#0f172a">{n.label.length > 29 ? `${n.label.slice(0, 28)}…` : n.label}{n.kind === 'aggregate' ? ` (${n.count})` : ''}</text>
          </g>)}
        </svg>
      </section>}
    {network.truncated && <p role="status">Sơ đồ đã giới hạn số nút; đây không phải toàn bộ lịch sử.</p>}
    <details><summary>Danh sách liên hệ ({nodes.length} nút)</summary>
      <ul className="text-sm space-y-1">{nodes.map((n) => <li key={n.id} className="break-all">{n.label} · {n.count}</li>)}</ul>
    </details>
    <p className="subtle">Đơn của shop khác chỉ được hiển thị dưới dạng số lượng tổng hợp.</p>
    <p className="subtle">Một checkout có thể chung nhiều định danh; không cộng các nhánh thành tổng số khách.</p>
  </>
}

export default function NetworkPanel({ path }: { path: string }) {
  const state = useLoad<RiskNetwork>(`${path}/network`)
  return <Section title="Liên hệ giữa các đơn hàng"><Loaded state={state}>{(network) => <NetworkGraph network={network} />}</Loaded></Section>
}
