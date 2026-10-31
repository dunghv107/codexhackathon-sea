import { useSearchParams } from 'react-router'
import { api, currentShop, enc, qs, useLoad } from '../../api'
import { fromCents, money, productLabel, toCents } from '../../format'
import { useStore } from '../../store'
import type { Category, ListingCard, Page } from '../../types'
import { Button, Field, FormDialog, Loaded, Pager, SelectField } from '../../ui'

const pricePattern = '\\d+(\\.\\d{1,2})?'

// "12.5" -> "12.50": tiền gửi lên dạng string thập phân 2 chữ số.
const price = (value: FormDataEntryValue | null) => fromCents(toCents(String(value).trim()))
const stock = (value: FormDataEntryValue | null) => (String(value ?? '').trim() === '' ? undefined : Number(value))

export default function Products() {
  const shop = useStore(currentShop)!
  const [params] = useSearchParams()
  const page = Number(params.get('page')) || 1
  const base = `/api/seller/shops/${enc(shop.shop_id)}/listings`
  const listings = useLoad<Page<ListingCard>>(`${base}?${qs({ page, limit: 20 })}`)
  const categories = useLoad<Category[]>('/api/catalog/categories')

  return (
    <>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Sản phẩm</h1>
        <FormDialog
          title="Tạo listing mới"
          trigger={<Button variant="primary">Tạo listing</Button>}
          onSubmit={(f) => {
            const category = String(f.get('category_code') ?? '')
            return api(base, 'POST', {
              product_name: String(f.get('product_name')).trim(),
              category_code: category && category !== 'none' ? category : undefined,
              current_price: price(f.get('current_price')),
              currency: String(f.get('currency')).trim().toUpperCase(),
              stock_quantity: stock(f.get('stock_quantity')),
            }).then(listings.reload)
          }}
        >
          <Field label="Tên sản phẩm" name="product_name" required />
          <SelectField
            label="Danh mục"
            name="category_code"
            defaultValue="none"
            options={[
              ['none', 'Không có'],
              ...(categories.data ?? []).map((c): [string, string] => [c.category_code, c.category_name_en ?? c.category_code]),
            ]}
          />
          <Field label="Giá" name="current_price" inputMode="decimal" pattern={pricePattern} required />
          <Field label="Tiền tệ" name="currency" defaultValue="VND" pattern="[A-Za-z]{3}" required />
          <Field label="Tồn kho (để trống nếu không giới hạn)" name="stock_quantity" type="number" min={0} />
        </FormDialog>
      </div>
      <Loaded state={listings}>
        {(res) => (
          <>
            {res.items.length === 0 && <p className="text-sm text-gray-500">Shop chưa có listing.</p>}
            <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 text-left text-gray-500">
                  <tr>
                    <th className="p-2 font-medium">Sản phẩm</th>
                    <th className="p-2 font-medium">Danh mục</th>
                    <th className="p-2 text-right font-medium">Giá</th>
                    <th className="p-2 text-right font-medium">Tồn kho</th>
                    <th className="p-2" />
                  </tr>
                </thead>
                <tbody>
                  {res.items.map((l) => (
                    <tr key={l.listing_id} className="border-t border-gray-100">
                      <td className="p-2">{productLabel(l.product_name, l.category_name_en, l.product_id)}</td>
                      <td className="p-2">{l.category_name_en ?? '—'}</td>
                      <td className="p-2 text-right">{money(l.current_price, l.currency)}</td>
                      <td className="p-2 text-right">{l.stock_quantity ?? 'Chưa có dữ liệu'}</td>
                      <td className="p-2 text-right">
                        <FormDialog
                          title="Sửa giá / tồn kho"
                          trigger={<Button>Sửa</Button>}
                          onSubmit={(f) =>
                            api(`${base}/${enc(l.listing_id)}`, 'PATCH', {
                              current_price: price(f.get('current_price')),
                              stock_quantity: stock(f.get('stock_quantity')),
                            }).then(listings.reload)
                          }
                        >
                          <p className="text-sm text-gray-500">Giá mới không đổi giá đã chốt của các đơn cũ.</p>
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
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pager page={res.page} totalPages={res.total_pages} />
          </>
        )}
      </Loaded>
    </>
  )
}
