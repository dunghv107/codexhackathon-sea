import { AlertDialog } from 'radix-ui'
import { useState } from 'react'
import { api, enc, errMsg, useLoad } from '../../api'
import type { SavedAddress } from '../../types'
import { Button, ErrorText, Field, FormDialog, Loaded, TextArea } from '../../ui'

type AddressBody = Pick<SavedAddress, 'label' | 'address_raw' | 'region' | 'country_code' | 'is_default'>

const toBody = (form: FormData): AddressBody => ({
  label: String(form.get('label')).trim(),
  address_raw: String(form.get('address_raw')).trim(),
  region: String(form.get('region')).trim(),
  country_code: String(form.get('country_code')).trim().toUpperCase(),
  is_default: form.get('is_default') === 'on',
})

function AddressFields({ address }: { address?: SavedAddress }) {
  return (
    <>
      <Field label="Tên gợi nhớ" name="label" defaultValue={address?.label} required />
      <TextArea label="Địa chỉ" name="address_raw" defaultValue={address?.address_raw} required />
      <Field label="Tỉnh/bang" name="region" defaultValue={address?.region} required />
      <Field
        label="Mã quốc gia (2 ký tự)"
        name="country_code"
        defaultValue={address?.country_code ?? 'BR'}
        required
        minLength={2}
        maxLength={2}
        pattern="[A-Za-z]{2}"
      />
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" name="is_default" defaultChecked={address?.is_default} />
        Đặt làm địa chỉ mặc định
      </label>
    </>
  )
}

export default function Addresses() {
  const addresses = useLoad<SavedAddress[]>('/api/buyer/addresses')
  const [error, setError] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const save = (body: AddressBody, id?: string) =>
    (id
      ? api(`/api/buyer/addresses/${enc(id)}`, 'PUT', body)
      : api('/api/buyer/addresses', 'POST', body)
    ).then(addresses.reload)

  async function run(action: () => Promise<unknown>) {
    setError('')
    try {
      await action()
    } catch (e) {
      setError(errMsg(e))
    }
  }

  return (
    <>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Địa chỉ nhận hàng</h1>
        <FormDialog
          title="Thêm địa chỉ"
          trigger={<Button variant="primary">Thêm địa chỉ</Button>}
          onSubmit={(f) => save(toBody(f))}
        >
          <AddressFields />
        </FormDialog>
      </div>
      {error && <ErrorText>{error}</ErrorText>}
      <Loaded state={addresses}>
        {(list) =>
          list.length === 0 ? (
            <p className="text-sm text-gray-500">Chưa có địa chỉ nào.</p>
          ) : (
            <ul className="grid gap-4 md:grid-cols-2">
              {list.map((a) => (
                <li
                  key={a.saved_address_id}
                  className="space-y-2 rounded-lg border border-gray-200 bg-white p-4"
                >
                  <p className="font-medium">
                    {a.label}{' '}
                    {a.is_default && (
                      <span className="rounded bg-green-100 px-2 py-0.5 text-xs text-green-800">
                        Mặc định
                      </span>
                    )}
                  </p>
                  <p className="text-sm">
                    {a.address_raw}, {a.region}, {a.country_code}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <FormDialog
                      title="Sửa địa chỉ"
                      trigger={<Button>Sửa</Button>}
                      onSubmit={(f) => save(toBody(f), a.saved_address_id)}
                    >
                      <AddressFields address={a} />
                    </FormDialog>
                    {!a.is_default && (
                      <Button onClick={() => run(() => save({ ...a, is_default: true }, a.saved_address_id))}>
                        Đặt mặc định
                      </Button>
                    )}
                    <AlertDialog.Root
                      open={deleting === a.saved_address_id}
                      onOpenChange={(open) => {
                        if (!busy) {
                          setDeleting(open ? a.saved_address_id : null)
                          setError('')
                        }
                      }}
                    >
                      <AlertDialog.Trigger asChild>
                        <Button variant="danger">Xóa</Button>
                      </AlertDialog.Trigger>
                      <AlertDialog.Portal>
                        <AlertDialog.Overlay className="dialog-overlay" />
                        <AlertDialog.Content className="dialog-content space-y-4">
                          <AlertDialog.Title className="text-lg font-medium">
                            Xóa địa chỉ “{a.label}”?
                          </AlertDialog.Title>
                          <AlertDialog.Description>
                            Địa chỉ đã lưu sẽ bị xóa. Địa chỉ của các đơn đã đặt không thay đổi.
                          </AlertDialog.Description>
                          {error && <ErrorText>{error}</ErrorText>}
                          <div className="flex justify-end gap-2">
                            <AlertDialog.Cancel asChild>
                              <Button disabled={busy}>Quay lại</Button>
                            </AlertDialog.Cancel>
                            <Button
                              variant="danger"
                              disabled={busy}
                              onClick={async () => {
                                setBusy(true)
                                setError('')
                                try {
                                  await api(`/api/buyer/addresses/${enc(a.saved_address_id)}`, 'DELETE')
                                  setDeleting(null)
                                  addresses.reload()
                                } catch (e) {
                                  setError(errMsg(e))
                                } finally {
                                  setBusy(false)
                                }
                              }}
                            >
                              {busy ? 'Đang xóa…' : 'Xóa địa chỉ'}
                            </Button>
                          </div>
                        </AlertDialog.Content>
                      </AlertDialog.Portal>
                    </AlertDialog.Root>
                  </div>
                </li>
              ))}
            </ul>
          )
        }
      </Loaded>
    </>
  )
}
