import { useState, type FormEvent } from 'react'
import { useNavigate, useLocation } from 'react-router'
import { api, currentShop, errMsg, profiles, tokens } from '../api'
import type { Profiles } from '../types'
import { Button, ErrorText, Field } from '../ui'

// Đăng nhập chung; có cả hai hồ sơ thì cho người dùng chọn portal.
export default function Login() {
  const navigate = useNavigate()
  const location = useLocation()
  const returnTo =
    typeof location.state?.from === 'string' && /^\/(buyer|seller)(\/|$)/.test(location.state.from)
      ? location.state.from
      : ''
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [choice, setChoice] = useState(false)

  async function login(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    setError('')
    // Bỏ phiên cũ để request đăng nhập không mang token cũ.
    tokens.set(null)
    profiles.set(null)
    currentShop.set(null)
    try {
      const res = await api<{ access_token: string; refresh_token?: string }>(
        '/api/auth/login',
        'POST',
        Object.fromEntries(new FormData(e.currentTarget)),
      )
      tokens.set({
        access_token: res.access_token,
        refresh_token: res.refresh_token,
      })
      const p = await api<Profiles>('/api/profiles')
      profiles.set(p)
      if ((p.buyer && returnTo.startsWith('/buyer/')) || (p.seller && returnTo.startsWith('/seller/')))
        navigate(returnTo)
      else if (p.buyer && p.seller) setChoice(true)
      else if (p.seller) navigate('/seller/shops')
      else if (p.buyer) navigate('/buyer/orders')
      else setError('Tài khoản chưa có hồ sơ người mua hoặc người bán.')
    } catch (err) {
      setError(errMsg(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-shell">
      <aside className="login-brand">
        <div className="brand">
          <span className="brand-mark">OF</span> Olist Fraud Review
        </div>
        <div>
          <p className="mb-6 text-amber-400">OLIST · SELLER & BUYER PORTAL</p>
          <h2>
            Mỗi đơn hàng.
            <br />
            Một quyết định rõ ràng.
          </h2>
        </div>
        <p className="text-sm text-stone-400">Theo dõi đơn hàng, kiểm tra rủi ro và quản lý cửa hàng.</p>
      </aside>
      <main className="login-form">
        <div className="space-y-6">
          <div>
            <p className="eyebrow mb-3">Chào mừng trở lại</p>
            <h1>{choice ? 'Chọn không gian làm việc' : 'Đăng nhập'}</h1>
            <p className="mt-2 text-sm text-gray-500">Tiếp tục với tài khoản của bạn.</p>
          </div>
          {choice ? (
            <div className="space-y-3">
              <p className="text-sm">Tài khoản có cả hồ sơ người mua và người bán. Vào portal nào?</p>
              <div className="grid gap-3">
                <Button className="text-left p-5!" onClick={() => navigate('/buyer/orders')}>
                  <span className="block text-lg mb-2">◇ Người mua →</span>
                  <span className="subtle">Khám phá sản phẩm và theo dõi đơn hàng.</span>
                </Button>
                <Button className="text-left p-5!" onClick={() => navigate('/seller/shops')}>
                  <span className="block text-lg mb-2">◫ Người bán →</span>
                  <span className="subtle">Quản lý cửa hàng và kiểm tra rủi ro đơn hàng.</span>
                </Button>
              </div>
            </div>
          ) : (
            <form onSubmit={login} className="space-y-4">
              <Field label="Tên đăng nhập" name="username" autoComplete="username" required />
              <Field
                label="Mật khẩu"
                name="password"
                type="password"
                autoComplete="current-password"
                required
              />
              {error && <ErrorText>{error}</ErrorText>}
              <Button variant="primary" className="w-full" disabled={busy}>
                {busy ? 'Đang đăng nhập…' : 'Đăng nhập'}
              </Button>
            </form>
          )}
        </div>
      </main>
    </div>
  )
}
