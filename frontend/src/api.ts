import { useEffect, useState } from 'react'
import { persisted, useStore } from './store'
import type { Profiles, Shop } from './types'

type Tokens = { access_token: string; refresh_token?: string }
type Envelope<T> = { is_success: boolean; data: T; message?: string; error?: { code: string; message: string } }

export const tokens = persisted<Tokens | null>('auth_tokens', null)
export const profiles = persisted<Profiles | null>('auth_profiles', null)
export const currentShop = persisted<Shop | null>('seller_shop', null)

export const useAuthed = () => useStore(tokens) !== null

export class ApiError extends Error {
  code?: string
  status: number
  constructor(message: string, status: number, code?: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))

// ID có thể chứa ':' nên luôn encode khi ghép vào path.
export const enc = encodeURIComponent

// Bỏ các tham số rỗng khỏi query string.
export const qs = (params: Record<string, string | number | null | undefined>) =>
  new URLSearchParams(
    Object.entries(params).flatMap(([k, v]) => (v === null || v === undefined || v === '' ? [] : [[k, String(v)]])),
  ).toString()

function send(path: string, method: string, body: unknown) {
  const access = tokens.get()?.access_token
  return fetch(path, {
    method,
    headers: {
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(access ? { Authorization: `Bearer ${access}` } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

// Các request 401 cùng lúc dùng chung một lần refresh, vì refresh token có thể bị xoay vòng.
// false = refresh token hết hiệu lực (401). Lỗi tạm thời (429, mạng) thì ném ra và giữ phiên đăng nhập.
let refreshing: Promise<boolean> | null = null
function refresh() {
  const refreshToken = tokens.get()?.refresh_token
  if (!refreshToken) return Promise.resolve(false)
  refreshing ??= send('/api/auth/refresh', 'POST', { refresh_token: refreshToken })
    .then(async (res) => {
      const body = (await res.json().catch(() => null)) as Envelope<Tokens> | null
      if (res.status === 401) return false
      if (!res.ok || !body?.is_success) {
        throw new ApiError(body?.error?.message ?? `Lỗi máy chủ (${res.status})`, res.status, body?.error?.code)
      }
      tokens.set({ access_token: body.data.access_token, refresh_token: body.data.refresh_token ?? refreshToken })
      return true
    })
    .finally(() => (refreshing = null))
  return refreshing
}

// Gọi API, gỡ envelope và ném ApiError với error.message của server.
export async function api<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  let res = await send(path, method, body)
  if (res.status === 401 && tokens.get()) {
    if (await refresh()) res = await send(path, method, body)
    if (res.status === 401) tokens.set(null)
  }
  if (res.status === 204) return undefined as T
  const env = (await res.json().catch(() => null)) as Envelope<T> | null
  if (!res.ok || !env?.is_success) {
    throw new ApiError(env?.error?.message ?? `Lỗi máy chủ (${res.status})`, res.status, env?.error?.code)
  }
  return env.data
}

// Tải dữ liệu GET theo path; path null thì không gọi. reload() để tải lại sau khi ghi.
export function useLoad<T>(path: string | null) {
  const [state, setState] = useState<{ path?: string; data?: T; error?: string }>({})
  const [version, setVersion] = useState(0)
  useEffect(() => {
    if (!path) return
    let alive = true
    api<T>(path).then(
      (data) => alive && setState({ path, data }),
      (e) => alive && setState({ path, error: errMsg(e) }),
    )
    return () => {
      alive = false
    }
  }, [path, version])
  const fresh = state.path === path
  return {
    data: fresh ? state.data : undefined,
    error: fresh ? state.error : undefined,
    reload: () => setVersion((v) => v + 1),
  }
}

export async function logout() {
  await api('/api/auth/logout', 'POST').catch(() => {})
  tokens.set(null)
  profiles.set(null)
  currentShop.set(null)
}
