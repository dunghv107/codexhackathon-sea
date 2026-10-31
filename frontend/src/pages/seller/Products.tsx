import { Link, useSearchParams } from 'react-router'
import { api, currentShop, enc, qs, useLoad } from '../../api'
import { fromCents, money, productLabel, toCents } from '../../format'
import { useStore } from '../../store'
import type { Category, ListingCard, Page } from '../../types'
import {
  Button,
  Field,
  FormDialog,
  Loaded,
  Pager,
  SelectField,
  TextArea,
  ProductPlaceholder,
} from '../../ui'

const pricePattern = '\\d+(\\.\\d{1,2})?'

// "12.5" -> "12.50": tiền gửi lên dạng string thập phân 2 chữ số.
const price = (value: FormDataEntryValue | null) => fromCents(toCents(String(value).trim()))
const stock = (value: FormDataEntryValue | null) =>
  String(value ?? '').trim() === '' ? undefined : Number(value)

export default function Products() {
  const shop = useStore(currentShop)!
  const [params] = useSearchParams()
  const page = Number(params.get('page')) || 1
  const base = `/api/seller/shops/${enc(shop.shop_id)}/listings`
  const listings = useLoad<Page<ListingCard>>(`${base}?${qs({ page, limit: 20 })}`)
  const categories = useLoad<Category[]>('/api/catalog/categories')

  return (
    <>
      <header className="page-head">
        <div>
          <p className="eyebrow mb-2">DANH MỤC CỬA HÀNG</p>
          <h1>Sản phẩm</h1>
          <p className="text-gray-500 mt-2">
            Quản lý nội dung, giá bán và tồn kho của {shop.display_name}.
          </p>
        </div>
        <FormDialog
          title="Thêm sản phẩm mới"
          trigger={<Button variant="primary">＋ Thêm sản phẩm</Button>}
          onSubmit={(f) => {
            const category = String(f.get('category_code') ?? '')
            return api(base, 'POST', {
              product_name: String(f.get('product_name')).trim(),
              description: String(f.get('description') ?? '').trim(),
              image_url: String(f.get('image_url') ?? '').trim(),
              category_code: category && category !== 'none' ? category : undefined,
              current_price: price(f.get('current_price')),
              currency: String(f.get('currency')).trim().toUpperCase(),
              stock_quantity: stock(f.get('stock_quantity')),
            }).then(listings.reload)
          }}
        >
          <Field label="Tên sản phẩm" name="product_name" maxLength={200} required />
          <TextArea label="Mô tả sản phẩm" name="description" maxLength={5000} />
          <Field
            label="Đường dẫn ảnh (HTTPS)"
            name="image_url"
            type="url"
            pattern="https://.*"
            maxLength={2048}
            placeholder="https://…"
          />
          <SelectField
            label="Danh mục"
            name="category_code"
            defaultValue="none"
            options={[
              ['none', 'Không có'],
              ...(categories.data ?? []).map((c): [string, string] => [
                c.category_code,
                c.category_name_en ?? c.category_code,
              ]),
            ]}
          />
          <Field
            label="Giá"
            name="current_price"
            inputMode="decimal"
            pattern={pricePattern}
            required
          />
          <Field
            label="Tiền tệ"
            name="currency"
            defaultValue="VND"
            pattern="[A-Za-z]{3}"
            required
          />
          <Field
            label="Tồn kho (để trống nếu không giới hạn)"
            name="stock_quantity"
            type="number"
            min={0}
          />
        </FormDialog>
      </header>
      <section className="seller-collection" aria-label="Danh sách sản phẩm">
        <Loaded state={listings}>
          {(res) => (
            <>
              <div className="seller-list-heading">
                <h2>
                  Tất cả sản phẩm <span className="seller-count">{res.total}</span>
                </h2>
                <span>Giá và tồn kho hiện tại</span>
              </div>
              {res.items.length === 0 ? (
                <div className="seller-empty">
                  <span aria-hidden="true">◇</span>
                  <h3>Cửa hàng chưa có sản phẩm</h3>
                  <p>Chọn “Thêm sản phẩm” để bắt đầu đăng bán.</p>
                </div>
              ) : (
                <section
                  className="seller-table-scroll"
                  aria-label="Bảng sản phẩm"
                  tabIndex={0 /* NOSONAR: S6845 - native keyboard scrolling requires focus in browsers without focusable scroll containers. */}
                >
                  <table className="seller-table seller-products-table">
                    <thead className="bg-gray-50 text-left text-gray-500">
                      <tr>
                        <th className="p-2 font-medium">Sản phẩm</th>
                        <th className="p-2 font-medium">Danh mục</th>
                        <th className="p-2 text-right font-medium">Giá</th>
                        <th className="p-2 text-right font-medium">Tồn kho</th>
                        <th className="p-2 text-right">Thao tác</th>
                      </tr>
                    </thead>
                    <tbody>
                      {res.items.map((l) => (
                        <tr key={l.listing_id} className="border-t border-gray-100">
                          <td className="p-2">
                            <div className="flex items-center gap-3">
                              <div className="cart-thumb w-14 shrink-0">
                                <ProductPlaceholder src={l.image_url} />
                              </div>
                              <div className="seller-product-name">
                                <strong>
                                  {productLabel(l.product_name, l.category_name_en, l.product_id)}
                                </strong>
                                <small>{l.listing_id}</small>
                              </div>
                            </div>
                          </td>
                          <td className="p-2">{l.category_name_en ?? '—'}</td>
                          <td className="p-2 text-right">{money(l.current_price, l.currency)}</td>
                          <td className="p-2 text-right">
                            <span
                              className={
                                l.stock_quantity === 0 ? 'seller-stock-empty' : 'seller-stock'
                              }
                            >
                              {l.stock_quantity === 0
                                ? 'Hết hàng'
                                : (l.stock_quantity ?? 'Không giới hạn')}
                            </span>
                          </td>
                          <td className="p-2 text-right">
                            <div className="seller-row-actions">
                              {l.current_price !== null && (
                                <Link
                                  to={`/products/${enc(l.listing_id)}`}
                                  className="seller-text-link"
                                >
                                  Xem ↗
                                </Link>
                              )}
                              <FormDialog
                                title="Chỉnh sửa sản phẩm"
                                trigger={<Button>Sửa</Button>}
                                onSubmit={(f) =>
                                  api(`${base}/${enc(l.listing_id)}`, 'PATCH', {
                                    product_name: String(f.get('product_name')).trim(),
                                    description: String(f.get('description') ?? '').trim(),
                                    image_url: String(f.get('image_url') ?? '').trim(),
                                    current_price: price(f.get('current_price')),
                                    stock_quantity: stock(f.get('stock_quantity')),
                                  }).then(listings.reload)
                                }
                              >
                                <p className="text-sm text-gray-500">
                                  Giá mới không đổi giá đã chốt của các đơn cũ.
                                </p>
                                <Field
                                  label="Tên sản phẩm"
                                  name="product_name"
                                  defaultValue={productLabel(
                                    l.product_name,
                                    l.category_name_en,
                                    l.product_id,
                                  )}
                                  required
                                  maxLength={200}
                                />
                                <TextArea
                                  label="Mô tả sản phẩm"
                                  name="description"
                                  defaultValue={l.description ?? ''}
                                  maxLength={5000}
                                />
                                <Field
                                  label="Đường dẫn ảnh (HTTPS)"
                                  name="image_url"
                                  defaultValue={l.image_url ?? ''}
                                  type="url"
                                  pattern="https://.*"
                                  maxLength={2048}
                                />
                                <Field
                                  label={`Giá (${l.currency})`}
                                  name="current_price"
                                  inputMode="decimal"
                                  pattern={pricePattern}
                                  defaultValue={l.current_price ?? ''}
                                  required
                                />
                                <Field
                                  label="Tồn kho"
                                  name="stock_quantity"
                                  type="number"
                                  min={0}
                                  defaultValue={l.stock_quantity ?? ''}
                                />
                              </FormDialog>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </section>
              )}
              <Pager page={res.page} totalPages={res.total_pages} />
            </>
          )}
        </Loaded>
      </section>
    </>
  )
}
