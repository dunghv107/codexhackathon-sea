import { Dialog, Label, Select } from 'radix-ui'
import { useId, useState, type ComponentProps, type FormEvent, type ReactNode } from 'react'
import { useSearchParams } from 'react-router'
import { errMsg } from './api'
import { sampleProductImage } from './product-images'
import { label, money, productLabel } from './format'
import type { OrderAddress, OrderItem } from './types'

export const inputCls =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-2 focus:outline-blue-500'

const variants = {
  primary: 'bg-[#ed860b] text-[#25180a] hover:bg-[#dc7900]',
  secondary: 'border border-gray-300 bg-white hover:bg-gray-100',
  danger: 'border border-red-300 bg-white text-red-700 hover:bg-red-50',
}

export function Button({
  variant = 'secondary',
  className = '',
  ...props
}: ComponentProps<'button'> & { variant?: keyof typeof variants }) {
  return (
    <button
      {...props}
      className={`ui-button rounded-md px-4 py-2.5 text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 ${variants[variant]} ${className}`}
    />
  )
}

export function Field({ label: text, ...props }: ComponentProps<'input'> & { label: string }) {
  const id = useId()
  return (
    <div className="space-y-1">
      <Label.Root htmlFor={id} className="text-sm font-medium">
        {text}
      </Label.Root>
      <input id={id} className={inputCls} {...props} />
    </div>
  )
}

export function TextArea({ label: text, ...props }: ComponentProps<'textarea'> & { label: string }) {
  const id = useId()
  return (
    <div className="space-y-1">
      <Label.Root htmlFor={id} className="text-sm font-medium">
        {text}
      </Label.Root>
      <textarea id={id} rows={3} className={inputCls} {...props} />
    </div>
  )
}

// Radix Select không cho value rỗng, nên các bộ lọc dùng 'all' cho "Tất cả".
export function SelectField({
  label: text,
  options,
  placeholder,
  ...props
}: Omit<ComponentProps<typeof Select.Root>, 'children'> & {
  label: string
  options: [value: string, text: string][]
  placeholder?: string
}) {
  const id = useId()
  return (
    <div className="space-y-1">
      <Label.Root htmlFor={id} className="text-sm font-medium">
        {text}
      </Label.Root>
      <Select.Root {...props}>
        <Select.Trigger id={id} className={`${inputCls} flex items-center justify-between gap-2 text-left`}>
          <Select.Value placeholder={placeholder ?? 'Chọn…'} />
          <Select.Icon aria-hidden>▾</Select.Icon>
        </Select.Trigger>
        <Select.Portal>
          <Select.Content
            position="popper"
            sideOffset={4}
            className="z-[70] max-h-72 min-w-(--radix-select-trigger-width) overflow-hidden rounded-md border border-gray-200 bg-white shadow-md"
          >
            <Select.Viewport className="p-1">
              {options.map(([value, optionText]) => (
                <Select.Item
                  key={value}
                  value={value}
                  className="cursor-pointer rounded px-2 py-1.5 text-sm outline-none data-highlighted:bg-gray-100 data-[state=checked]:font-semibold"
                >
                  <Select.ItemText>{optionText}</Select.ItemText>
                </Select.Item>
              ))}
            </Select.Viewport>
          </Select.Content>
        </Select.Portal>
      </Select.Root>
    </div>
  )
}

export function ErrorText({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="text-sm text-red-600">
      {children}
    </p>
  )
}

// Hiển thị lỗi/đang tải cho kết quả của useLoad, có dữ liệu thì gọi children.
export function Loaded<T>({
  state,
  children,
}: {
  state: { data?: T; error?: string; reload?: () => void }
  children: (data: T) => ReactNode
}) {
  if (state.error && state.data === undefined)
    return (
      <div className="card space-y-3">
        <ErrorText>{state.error}</ErrorText>
        {state.reload && <Button onClick={state.reload}>Thử lại</Button>}
      </div>
    )
  if (state.data === undefined)
    return (
      <div role="status" className="space-y-4">
        <p className="subtle">Đang tải…</p>
        <div className="skeleton" />
        <div className="skeleton" />
      </div>
    )
  return (
    <>
      {state.error && (
        <div role="alert" className="card space-y-2">
          <p className="text-red-700">{state.error} · Dữ liệu đang hiển thị có thể đã cũ.</p>
          {state.reload && <Button onClick={state.reload}>Thử lại</Button>}
        </div>
      )}
      {children(state.data)}
    </>
  )
}

