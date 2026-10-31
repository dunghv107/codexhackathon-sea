import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { api, currentShop, errMsg, profiles, tokens } from '../api'
import type { Profiles } from '../types'
import { Button, ErrorText, Field } from '../ui'

// Đăng nhập chung; có cả hai hồ sơ thì cho người dùng chọn portal.
export default function Login() {
  const navigate = useNavigate()
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
      tokens.set({ access_token: res.access_token, refresh_token: res.refresh_token })
      const p = await api<Profiles>('/api/profiles')
      profiles.set(p)
      if (p.buyer && p.seller) setChoice(true)
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
    <div className="mx-auto mt-12 max-w-sm space-y-6">
      <h1 className="text-2xl font-semibold">Đăng nhập</h1>
      {choice ? (
        <div className="space-y-3">
          <p className="text-sm">Tài khoản có cả hồ sơ người mua và người bán. Vào portal nào?</p>
          <div className="flex gap-2">
            <Button variant="primary" onClick={() => navigate('/buyer/orders')}>
              Người mua
            </Button>
            <Button variant="primary" onClick={() => navigate('/seller/shops')}>
              Người bán
            </Button>
          </div>
        </div>
      ) : (
        <form onSubmit={login} className="space-y-4">
          <Field label="Tên đăng nhập" name="username" autoComplete="username" required />
          <Field label="Mật khẩu" name="password" type="password" autoComplete="current-password" required />
          {error && <ErrorText>{error}</ErrorText>}
          <Button variant="primary" className="w-full" disabled={busy}>
            {busy ? 'Đang đăng nhập…' : 'Đăng nhập'}
          </Button>
        </form>
      )}
    </div>
  )
}
