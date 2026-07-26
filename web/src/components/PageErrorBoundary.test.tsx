// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PageErrorBoundary } from './PageErrorBoundary'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('PageErrorBoundary', () => {
  it('keeps navigation recovery guidance visible when a page render fails', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(<PageErrorBoundary><BrokenPage /></PageErrorBoundary>)

    expect(screen.getByRole('heading', { name: '当前页面暂时无法显示' })).toBeTruthy()
    expect(screen.getByText(/仍可使用左侧导航打开其他页面/)).toBeTruthy()
    expect(screen.getByRole('button', { name: '重试当前页面' })).toBeTruthy()
  })
})

function BrokenPage(): never {
  throw new Error('render failed')
}
