import { Link } from 'react-router'
import { currentShop, enc, useLoad } from '../../api'
import { fulfillmentLabel, reviewLabel, riskLabel } from '../../format'
import { useStore } from '../../store'
import type { Dashboard as DashboardData } from '../../types'
import { Loaded, Section } from '../../ui'

// Số đếm thật từ DB theo shop; mỗi ô dẫn tới danh sách đơn đã lọc.
function Counts({ title, filter, counts, labels }: { title: string; filter: string; counts: Record<string, number>; labels: Record<string, string> }) {
  return (
    <Section title={title}>
      <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {Object.entries(counts).map(([key, n]) => (
          <li key={key}>
            <Link
              to={`/seller/orders?${filter}=${key}`}
              className="block rounded-md border border-gray-200 p-3 hover:border-gray-400 focus-visible:outline-2 focus-visible:outline-blue-500"
            >
              <span className="block text-xs text-gray-500">{labels[key] ?? key}</span>
              <span className="text-xl font-semibold">{n}</span>
            </Link>
          </li>
        ))}
      </ul>
    </Section>
  )
}

export default function Dashboard() {
  const shop = useStore(currentShop)!
  const dashboard = useLoad<DashboardData>(`/api/seller/shops/${enc(shop.shop_id)}/dashboard`)

  return (
    <>
      <h1 className="text-2xl font-semibold">Tổng quan · {shop.display_name}</h1>
      <p className="text-sm text-gray-600">Chọn mức rủi ro để mở hàng đợi review. Điểm lấy từ assessment đã lưu; hệ thống không tự chấm điểm.</p>
      <Loaded state={dashboard}>
        {(d) => (
          <>
            <Counts title="Kiểm tra đơn" filter="review_status" counts={d.review_status} labels={reviewLabel} />
            <Counts title="Mức rủi ro" filter="risk_level" counts={d.risk_level} labels={riskLabel} />
            <Counts title="Giao hàng" filter="fulfillment_status" counts={d.fulfillment_status} labels={fulfillmentLabel} />
          </>
        )}
      </Loaded>
    </>
  )
}
