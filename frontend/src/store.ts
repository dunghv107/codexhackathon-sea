import { useSyncExternalStore } from 'react'

// Một giá trị lưu trong localStorage và báo cho các component đang dùng khi đổi.
// localStorage có thể ném lỗi (private mode, bị chặn) nên mọi lần đọc/ghi đều try/catch.
export function persisted<T>(key: string, initial: T) {
  let value = initial
  try {
    const raw = localStorage.getItem(key)
    if (raw) value = JSON.parse(raw) as T
  } catch {
    // giữ giá trị mặc định
  }
  const listeners = new Set<() => void>()
  // Tab khác ghi cùng key (vd. refresh token vừa xoay vòng, đăng xuất) thì cập nhật theo, tránh giữ token cũ.
  window.addEventListener('storage', (e) => {
    if (e.key !== key) return
    try {
      value = e.newValue ? (JSON.parse(e.newValue) as T) : initial
    } catch {
      return
    }
    listeners.forEach((l) => l())
  })
  return {
    get: () => value,
    set(next: T) {
      value = next
      try {
        localStorage.setItem(key, JSON.stringify(next))
      } catch {
        // chỉ còn trong bộ nhớ cho tới khi tải lại trang
      }
      listeners.forEach((l) => l())
    },
    subscribe(l: () => void) {
      listeners.add(l)
      return () => void listeners.delete(l)
    },
  }
}

export function useStore<T>(store: ReturnType<typeof persisted<T>>) {
  return useSyncExternalStore(store.subscribe, store.get)
}
