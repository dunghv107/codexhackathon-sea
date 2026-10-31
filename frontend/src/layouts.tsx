import { useState, type ReactNode } from 'react'
import { Dialog, DropdownMenu } from 'radix-ui'
import { Link, Navigate, NavLink, Outlet, useLocation, useMatch, useNavigate } from 'react-router'
import { currentShop, logout, profiles, useAuthed, useLoad } from './api'
import { cart } from './cart'
import { useStore } from './store'
import { Button } from './ui'
import type { Shop } from './types'

const navCls = ({ isActive }: { isActive: boolean }) => `nav-item${isActive ? ' active' : ''}`
function Brand({ seller = false }: { seller?: boolean }) {
  return (
    <Link to="/" className="brand">
      <span className="brand-mark">OF</span>
      <span>
        Olist Fraud Review
        <small>{seller ? 'Portal người bán' : 'Đơn hàng rõ ràng. Quyết định tự tin.'}</small>
      </span>
    </Link>
  )
}

export function RootLayout() {
  const authed = useAuthed()
  const prof = useStore(profiles)
  const lines = useStore(cart)
  const navigate = useNavigate()
  const location = useLocation()
  if (location.pathname.startsWith('/seller') || location.pathname === '/login') return <Outlet />
  return (
    <>
      <header className="buyer-header">
        <nav aria-label="Cửa hàng">
          <Brand />
          <NavLink to="/" className={navCls}>
            Sản phẩm
          </NavLink>
          {prof?.buyer && (
            <>
              <NavLink to="/buyer/orders" className={navCls}>
                Đơn hàng
              </NavLink>
              <NavLink to="/buyer/addresses" className={navCls}>
                Địa chỉ
              </NavLink>
            </>
          )}
          <NavLink to="/buyer/checkout" className={navCls}>
            Giỏ hàng · {lines.reduce((n, l) => n + l.quantity, 0)}
          </NavLink>
          {prof?.seller && (
            <NavLink to="/seller/dashboard" className={navCls}>
              Portal người bán ↗
            </NavLink>
          )}
          {authed ? (
            <button className="nav-item" onClick={() => logout().then(() => navigate('/login'))}>
              Đăng xuất
            </button>
          ) : (
            <NavLink to="/login" className={navCls}>
              Đăng nhập
            </NavLink>
          )}
        </nav>
      </header>
      <main className="buyer-main space-y-6">
        <Outlet />
      </main>
    </>
  )
}

function PortalGuard({ role, children }: { role: 'buyer' | 'seller'; children: ReactNode }) {
  const authed = useAuthed()
  const prof = useStore(profiles)?.[role]
  if (!authed) return <Navigate to="/login" replace />
  if (!prof)
    return (
      <p className="card">
        Tài khoản này không có hồ sơ {role === 'buyer' ? 'người mua' : 'người bán'}.{' '}
        <Link to="/login" className="underline">
          Đăng nhập tài khoản khác
        </Link>
      </p>
    )
  return (
    <div className="space-y-5">
      {prof.status === 'suspended' && (
        <p role="alert" className="card text-red-700">
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
      <Outlet />
    </PortalGuard>
  )
}

function SellerNav({ close }: { close?: () => void }) {
  const shop = useStore(currentShop)
  const prof = useStore(profiles)
  const navigate = useNavigate()
  const shops = useLoad<Shop[]>(prof?.seller ? '/api/seller/shops' : null)
  return (
    <>
      <Brand seller />
      <DropdownMenu.Root>
        <DropdownMenu.Trigger className="shop-switch text-left">
          <span className="eyebrow">Shop đang chọn ⌄</span>
          <strong>{shop?.display_name ?? 'Chọn shop'}</strong>
          <span className="subtle">
            {shop ? `${shop.batch_id} · ${shop.assessed_orders} đơn đã đánh giá` : 'Quản lý / tạo shop'}
          </span>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            sideOffset={6}
            className="z-[70] w-64 rounded-lg border border-gray-200 bg-white p-2 shadow-lg"
          >
            {shops.error && (
              <p role="alert" className="p-2 text-xs text-red-700">
                {shops.error}
              </p>
            )}
            {shops.data?.map((s) => (
              <DropdownMenu.Item
                key={s.shop_id}
                className="rounded-md px-3 py-2 cursor-pointer outline-none data-highlighted:bg-gray-50"
                onSelect={() => {
                  currentShop.set(s)
                  navigate('/seller/dashboard')
                  close?.()
                }}
              >
                <span className="block font-medium">
                  {s.display_name}
                  {s.shop_id === shop?.shop_id ? ' ✓' : ''}
                </span>
                <span className="subtle">
                  {s.batch_id} · {s.assessed_orders} đã đánh giá
                </span>
              </DropdownMenu.Item>
            ))}
            <DropdownMenu.Separator className="h-px bg-gray-200 my-2" />
            <DropdownMenu.Item asChild>
              <Link className="block p-2 text-blue-700" to="/seller/shops" onClick={close}>
                Quản lý / tạo shop →
              </Link>
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <nav aria-label="Người bán">
        <NavLink to="/seller/dashboard" className={navCls} onClick={close}>
          ◫ &nbsp; Tổng quan
        </NavLink>
        <NavLink to="/seller/orders" className={navCls} onClick={close}>
          ☷ &nbsp; Hàng đợi review
        </NavLink>
        <NavLink to="/seller/products" className={navCls} onClick={close}>
          ◇ &nbsp; Sản phẩm
        </NavLink>
      </nav>
      <footer className="sidebar-footer">
        <strong>{prof?.seller?.display_name}</strong>
        <span className="subtle">{prof?.seller?.status === 'active' ? '● Đang hoạt động' : 'Tạm khóa'}</span>
        {prof?.buyer && <Link to="/buyer/orders">Chuyển sang portal người mua ↗</Link>}
        <button className="text-left subtle" onClick={() => logout().then(() => navigate('/login'))}>
          Đăng xuất
        </button>
      </footer>
    </>
  )
}
export function SellerLayout() {
  const shop = useStore(currentShop)
  const onShops = useMatch('/seller/shops')
  const [open, setOpen] = useState(false)
  return (
    <div className="seller-shell">
      <aside className="seller-sidebar">
        <SellerNav />
      </aside>
      <main className="seller-main">
        <div>
          <div className="seller-mobile">
            <Dialog.Root open={open} onOpenChange={setOpen}>
              <Dialog.Trigger asChild>
                <Button aria-label="Mở điều hướng">☰</Button>
              </Dialog.Trigger>
              <Dialog.Portal>
                <Dialog.Overlay className="dialog-overlay" />
                <Dialog.Content
                  aria-describedby={undefined}
                  className="seller-sidebar fixed! z-[61] left-0 top-0"
                >
                  <Dialog.Title className="sr-only">Điều hướng người bán</Dialog.Title>
                  <Dialog.Close className="absolute right-2 top-1" aria-label="Đóng điều hướng">
                    ×
                  </Dialog.Close>
                  <SellerNav close={() => setOpen(false)} />
                </Dialog.Content>
              </Dialog.Portal>
            </Dialog.Root>
            <span>Portal người bán</span>
          </div>
          <PortalGuard role="seller">
            {shop || onShops ? <Outlet /> : <Navigate to="/seller/shops" replace />}
          </PortalGuard>
        </div>
      </main>
    </div>
  )
}
