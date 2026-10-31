export function money(amount: string | null, currency: string) {
  if (amount === null) return 'Chưa có giá'
  try {
    return new Intl.NumberFormat('vi-VN', { style: 'currency', currency }).format(Number(amount))
  } catch {
    return `${amount} ${currency}` // mã tiền tệ lạ: Intl ném RangeError, không để vỡ cả trang
  }
}

export const dateTime = (iso: string | null) => (iso ? new Date(iso).toLocaleString('vi-VN') : '—')

// "129.9" -> 12990. Chỉ dùng để ước tính tổng giỏ hàng; tổng chính thức do server tính.
export function toCents(amount: string) {
  const [int, frac = ''] = amount.split('.')
  return Number(int) * 100 + Number(frac.padEnd(2, '0').slice(0, 2))
}
export const fromCents = (cents: number) => `${Math.trunc(cents / 100)}.${String(cents % 100).padStart(2, '0')}`

// Olist không có tên sản phẩm: dùng tên danh mục + 6 ký tự cuối của ID.
export function productLabel(name: string | null, category: string | null, id: string) {
  return name ?? `${category ?? 'Sản phẩm'} · ${id.slice(-6)}`
}

export const label = (map: Record<string, string>, value: string | null) => (value ? (map[value] ?? value) : '—')

export const displayStatusLabel: Record<string, string> = {
  confirming: 'Đơn đang được xác nhận',
  confirmed: 'Đã xác nhận',
  packing: 'Đang đóng gói',
  shipped: 'Đang giao',
  delivered: 'Đã giao',
  canceled: 'Đã hủy',
  returned: 'Đã trả hàng',
}
export const fulfillmentLabel: Record<string, string> = {
  unfulfilled: 'Chưa xử lý',
  packing: 'Đang đóng gói',
  shipped: 'Đã gửi hàng',
  delivered: 'Đã giao',
  canceled: 'Đã hủy',
  returned: 'Đã trả hàng',
}
export const reviewLabel: Record<string, string> = {
  pending: 'Chờ đánh giá',
  hold: 'Hold',
  released: 'Released',
}
export const riskLabel: Record<string, string> = {
  low: 'Thấp',
  medium: 'Trung bình',
  high: 'Cao',
  unassessed: 'Chưa đánh giá',
}
export const paymentMethodLabel: Record<string, string> = {
  cod: 'Thanh toán khi nhận hàng (COD)',
  credit_card: 'Thẻ tín dụng',
  debit_card: 'Thẻ ghi nợ',
  boleto: 'Boleto',
  voucher: 'Voucher',
  mixed: 'Kết hợp',
  not_defined: 'Không xác định',
  simulated: 'Mô phỏng',
}
export const paymentStatusLabel: Record<string, string> = {
  unknown: 'Không rõ',
  pending: 'Chờ thanh toán',
  succeeded: 'Đã thanh toán',
  failed: 'Thất bại',
  refunded: 'Đã hoàn tiền',
}
export const eventLabel: Record<string, string> = {
  created: 'Đặt hàng',
  confirmed: 'Xác nhận đơn',
  address_changed: 'Đổi địa chỉ',
  canceled: 'Hủy đơn',
  delivery_failed: 'Giao hàng thất bại',
  delivery_succeeded: 'Giao hàng thành công',
  return_requested: 'Yêu cầu trả hàng',
}
export const actorLabel: Record<string, string> = {
  buyer: 'Người mua',
  seller: 'Người bán',
  carrier: 'Vận chuyển',
  platform: 'Nền tảng',
  policy: 'Chính sách',
  unknown: 'Không rõ',
}
export const decisionLabel: Record<string, string> = { hold: 'Hold', release: 'Release', keep_hold: 'Keep Hold' }
