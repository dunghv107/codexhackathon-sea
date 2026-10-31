import { persisted } from './store'
import type { ListingCard } from './types'

// shortcut: giỏ hàng chỉ nằm trong localStorage của trình duyệt (chưa có bảng cart), thêm bảng khi cần đồng bộ nhiều thiết bị.
export type CartLine = { listing: ListingCard; quantity: number }
export const cart = persisted<CartLine[]>('cart', [])

// Số lượng 1..99 (giới hạn của checkout) và không vượt tồn kho nếu có.
export const maxQuantity = (l: ListingCard) => Math.min(99, l.stock_quantity ?? 99)

export function setQuantity(listing: ListingCard, quantity: number) {
  const q = Math.min(Math.max(0, Math.trunc(quantity) || 0), maxQuantity(listing))
  const lines = cart.get()
  const next = lines.some((l) => l.listing.listing_id === listing.listing_id)
    ? lines.map((l) => (l.listing.listing_id === listing.listing_id ? { listing, quantity: q } : l))
    : [...lines, { listing, quantity: q }]
  cart.set(next.filter((l) => l.quantity > 0))
}

export function addToCart(listing: ListingCard) {
  const line = cart.get().find((l) => l.listing.listing_id === listing.listing_id)
  setQuantity(listing, (line?.quantity ?? 0) + 1)
}
