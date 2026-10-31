import { useNavigate } from 'react-router'
import { api, currentShop, useLoad } from '../../api'
import { useStore } from '../../store'
import type { Shop } from '../../types'
import { Button, Field, FormDialog, Loaded } from '../../ui'

export default function Shops() {
  const shops = useLoad<Shop[]>('/api/seller/shops')
  const current = useStore(currentShop)
  const navigate = useNavigate()

  return (
    <>
      <header className="page-head">
        <div>
          <p className="eyebrow mb-2">KHÔNG GIAN KINH DOANH</p>
          <h1>Cửa hàng của tôi</h1>
          <p className="text-gray-500 mt-2">
            Chọn cửa hàng để quản lý sản phẩm, đơn hàng và hoạt động review.
          </p>
        </div>
        <FormDialog
          title="Tạo cửa hàng"
          trigger={<Button variant="primary">＋ Tạo cửa hàng</Button>}
          onSubmit={(f) =>
            api<Shop>('/api/seller/shops', 'POST', {
              display_name: String(f.get('display_name')).trim(),
            }).then(shops.reload)
          }
        >
          <Field label="Tên cửa hàng" name="display_name" required />
        </FormDialog>
      </header>
      <Loaded state={shops}>
        {(list) =>
          list.length === 0 ? (
            <div className="card seller-empty">
              <span aria-hidden="true">◇</span>
              <h3>Bắt đầu với cửa hàng đầu tiên</h3>
              <p>Tạo cửa hàng để đăng sản phẩm và tiếp nhận đơn hàng.</p>
            </div>
          ) : (
            <ul className="seller-shop-grid">
              {list.map((s) => (
                <li
                  key={s.shop_id}
                  className="card seller-shop-card"
                  data-selected={current?.shop_id === s.shop_id}
                >
                  <div className="seller-shop-card-top">
                    <span className="seller-shop-avatar" aria-hidden="true">
                      {s.display_name.slice(0, 1).toUpperCase()}
                    </span>
                    {current?.shop_id === s.shop_id && (
                      <span className="seller-selected-tag">Đang quản lý</span>
                    )}
                  </div>
                  <div>
                    <h2>{s.display_name}</h2>
                    <code className="subtle">{s.shop_id}</code>
                  </div>
                  <div className="seller-shop-meta">
                    <span>{s.batch_id}</span>
                    <span>
                      <strong>{s.assessed_orders}</strong> đơn đã đánh giá
                    </span>
                  </div>
                  {current?.shop_id === s.shop_id ? (
                    <Button variant="primary" onClick={() => navigate('/seller/dashboard')}>
                      Quản lý cửa hàng →
                    </Button>
                  ) : (
                    <Button
                      onClick={() => {
                        currentShop.set(s)
                        navigate('/seller/dashboard')
                      }}
                    >
                      Chọn cửa hàng →
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )
        }
      </Loaded>
    </>
  )
}
