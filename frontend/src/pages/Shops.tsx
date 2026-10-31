import { Link, useSearchParams } from 'react-router'
import { enc, qs, useLoad } from '../api'
import type { CatalogShop, Page } from '../types'
import { Loaded, Pager } from '../ui'
export default function Shops() {
  const [params] = useSearchParams()
  const shops = useLoad<Page<CatalogShop>>(
    `/api/catalog/shops?${qs({ page: Number(params.get('page')) || 1, limit: 20 })}`,
  )
  return (
    <>
      <header className="page-head">
        <div>
          <p className="eyebrow">CÁC GIAN HÀNG TRÊN AFD.</p>
          <h1 className="mt-2">Tìm cửa hàng bạn yêu thích</h1>
          <p className="text-gray-500 mt-2">Ghé từng gian hàng và khám phá danh mục sản phẩm.</p>
        </div>
      </header>
      <Loaded state={shops}>
        {(res) => (
          <>
            {res.items.length === 0 ? (
              <p className="empty">Chưa có cửa hàng nào.</p>
            ) : (
              <div className="shop-directory">
                {res.items.map((s) => (
                  <Link className="card directory-card" key={s.shop_id} to={`/shops/${enc(s.shop_id)}`}>
                    <div className="shop-avatar">{s.display_name.slice(0, 1)}</div>
                    <h2>{s.display_name}</h2>
                    <p className="subtle">{s.listing_count} sản phẩm đang niêm yết</p>
                    <span>Ghé cửa hàng ↗</span>
                  </Link>
                ))}
              </div>
            )}
            <Pager page={res.page} totalPages={res.total_pages} />
          </>
        )}
      </Loaded>
    </>
  )
}
