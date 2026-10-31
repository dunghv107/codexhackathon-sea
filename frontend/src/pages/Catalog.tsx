import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { enc, qs, useLoad } from '../api'
import { addToCart, cart, maxQuantity } from '../cart'
import { money, productLabel } from '../format'
import { useStore } from '../store'
import type { CatalogShop, Category, ListingCard, Page } from '../types'
import { Button, Loaded, Pager, ProductPlaceholder, SelectField } from '../ui'

export default function Catalog() {
  const { shopId } = useParams()
  const [params, setParams] = useSearchParams()
  const category = params.get('category') ?? ''
  const currency = params.get('currency') ?? ''
  const query = params.get('q') ?? ''
  const page = Number(params.get('page')) || 1
  const lines = useStore(cart)
  const [notice, setNotice] = useState('')
  const categories = useLoad<Category[]>('/api/catalog/categories')
  const shop = useLoad<CatalogShop>(shopId ? `/api/catalog/shops/${enc(shopId)}` : null)
  const listings = useLoad<Page<ListingCard>>(
    `/api/catalog/listings?${qs({ shop_id: shopId, category_code: category, q: query, sort: params.get('sort'), currency, in_stock: params.get('in_stock'), page, limit: 20 })}`,
  )
  const filter = (key: string, value: string) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      if (!value || value === 'all') next.delete(key)
      else next.set(key, value)
      if (key === 'currency' && (!value || value === 'all')) next.delete('sort')
      next.delete('page')
      return next
    })
  return (
    <>
      {shopId ? (
        <Loaded state={shop}>
          {(s) => (
            <section className="shop-banner">
              <div className="shop-avatar">{s.display_name.slice(0, 1).toUpperCase()}</div>
              <div>
                <p className="eyebrow">CỬA HÀNG TRÊN OLIST</p>
                <h1>{s.display_name}</h1>
                <p>{s.listing_count} sản phẩm đang niêm yết</p>
              </div>
              <Link to="/shops" className="ml-auto underline">
                Khám phá cửa hàng khác →
              </Link>
            </section>
          )}
        </Loaded>
      ) : (
        !query &&
        !category && (
          <section className="market-hero">
            <div>
              <p className="eyebrow">OLIST MARKETPLACE</p>
              <h1>
                Khám phá điều mới.
                <br />
                <span>Chọn món bạn yêu.</span>
              </h1>
              <p>
                Tìm sản phẩm, ghé cửa hàng và theo dõi từng đơn mua
                <br className="hidden sm:block" /> — tất cả trong một không gian.
              </p>
              <a className="hero-cta" href="#products">
                Khám phá sản phẩm ↗
              </a>
              <Link to="/shops" className="hero-secondary">
                Ghé các cửa hàng
              </Link>
            </div>
            <div className="hero-art" aria-hidden="true">
              <div className="art-disc" />
              <div className="art-bag">
                <span>olist</span>
                <i />
              </div>
              <div className="art-box">◇</div>
              <span className="art-tag">
                MỘT CHÚT CẢM HỨNG
                <br />
                CHO MỖI NGÀY
              </span>
            </div>
          </section>
        )
      )}
      {!shopId && !query && (
        <div className="market-shortcuts">
          <Link to="/shops">
            <span>◇</span>
            <div>
              <strong>Ghé thăm cửa hàng</strong>
              <small>Khám phá từng gian hàng</small>
            </div>
            <b>↗</b>
          </Link>
          <Link to="/buyer/orders">
            <span>▤</span>
            <div>
              <strong>Đơn mua của bạn</strong>
              <small>Theo dõi tiến trình giao hàng</small>
            </div>
            <b>↗</b>
          </Link>
          <Link to="/buyer/checkout">
            <span>＋</span>
            <div>
              <strong>Giỏ hàng</strong>
              <small>{lines.reduce((n, l) => n + l.quantity, 0)} sản phẩm đang chờ bạn</small>
            </div>
            <b>↗</b>
          </Link>
        </div>
      )}
      <div className="catalog-layout" id="products">
        <aside className="catalog-filters">
          <h2>Bộ lọc tìm kiếm</h2>
          <div className="filter-group">
            <h3>Danh mục</h3>
            <button className={!category ? 'selected' : ''} onClick={() => filter('category', '')}>
              Tất cả sản phẩm
            </button>
            <Loaded state={categories}>
              {(cats) =>
                cats.map((c) => (
                  <button
                    key={c.category_code}
                    className={category === c.category_code ? 'selected' : ''}
                    onClick={() => filter('category', c.category_code)}
                  >
                    {c.category_name_en ?? c.category_code}
                  </button>
                ))
              }
            </Loaded>
          </div>
          <SelectField
            label="Tiền tệ"
            value={currency || 'all'}
            options={[
              ['all', 'Tất cả tiền tệ'],
              ['VND', 'Việt Nam · VND'],
              ['BRL', 'Brazil · BRL'],
            ]}
            onValueChange={(v) => filter('currency', v)}
          />
          <label className="stock-filter">
            <input
              type="checkbox"
              checked={params.get('in_stock') === 'true'}
              onChange={(e) => filter('in_stock', e.target.checked ? 'true' : '')}
            />{' '}
            Chỉ sản phẩm còn hàng
          </label>
          {params.size > 0 && <Button onClick={() => setParams({})}>Xóa tất cả bộ lọc</Button>}
        </aside>
        <section className="catalog-results">
          <header className="catalog-heading">
            <div>
              <p className="eyebrow">CHỌN MÓN BẠN YÊU THÍCH</p>
              <h2>
                {query
                  ? `Kết quả cho “${query}”`
                  : category
                    ? (categories.data?.find((c) => c.category_code === category)?.category_name_en ??
                      category)
                    : 'Khám phá sản phẩm'}
              </h2>
              {listings.data && (
                <p className="subtle">
                  {listings.data.total} sản phẩm{shopId ? ' trong cửa hàng' : ''}
                </p>
              )}
            </div>
            <div className="sort-control">
              <SelectField
                label="Sắp xếp theo"
                value={params.get('sort') || 'all'}
                options={[
                  ['all', 'Mặc định'],
                  ['name', 'Tên sản phẩm A–Z'],
                  ...(currency
                    ? ([
                        ['price_asc', 'Giá: thấp đến cao'],
                        ['price_desc', 'Giá: cao đến thấp'],
                      ] as [string, string][])
                    : []),
                ]}
                onValueChange={(v) => filter('sort', v)}
              />
              {!currency && <p className="subtle">Chọn tiền tệ để sắp xếp giá</p>}
            </div>
          </header>
          <Loaded state={listings}>
            {(res) => (
              <>
                {res.items.length === 0 ? (
                  <div className="empty">
                    <p className="text-lg mb-2">Chưa tìm thấy sản phẩm phù hợp</p>
                    <p>Thử từ khóa khác hoặc bỏ bớt bộ lọc.</p>
                    <Button className="mt-4" onClick={() => setParams({})}>
                      Xem tất cả sản phẩm
                    </Button>
                  </div>
                ) : (
                  <ul className="market-products">
                    {res.items.map((l) => {
                      const name = productLabel(l.product_name, l.category_name_en, l.product_id)
                      const inCart = lines.find((x) => x.listing.listing_id === l.listing_id)?.quantity ?? 0
                      return (
                        <li key={l.listing_id} className="product-card">
                          <Link to={`/products/${enc(l.listing_id)}`} className="product-visual">
                            <ProductPlaceholder src={l.image_url} alt={name} />
                            {l.stock_quantity === 0 && (
                              <span className="product-stock-badge">Tạm hết hàng</span>
                            )}
                          </Link>
                          <div className="product-card-body">
                            <Link to={`/shops/${enc(l.shop_id)}`} className="product-shop">
                              {l.shop_name} ↗
                            </Link>
                            <Link to={`/products/${enc(l.listing_id)}`} className="product-title">
                              {name}
                            </Link>
                            <p className="product-price">{money(l.current_price, l.currency)}</p>
                            <div className="product-card-bottom">
                              <span>
                                {l.stock_quantity === null
                                  ? 'Tồn kho chưa cập nhật'
                                  : `Còn ${l.stock_quantity}`}
                              </span>
                              <Button
                                disabled={inCart >= maxQuantity(l)}
                                aria-label={`Thêm ${name} vào giỏ`}
                                onClick={() => {
                                  addToCart(l)
                                  setNotice(`Đã thêm ${name} vào giỏ hàng`)
                                }}
                              >
                                ＋
                              </Button>
                            </div>
                          </div>
                        </li>
                      )
                    })}
                  </ul>
                )}
                <Pager page={res.page} totalPages={res.total_pages} />
              </>
            )}
          </Loaded>
        </section>
      </div>
      {notice && (
        <div role="status" className="toast">
          {notice}{' '}
          <Link to="/buyer/checkout" className="underline ml-3">
            Xem giỏ hàng
          </Link>
          <button onClick={() => setNotice('')} aria-label="Đóng thông báo" className="ml-4">
            ×
          </button>
        </div>
      )}
    </>
  )
}
