import { afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'

// jsdom has no layout/pointer capture; these browser APIs are used by Radix.
window.HTMLElement.prototype.scrollIntoView = () => {}
window.HTMLElement.prototype.hasPointerCapture = () => false
window.HTMLElement.prototype.setPointerCapture = () => {}
window.HTMLElement.prototype.releasePointerCapture = () => {}
window.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} }
window.PointerEvent = window.MouseEvent

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  localStorage.clear()
})
