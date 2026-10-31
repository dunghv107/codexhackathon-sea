import { test, expect, vi } from 'vitest'
import { act, screen } from '@testing-library/react'
import { mockApi, page } from './helpers'

const capture = vi.hoisted(() => ({ root: null }))
vi.mock('react-dom/client', async (original) => {
  const client = await original()
  return { ...client, createRoot: (...args) => {
    capture.root = client.createRoot(...args)
    return capture.root
  } }
})

test('application entry mounts the real route tree and loads the public catalog', async () => {
  const element = document.createElement('div')
  element.id = 'root'
  document.body.append(element)
  mockApi((path) => path.includes('categories') ? [] : page())
  try {
    await act(async () => { await import('../src/main') })
    expect(await screen.findByText('Chưa tìm thấy sản phẩm phù hợp')).toBeTruthy()
    expect(screen.getByRole('navigation', { name: 'Cửa hàng' })).toBeTruthy()
  } finally {
    await act(async () => capture.root?.unmount())
    element.remove()
  }
})
