import { useState } from 'react'
import { Link, useLocation } from 'react-router'
import { api, enc, errMsg, useLoad } from '../../api'
import { cart, maxQuantity, setQuantity } from '../../cart'
import { fromCents, money, paymentMethodLabel, productLabel, toCents } from '../../format'
import { useStore } from '../../store'
import type { CheckoutResult, SavedAddress } from '../../types'
import { Button, ErrorText, Field, Loaded, Section, SelectField, ProductPlaceholder } from '../../ui'

const methods = ['cod', 'credit_card', 'debit_card', 'boleto', 'voucher']

export default function Checkout() {
  const lines = useStore(cart)
  const location = useLocation()
  const [selectedIds, setSelectedIds] = useState<string[]>(() => {
    const only = new URLSearchParams(location.search).get('only') ?? location.state?.only
    return lines.filter((l) => !only || l.listing.listing_id === only).map((l) => l.listing.listing_id)
  })
  // Cart updates from another tab must not select additional purchases.
  const selectedLines = lines.filter((l) => selectedIds.includes(l.listing.listing_id))
  const toggle = (id: string, checked: boolean) =>
    setSelectedIds((prev) => (checked ? [...prev, id] : prev.filter((x) => x !== id)))
  const addresses = useLoad<SavedAddress[]>('/api/buyer/addresses')
  const [addressId, setAddressId] = useState<string>()
  const [method, setMethod] = useState('cod')
  const [phone, setPhone] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<CheckoutResult>()

  const list = addresses.data ?? []
  const selected = addressId ?? (list.find((a) => a.is_default) ?? list[0])?.saved_address_id
  const currencies = [...new Set(selectedLines.map((l) => l.listing.currency))]
  // Tổng tạm tính; tổng chính thức do server tính lại theo giá hiện tại.
  const estimate = fromCents(
    selectedLines.reduce((n, l) => n + toCents(l.listing.current_price ?? '0') * l.quantity, 0),
  )

  async function placeOrder() {
    if (
      busy ||
      selectedLines.length === 0 ||
      currencies.length !== 1 ||
      !list.some((a) => a.saved_address_id === selected)
    )
      return
    setBusy(true)
    setError('')
    try {
      const res = await api<CheckoutResult>('/api/buyer/checkouts', 'POST', {
        saved_address_id: selected,
        payment_method: method,
        phone,
        items: selectedLines.map((l) => ({
          listing_id: l.listing.listing_id,
          quantity: l.quantity,
        })),
      })
      const purchased = new Set(selectedLines.map((l) => l.listing.listing_id))
      cart.set(cart.get().filter((l) => !purchased.has(l.listing.listing_id)))
      setResult(res)
    } catch (e) {
      setError(errMsg(e))
    } finally {
      setBusy(false)
    }
  }

  if (result) {
    return (
      <Section title="Đặt hàng thành công">
        <p className="text-sm">
          Mã thanh toán {result.checkout_id} · {money(result.total_amount, result.currency)}
        </p>
        <ul className="space-y-1 text-sm">
          {result.orders.map((o) => (
            <li key={o.order_id}>
              <Link to={`/buyer/orders/${enc(o.order_id)}`} className="text-blue-700 hover:underline">
                {o.shop_name}: {money(o.order_total, result.currency)}
              </Link>
            </li>
          ))}
        </ul>
      </Section>
    )
  }

  return (
    <>
      <h1 className="text-2xl font-semibold">Giỏ hàng và thanh toán</h1>
      <div className="checkout-grid">
        <Section title="Giỏ hàng">
          {lines.length > 0 && (
            <div className="selection-bar">
              <label>
                <input
                  type="checkbox"
                  disabled={busy}
                  checked={selectedLines.length === lines.length}
                  onChange={(e) =>
                    setSelectedIds(e.target.checked ? lines.map((l) => l.listing.listing_id) : [])
                  }
                />
                Chọn tất cả ({lines.length})
              </label>
              <span>{selectedLines.length} mặt hàng đã chọn</span>
            </div>
          )}
          {lines.length === 0 ? (
            <p className="text-sm text-gray-500">
              Giỏ hàng trống.{' '}
              <Link to="/" className="text-blue-700 hover:underline">
                Xem sản phẩm
              </Link>
            </p>
          ) : (
            <ul className="divide-y divide-gray-100">
              {[...new Set(lines.map((line) => line.listing.shop_id))].map((shopId) => (
                <li key={shopId} className="py-4">
                  <h2 className="font-medium mb-3">
                    <Link to={`/shops/${enc(shopId)}`}>
                      ◇ {lines.find((line) => line.listing.shop_id === shopId)!.listing.shop_name} ↗
                    </Link>
                  </h2>
                  <ul>
                    {lines
                      .filter((line) => line.listing.shop_id === shopId)
                      .map(({ listing: l, quantity }) => (
                        <li key={l.listing_id} className="flex flex-wrap items-center gap-3 py-2 text-sm">
                          <input
                            type="checkbox"
                            disabled={busy}
                            aria-label={`Chọn ${productLabel(l.product_name, l.category_name_en ?? l.category_code)}`}
                            checked={selectedIds.includes(l.listing_id)}
                            onChange={(e) => toggle(l.listing_id, e.target.checked)}
                          />
                          <div className="cart-thumb w-16 shrink-0">
                            <ProductPlaceholder
                              productId={l.product_id}
                              src={l.image_url}
                              alt={productLabel(l.product_name, l.category_name_en ?? l.category_code)}
                            />
                          </div>
                          <span className="flex-1">
                            <Link to={`/products/${enc(l.listing_id)}`}>
                              {productLabel(l.product_name, l.category_name_en ?? l.category_code)}
                            </Link>
                            <span className="block text-xs text-gray-500">{l.shop_name}</span>
                          </span>
                          <span>{money(l.current_price, l.currency)}</span>
                          <Button
                            aria-label={`Giảm số lượng ${productLabel(l.product_name, l.category_name_en ?? l.category_code)}`}
                            disabled={quantity <= 1 || busy}
                            onClick={() => setQuantity(l, quantity - 1)}
                          >
                            −
                          </Button>
                          <input
                            disabled={busy}
                            type="number"
                            aria-label={`Số lượng ${productLabel(l.product_name, l.category_name_en ?? l.category_code)}`}
                            min={1}
                            max={maxQuantity(l)}
                            value={quantity}
                            onChange={(e) => setQuantity(l, Math.max(1, e.target.valueAsNumber || 1))}
                            className="w-16 rounded-md border border-gray-300 px-2 py-1 focus:outline-2 focus:outline-blue-500"
                          />
                          <Button
                            aria-label={`Tăng số lượng ${productLabel(l.product_name, l.category_name_en ?? l.category_code)}`}
                            disabled={quantity >= maxQuantity(l) || busy}
                            onClick={() => setQuantity(l, quantity + 1)}
                          >
                            +
                          </Button>
                          <Button disabled={busy} variant="danger" onClick={() => setQuantity(l, 0)}>
                            Xóa
                          </Button>
                        </li>
                      ))}
                  </ul>
                </li>
              ))}
            </ul>
          )}
          {currencies.length === 1 && (
            <p className="text-right font-semibold">Tạm tính: {money(estimate, currencies[0])}</p>
          )}
          {currencies.length > 1 && (
            <ErrorText>
              Các sản phẩm đã chọn có nhiều loại tiền tệ. Hãy chọn cùng BRL hoặc cùng VND để đặt riêng từng
              lần.
            </ErrorText>
          )}
        </Section>

        <Section title="Giao hàng và thanh toán">
          <Link to="/buyer/addresses" className="text-blue-700 text-sm">
            Quản lý / thêm địa chỉ →
          </Link>
          <Loaded state={addresses}>
            {(addrs) =>
              addrs.length === 0 ? (
                <p className="text-sm">
                  Chưa có địa chỉ.{' '}
                  <Link to="/buyer/addresses" className="text-blue-700 hover:underline">
                    Thêm địa chỉ
                  </Link>
                </p>
              ) : (
                <fieldset className="space-y-3" disabled={busy}>
                  <legend className="text-sm font-medium mb-3">Địa chỉ nhận hàng</legend>
                  {addrs.map((a) => (
                    <label className="address-choice" key={a.saved_address_id}>
                      <span className="sr-only">Giao đến địa chỉ</span>
                      <input
                        type="radio"
                        name="shipping-address"
                        value={a.saved_address_id}
                        checked={selected === a.saved_address_id}
                        onChange={() => setAddressId(a.saved_address_id)}
                      />
                      <span>
                        <strong className="block text-sm font-medium">
                          {a.label}
                          {a.is_default ? ' · Mặc định' : ''}
                        </strong>
                        <span className="subtle">
                          {a.address_raw}, {a.region}
                        </span>
                      </span>
                    </label>
                  ))}
                </fieldset>
              )
            }
          </Loaded>
          <Field
            label="Số điện thoại nhận hàng"
            type="tel"
            autoComplete="tel"
            required
            maxLength={40}
            value={phone}
            disabled={busy}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="09… hoặc +84…"
          />
          <SelectField
            disabled={busy}
            label="Phương thức thanh toán"
            value={method}
            onValueChange={setMethod}
            options={methods.map((m) => [m, paymentMethodLabel[m]])}
          />
          <p className="subtle">
            Mỗi shop tạo một đơn riêng. Tổng tiền chính thức được xác nhận khi đặt hàng.
          </p>
          <div className="checkout-summary">
            <span>Tạm tính · {selectedLines.length} mặt hàng</span>
            <strong>{currencies.length === 1 ? money(estimate, currencies[0]) : '—'}</strong>
          </div>
          {error && <ErrorText>{error}</ErrorText>}
          <Button
            variant="primary"
            disabled={
              busy ||
              !phone.trim() ||
              selectedLines.length === 0 ||
              !list.some((a) => a.saved_address_id === selected) ||
              currencies.length !== 1
            }
            onClick={placeOrder}
          >
            {busy ? 'Đang đặt hàng…' : `Đặt hàng (${selectedLines.length})`}
          </Button>
        </Section>
      </div>
    </>
  )
}
