import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { enc, useLoad } from '../api'
import { cart, maxQuantity, setQuantity } from '../cart'
import { money, productLabel } from '../format'
import { useStore } from '../store'
import type { ListingCard } from '../types'
import { Button, Loaded, ProductPlaceholder, Section } from '../ui'

export default function ProductDetail() {
  const { listingId = '' } = useParams()
  const listing = useLoad<ListingCard>(`/api/catalog/listings/${enc(listingId)}`)
  const lines = useStore(cart)
  const navigate = useNavigate()
  const [quantity, setCount] = useState(1)
  const [notice, setNotice] = useState('')
  return (
    <Loaded state={listing}>
      {(l) => {
        const name = productLabel(l.product_name, l.category_name_en, l.product_id)
        const existing = lines.find((x) => x.listing.listing_id === l.listing_id)?.quantity ?? 0
        const available = Math.max(0, maxQuantity(l) - existing)
        const count = Math.min(quantity, available)
        const add = (checkout = false) => {
          if (count < 1) return
          setQuantity(l, existing + count)
          if (checkout) navigate(`/buyer/checkout?only=${enc(l.listing_id)}`)
          else setNotice(`Đã thêm ${count} sản phẩm vào giỏ hàng`)
        }
        return (
          <>
            <nav className="breadcrumbs" aria-label="Đường dẫn">
              <Link to="/">Trang chủ</Link>
              <span>/</span>
              <Link to={`/shops/${enc(l.shop_id)}`}>{l.shop_name}</Link>
              <span>/</span>
              <span>{name}</span>
            </nav>
            <section className="product-detail card">
              <div className="detail-image">
                <ProductPlaceholder src={l.image_url} alt={name} />
              </div>
              <div className="detail-info">
                <Link to={`/shops/${enc(l.shop_id)}`} className="eyebrow">
                  {l.shop_name} ↗
                </Link>
                <h1>{name}</h1>
                <span className="badge">{l.category_name_en ?? 'Sản phẩm tại cửa hàng'}</span>
                <div className="detail-price">{money(l.current_price, l.currency)}</div>
                <dl className="detail-facts">
                  <div>
                    <dt>Tồn kho</dt>
                    <dd>{l.stock_quantity === null ? 'Chưa cập nhật' : `${l.stock_quantity} sản phẩm`}</dd>
                  </div>
                  <div>
                    <dt>Tiền tệ</dt>
                    <dd>{l.currency} · giữ nguyên theo sản phẩm</dd>
                  </div>
                  <div>
                    <dt>Đã thêm vào giỏ</dt>
                    <dd>{existing} sản phẩm</dd>
                  </div>
                </dl>
                <div className="quantity-picker">
                  <span>Số lượng</span>
                  <Button
                    aria-label="Giảm số lượng"
                    disabled={count <= 1}
                    onClick={() => setCount(count - 1)}
                  >
                    −
                  </Button>
                  <input
                    aria-label="Số lượng"
                    type="number"
                    min={1}
                    max={available}
                    value={count || 1}
                    disabled={!available}
                    onChange={(e) => setCount(Math.max(1, Math.min(available, Math.trunc(e.target.valueAsNumber) || 1)))}
                  />
                  <Button
                    aria-label="Tăng số lượng"
                    disabled={count >= available}
                    onClick={() => setCount(count + 1)}
                  >
                    +
                  </Button>
                </div>
                <div className="detail-actions">
                  <Button disabled={!available} onClick={() => add()}>
                    ＋ Thêm vào giỏ hàng
                  </Button>
                  <Button variant="primary" disabled={!available} onClick={() => add(true)}>
                    Mua ngay →
                  </Button>
                </div>
                {!available && (
                  <p className="subtle">
                    {l.stock_quantity === 0
                      ? 'Sản phẩm tạm hết hàng.'
                      : 'Đã đạt số lượng tối đa trong giỏ hàng.'}
                  </p>
                )}
                <p role="status" className="text-sm text-green-800">
                  {notice}
                  {notice && (
                    <Link to="/buyer/checkout" className="underline ml-2">
                      Xem giỏ hàng
                    </Link>
                  )}
                </p>
              </div>
            </section>
            <section className="shop-strip">
              <div className="shop-avatar">{l.shop_name.slice(0, 1)}</div>
              <div>
                <h2>{l.shop_name}</h2>
                <p className="subtle">Khám phá thêm sản phẩm từ cửa hàng này</p>
              </div>
              <Link className="hero-secondary ml-auto" to={`/shops/${enc(l.shop_id)}`}>
                Xem cửa hàng →
              </Link>
            </section>
            <Section title="Mô tả sản phẩm">
              <p className="whitespace-pre-wrap leading-7">
                {l.description || 'Cửa hàng chưa bổ sung mô tả cho sản phẩm này.'}
              </p>
            </Section>
          </>
        )
      }}
    </Loaded>
  )
}
