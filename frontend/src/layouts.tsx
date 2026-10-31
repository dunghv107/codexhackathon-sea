import type { ReactNode } from 'react'
import { Link, Navigate, NavLink, Outlet, useMatch, useNavigate } from 'react-router'
import { currentShop, logout, profiles, useAuthed } from './api'
import { cart } from './cart'
import { useStore } from './store'

const navCls = ({ isActive }: { isActive: boolean }) =>
  `rounded px-2 py-1 text-sm focus-visible:outline-2 focus-visible:outline-blue-500 ${isActive ? 'bg-gray-900 text-white' : 'hover:bg-gray-100'}`

export function RootLayout() {
  const authed = useAuthed()
  const prof = useStore(profiles)
  const lines = useStore(cart)
  const navigate = useNavigate()
  const count = lines.reduce((n, l) => n + l.quantity, 0)

  return (
    <div className="min-h-screen bg-gray-50 text-gray-900">
      <header className="border-b border-gray-200 bg-white">
        <nav className="mx-auto flex max-w-6xl flex-wrap items-center gap-2 px-4 py-3">
          <Link to="/" className="mr-auto text-lg font-semibold">
            Sàn mẫu
          </Link>
          <NavLink to="/buyer/checkout" className={navCls}>
            Giỏ hàng ({count})
          </NavLink>
          {authed && prof?.buyer && (
            <NavLink to="/buyer/orders" className={navCls}>
              Người mua
            </NavLink>
          )}
          {authed && prof?.seller && (
            <NavLink to="/seller/dashboard" className={navCls}>
              Người bán
            </NavLink>
          )}
          {authed ? (
            <button
              className="rounded px-2 py-1 text-sm hover:bg-gray-100 focus-visible:outline-2 focus-visible:outline-blue-500"
              onClick={() => logout().then(() => navigate('/login'))}
            >
              Đăng xuất
            </button>
          ) : (
            <NavLink to="/login" className={navCls}>
              Đăng nhập
            </NavLink>
          )}
        </nav>
      </header>
      <main className="mx-auto max-w-6xl space-y-6 px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}

// Chỉ hỗ trợ UX: server vẫn tự kiểm quyền buyer/seller từ JWT.
function PortalGuard({ role, children }: { role: 'buyer' | 'seller'; children: ReactNode }) {
  const authed = useAuthed()
  const prof = useStore(profiles)?.[role]
  if (!authed) return <Navigate to="/login" replace />
  if (!prof) {
    return (
      <p className="text-sm">
        Tài khoản này không có hồ sơ {role === 'buyer' ? 'người mua' : 'người bán'}.{' '}
        <Link to="/login" className="underline">
          Đăng nhập tài khoản khác
        </Link>
      </p>
    )
  }
  return (
    <div className="space-y-4">
      {prof.status === 'suspended' && (
        <p role="alert" className="text-sm text-red-600">
          Tài khoản đang bị tạm khóa: chỉ xem được dữ liệu, không thể thay đổi.
        </p>
      )}
      {children}
    </div>
  )
}

export function BuyerLayout() {
  return (
    <PortalGuard role="buyer">
      <nav aria-label="Người mua" className="flex flex-wrap gap-2">
        <NavLink to="/buyer/orders" className={navCls}>
          Đơn hàng
        </NavLink>
        <NavLink to="/buyer/addresses" className={navCls}>
          Địa chỉ
        </NavLink>
        <NavLink to="/buyer/checkout" className={navCls}>
          Thanh toán
        </NavLink>
      </nav>
      <Outlet />
    </PortalGuard>
  )
}

export function SellerLayout() {
  const shop = useStore(currentShop)
  const onShops = useMatch('/seller/shops')
  return (
    <PortalGuard role="seller">
      <nav aria-label="Người bán" className="flex flex-wrap items-center gap-2">
        <NavLink to="/seller/shops" className={navCls}>
          Shop{shop ? `: ${shop.display_name}` : ''}
        </NavLink>
        <NavLink to="/seller/dashboard" className={navCls}>
          Tổng quan
        </NavLink>
        <NavLink to="/seller/products" className={navCls}>
          Sản phẩm
        </NavLink>
        <NavLink to="/seller/orders" className={navCls}>
          Đơn hàng
        </NavLink>
      </nav>
      {shop || onShops ? <Outlet /> : <Navigate to="/seller/shops" replace />}
    </PortalGuard>
  )
}
