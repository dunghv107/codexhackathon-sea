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
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Shop của tôi</h1>
        <FormDialog
          title="Tạo shop"
          trigger={<Button variant="primary">Tạo shop</Button>}
          onSubmit={(f) =>
            api<Shop>('/api/seller/shops', 'POST', { display_name: String(f.get('display_name')).trim() }).then(shops.reload)
          }
        >
          <Field label="Tên shop" name="display_name" required />
        </FormDialog>
      </div>
      <Loaded state={shops}>
        {(list) =>
          list.length === 0 ? (
            <p className="text-sm text-gray-500">Chưa có shop nào.</p>
          ) : (
            <ul className="space-y-2">
              {list.map((s) => (
                <li
                  key={s.shop_id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white p-4"
                >
                  <span>
                    <span className="font-medium">{s.display_name}</span>
                    <span className="block text-xs text-gray-500">{s.shop_id}</span>
                  </span>
                  {current?.shop_id === s.shop_id ? (
                    <span className="text-sm text-green-700">Đang chọn</span>
                  ) : (
                    <Button
                      onClick={() => {
                        currentShop.set(s)
                        navigate('/seller/dashboard')
                      }}
                    >
                      Chọn
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
