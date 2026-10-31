import { useState, type ReactNode } from 'react'
import { Dialog, DropdownMenu } from 'radix-ui'
import {
  Form,
  Link,
  Navigate,
  NavLink,
  Outlet,
  useLocation,
  useMatch,
  useNavigate,
} from 'react-router'
import { currentShop, logout, profiles, useAuthed, useLoad } from './api'
import { cart } from './cart'
import { useStore } from './store'
import { Button } from './ui'
import type { Shop } from './types'

const navCls = ({ isActive }: { isActive: boolean }) => `nav-item${isActive ? ' active' : ''}`
function Brand() {
  return (
    <Link to="/" className="brand">
      <span className="brand-mark">OF</span>
      <span>
        olist
        <small>Đơn hàng rõ ràng. Quyết định tự tin.</small>
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
  if (
    location.pathname.startsWith('/seller') ||
    location.pathname.startsWith('/demo/') ||
    location.pathname === '/login'
  )
    return <Outlet />
  return (
    <>
      <header className="market-header">
        <div className="market-topbar">
          <span>Mua sắm & kết nối cùng Olist</span>
          <div>
            <Link to="/seller/dashboard">Kênh người bán ↗</Link>
            {authed ? (
              <>
                <span>{prof?.buyer?.display_name ?? prof?.seller?.display_name}</span>
                <button onClick={() => logout().then(() => navigate('/login'))}>Đăng xuất</button>
              </>
            ) : (
              <Link to="/login">Đăng nhập</Link>
            )}
          </div>
        </div>
        <div className="market-searchbar">
          <Brand />
          <Form action="/" role="search" className="market-search">
            <label className="sr-only" htmlFor="market-search">
              Tìm kiếm sản phẩm
            </label>
            <input
              id="market-search"
              key={location.search}
              name="q"
              type="search"
              maxLength={120}
              defaultValue={new URLSearchParams(location.search).get('q') ?? ''}
              placeholder="Tìm sản phẩm, danh mục bạn yêu thích…"
            />
            <button aria-label="Tìm kiếm" type="submit">
              <svg
                aria-hidden="true"
                viewBox="0 0 24 24"
                width="18"
                height="18"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
              >
                <circle cx="10" cy="10" r="6" />
                <path d="m15 15 5 5" />
              </svg>
              <span>Tìm kiếm</span>
            </button>
          </Form>
          <Link to="/buyer/checkout" className="header-cart">
            <svg
              aria-hidden="true"
              viewBox="0 0 24 24"
              width="25"
              height="25"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
            >
              <path
                d="M2 3h3l3 13h11l3-9H6M9 20h.01M18 20h.01"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
              <circle cx="9" cy="20" r="1" />
              <circle cx="18" cy="20" r="1" />
            </svg>
            <span>
              Giỏ hàng <b>{lines.reduce((n, l) => n + l.quantity, 0)}</b>
            </span>
          </Link>
        </div>
        <nav className="market-nav" aria-label="Cửa hàng">
          <NavLink to="/" end className={navCls}>
            Khám phá sản phẩm
          </NavLink>
          <NavLink to="/shops" className={navCls}>
            Cửa hàng
          </NavLink>
          <NavLink to="/buyer/orders" className={navCls}>
            Đơn mua
          </NavLink>
          <NavLink to="/buyer/addresses" className={navCls}>
            Địa chỉ nhận hàng
          </NavLink>
        </nav>
      </header>
      <main className="buyer-main space-y-6">
        <Outlet />
      </main>
      <footer className="market-footer">
        <div>
          <strong>
            olist<span> • </span>
          </strong>
          <p>Một nơi để khám phá. Một cách mua sắm rõ ràng.</p>
        </div>
        <Link to="/shops">Cửa hàng</Link>
        <Link to="/buyer/orders">Đơn mua</Link>
        <Link to="/seller/dashboard">Kênh người bán ↗</Link>
      </footer>
    </>
  )
}

export function PortalGuard({ role, children }: { role: 'buyer' | 'seller'; children: ReactNode }) {
  const authed = useAuthed()
  const prof = useStore(profiles)?.[role]
  const location = useLocation()
  if (!authed)
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
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
      <Link to="/seller/dashboard" className="seller-brand">
        <span>
          olist<span className="seller-brand-dot">.</span>
        </span>
        <small>KÊNH NGƯỜI BÁN</small>
      </Link>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger className="shop-switch text-left">
          <span className="eyebrow">
            CỬA HÀNG ĐANG CHỌN <span aria-hidden="true">⌄</span>
          </span>
          <strong>{shop?.display_name ?? 'Chọn shop'}</strong>
          <span className="subtle">
            {shop ? `${shop.assessed_orders} đơn đã đánh giá` : 'Quản lý / tạo shop'}
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
        <p className="seller-nav-label">QUẢN LÝ CỬA HÀNG</p>
        {[
          ['/seller/dashboard', 'Tổng quan', 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z'],
          ['/seller/orders', 'Đơn hàng & review', 'M6 3h12v18H6zM9 7h6M9 11h6M9 15h4'],
          ['/seller/products', 'Sản phẩm', 'm3 7 9-4 9 4v10l-9 4-9-4zM3 7l9 5 9-5M12 12v9M8 5l9 5'],
          ['/seller/shops', 'Cửa hàng của tôi', 'M3 10h18l-2-6H5zM4 10v11h16V10M9 21v-7h6v7'],
        ].map(([to, title, path]) => (
          <NavLink key={to} to={to} className={navCls} onClick={close}>
            <svg
              width="19"
              height="19"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <path d={path} />
            </svg>
            {title}
          </NavLink>
        ))}
      </nav>
      <Link to="/" className="seller-market-link" onClick={close}>
        <span>Khám phá sàn Olist</span>
        <span aria-hidden="true">↗</span>
      </Link>
      <footer className="sidebar-footer">
        <div className="seller-account">
          <span className="seller-avatar" aria-hidden="true">
            {prof?.seller?.display_name.slice(0, 1).toUpperCase()}
          </span>
          <div>
            <strong>{prof?.seller?.display_name}</strong>
            <span className="subtle">
              {prof?.seller?.status === 'active' ? 'Đang hoạt động' : 'Tạm khóa'}
            </span>
          </div>
        </div>
        {prof?.buyer && <Link to="/buyer/orders">Chuyển sang portal người mua ↗</Link>}
        <button
          className="text-left subtle"
          onClick={() => logout().then(() => navigate('/login'))}
        >
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
      <div className="seller-workspace">
        <header className="seller-topbar">
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
          </div>
          <div className="seller-context">
            <span>Kênh người bán</span>
            <span aria-hidden="true">/</span>
            <strong>{shop?.display_name ?? 'Cửa hàng của bạn'}</strong>
          </div>
          <Link
            to={shop ? `/shops/${encodeURIComponent(shop.shop_id)}` : '/'}
            className="seller-storefront"
          >
            Xem cửa hàng <span aria-hidden="true">↗</span>
          </Link>
        </header>
        <main className="seller-main">
          <PortalGuard role="seller">
            {shop || onShops ? <Outlet /> : <Navigate to="/seller/shops" replace />}
          </PortalGuard>
        </main>
      </div>
    </div>
  )
}
