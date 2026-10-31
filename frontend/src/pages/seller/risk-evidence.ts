export function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}
export function display(value: unknown): string {
  if (value === null || value === undefined) return 'Chưa biết'
  if (typeof value === 'number' || (typeof value === 'string' && value.trim() && Number.isFinite(Number(value))))
    return Number(value).toLocaleString('vi-VN', { maximumFractionDigits: 4 })
  return typeof value === 'string' ? value : JSON.stringify(value)
}
export function evidenceSummary(id: string, facts: Record<string, unknown>, params: Record<string, unknown>): string | null {
  const window = params.window_days != null ? ` trong ${display(params.window_days)} ngày` : ''
  if ((id === 'R1' || id === 'R2') && facts.failed != null && facts.resolved != null)
    return `${display(facts.failed)} / ${display(facts.resolved)} lần mua có kết quả giao${window} đã thất bại và được xác minh do người mua, theo ${id === 'R1' ? 'số điện thoại' : 'cụm địa chỉ'}.`
  if ((id === 'R3' || id === 'R10') && facts.count != null)
    return `${display(facts.count)} lần ${id === 'R3' ? 'giao thất bại đã xác minh do người mua' : 'người mua hủy sau khi xác nhận'}${window}.`
  return null
}
