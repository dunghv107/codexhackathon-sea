import { Link, useParams, useSearchParams } from 'react-router'
import { enc, qs, useLoad } from '../api'
import { addToCart, cart, maxQuantity } from '../cart'
import { money, productLabel } from '../format'
import { useStore } from '../store'
import type { CatalogShop, Category, ListingCard, Page } from '../types'
import { Button, Loaded, Pager, ProductPlaceholder, SelectField } from '../ui'

// Trang chủ (/) và trang shop (/shops/:shopId) dùng chung lưới listing.
export default function Catalog() {
  const { shopId } = useParams()
  const [params, setParams] = useSearchParams()
  const category = params.get('category') ?? ''
  const page = Number(params.get('page')) || 1
  const lines = useStore(cart)

  const categories = useLoad<Category[]>('/api/catalog/categories')
  const shop = useLoad<CatalogShop>(shopId ? `/api/catalog/shops/${enc(shopId)}` : null)
  const listings = useLoad<Page<ListingCard>>(
    `/api/catalog/listings?${qs({ shop_id: shopId, category_code: category, page, limit: 24 })}`,
  )

  const inCart = (l: ListingCard) => lines.find((x) => x.listing.listing_id === l.listing_id)?.quantity ?? 0

  return (
    <>
      <div className="flex flex-wrap items-end justify-between gap-4">
        {shopId ? (
          <Loaded state={shop}>
            {(s) => (
              <div>
                <h1 className="text-2xl font-semibold">{s.display_name}</h1>
                <p className="text-sm text-gray-500">{s.listing_count} sản phẩm</p>
              </div>
            )}
          </Loaded>
        ) : (
          <h1 className="text-2xl font-semibold">Sản phẩm</h1>
        )}
        <div className="w-64">
          <SelectField
            label="Danh mục"
            value={category || 'all'}
            onValueChange={(v) => setParams(v === 'all' ? {} : { category: v })}
            options={[
              ['all', 'Tất cả danh mục'],
              ...(categories.data ?? []).map((c): [string, string] => [
                c.category_code,
                c.category_name_en ?? c.category_code,
              ]),
            ]}
          />
        </div>
      </div>

      <Loaded state={listings}>
        {(res) => (
          <>
            {res.items.length === 0 && <p className="text-sm text-gray-500">Không có sản phẩm.</p>}
            <ul className="catalog-grid grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
              {res.items.map((l) => (
                <li
                  key={l.listing_id}
                  className="flex flex-col gap-2 rounded-lg border border-gray-200 bg-white p-4"
                >
                  <ProductPlaceholder />
                  <p className="font-medium">
                    {productLabel(l.product_name, l.category_name_en, l.product_id)}
                  </p>
                  {l.category_name_en && <p className="text-xs text-gray-500">{l.category_name_en}</p>}
                  {!shopId && (
                    <Link to={`/shops/${enc(l.shop_id)}`} className="text-sm text-blue-700 hover:underline">
                      {l.shop_name}
                    </Link>
                  )}
                  <p className="mt-auto text-lg font-semibold">{money(l.current_price, l.currency)}</p>
                  {l.stock_quantity !== null && (
                    <p className="text-xs text-gray-500">Còn {l.stock_quantity}</p>
                  )}
                  <Button
                    variant="primary"
                    disabled={inCart(l) >= maxQuantity(l)}
                    onClick={() => addToCart(l)}
                  >
                    {l.current_price === null
                      ? 'Chưa có giá'
                      : maxQuantity(l) === 0
                        ? 'Hết hàng'
                        : inCart(l)
                          ? `Thêm vào giỏ (${inCart(l)})`
                          : 'Thêm vào giỏ'}
                  </Button>
                </li>
              ))}
            </ul>
            <Pager page={res.page} totalPages={res.total_pages} />
          </>
        )}
      </Loaded>
    </>
  )
}