export function Pager({ page, totalPages }: { page: number; totalPages: number }) {
  const [, setParams] = useSearchParams()
  if (totalPages <= 1) return null
  const go = (p: number) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('page', String(p))
      return next
    })
  return (
    <nav aria-label="Phân trang" className="flex items-center gap-3 text-sm">
      <Button disabled={page <= 1} onClick={() => go(page - 1)}>
        Trước
      </Button>
      <span>
        Trang {page} / {totalPages}
      </span>
      <Button disabled={page >= totalPages} onClick={() => go(page + 1)}>
        Sau
      </Button>
    </nav>
  )
}

// Dialog chứa form; lỗi từ onSubmit hiện trong dialog, thành công thì đóng.
export function FormDialog({
  title,
  trigger,
  submitLabel = 'Lưu',
  submitDisabled = false,
  onSubmit,
  children,
}: {
  title: string
  trigger: ReactNode
  submitLabel?: string
  submitDisabled?: boolean
  onSubmit: (form: FormData) => Promise<unknown>
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (busy || submitDisabled) return
    setBusy(true)
    setError('')
    try {
      await onSubmit(new FormData(e.currentTarget))
      setOpen(false)
    } catch (err) {
      setError(errMsg(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(o) => {
        if (busy) return
        setOpen(o)
        setError('')
      }}
    >
      <Dialog.Trigger asChild>{trigger}</Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content aria-describedby={undefined} className="dialog-content space-y-4">
          <Dialog.Title className="text-lg font-semibold">{title}</Dialog.Title>
          <form onSubmit={submit} className="space-y-3">
            {children}
            {error && <ErrorText>{error}</ErrorText>}
            <div className="flex justify-end gap-2 pt-2">
              <Dialog.Close asChild>
                <Button type="button" disabled={busy}>
                  Hủy
                </Button>
              </Dialog.Close>
              <Button variant="primary" disabled={busy || submitDisabled}>
                {busy ? 'Đang gửi…' : submitLabel}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="card space-y-4">
      <h2 className="font-medium">{title}</h2>
      {children}
    </section>
  )
}

export function OrderItems({ items, currency }: { items: OrderItem[]; currency: string }) {
  return (
    <table className="w-full text-sm">
      <thead className="text-left text-gray-500">
        <tr>
          <th className="py-1 font-medium">Sản phẩm</th>
          <th className="py-1 text-right font-medium">SL</th>
          <th className="py-1 text-right font-medium">Đơn giá</th>
          <th className="py-1 text-right font-medium">Phí ship</th>
        </tr>
      </thead>
      <tbody>
        {items.map((it) => (
          <tr key={it.item_id} className="border-t border-gray-100">
            <td className="py-1">{productLabel(it.product_name, it.category_name_en)}</td>
            <td className="py-1 text-right">{it.quantity}</td>
            <td className="py-1 text-right">{money(it.unit_price, currency)}</td>
            <td className="py-1 text-right">{money(it.freight_amount, currency)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function AddressBlock({ address }: { address: OrderAddress }) {
  return (
    <address className="text-sm not-italic">
      {address.address_raw ?? 'Chưa biết địa chỉ'}
      <br />
      {[address.city, address.region, address.postal_prefix, address.country_code].filter(Boolean).join(', ')}
    </address>
  )
}

export function Badge({ map, value }: { map: Record<string, string>; value: string | null }) {
  return (
    <span className="badge" data-value={value ?? 'unassessed'}>
      {value === 'high' ? '▲ ' : value === 'medium' ? '◆ ' : value === 'low' ? '● ' : ''}
      {label(map, value)}
    </span>
  )
}

export function ProductPlaceholder({
  productId,
  src,
  alt = 'Ảnh sản phẩm',
}: Readonly<{ productId: string; src?: string | null; alt?: string }>) {
  const [failed, setFailed] = useState<string[]>([])
  const sample = sampleProductImage(productId)
  const image = src?.startsWith('https://') && !failed.includes(src) ? src : sample
  if (!failed.includes(image))
    return (
      <div className="product-placeholder has-image">
        <img
          src={image}
          alt={image === sample ? 'Ảnh minh họa sản phẩm' : alt}
          title={image === sample ? 'Ảnh minh họa' : undefined}
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailed((previous) => [...previous, image])}
        />
      </div>
    )
  return (
    <div className="product-placeholder" aria-label="Chưa có ảnh sản phẩm">
      <svg viewBox="0 0 48 48" aria-hidden="true">
        <path d="m24 5 17 9v20l-17 9-17-9V14zM7 14l17 10 17-10M24 24v19M15 9l18 10v10" />
      </svg>
    </div>
  )
}
