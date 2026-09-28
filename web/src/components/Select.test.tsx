// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { Select } from './Select'

afterEach(cleanup)

it('keeps the committed selection on Escape and supports keyboard selection around disabled options', async () => {
  const change = vi.fn()
  render(<Select aria-label="Protocol" value="all" onChange={change}><option value="all">All</option><option disabled value="tcp">TCP</option><option value="udp">UDP</option></Select>)
  const trigger = screen.getByRole('combobox')
  await userEvent.click(trigger)
  await userEvent.keyboard('{ArrowDown}{Escape}')
  expect(change).not.toHaveBeenCalled()
  expect(screen.queryByRole('listbox')).toBeNull()
  expect(document.activeElement).toBe(trigger)
  await userEvent.click(trigger)
  await userEvent.keyboard('{ArrowDown}{Enter}')
  expect(change).toHaveBeenCalledWith('udp')
  expect(screen.queryByRole('listbox')).toBeNull()
})

it('supports typeahead and closes on outside click without changing a selection', async () => {
  const change = vi.fn()
  render(<Select aria-label="Language" value="zh" onChange={change}><option value="zh">简体中文</option><option value="en">English</option></Select>)
  await userEvent.click(screen.getByRole('combobox'))
  await userEvent.keyboard('eng{Enter}')
  expect(change).toHaveBeenCalledWith('en')
  await userEvent.click(screen.getByRole('combobox'))
  fireEvent.pointerDown(document.body)
  expect(screen.queryByRole('listbox')).toBeNull()
  expect(change).toHaveBeenCalledTimes(1)
})

it('does not open a disabled control or submit its parent form', async () => {
  const submit = vi.fn(event => event.preventDefault())
  const view = render(<form onSubmit={submit}><Select disabled aria-label="Protocol" value="tcp" onChange={vi.fn()}><option value="tcp">TCP</option></Select></form>)
  await userEvent.click(screen.getByRole('combobox'))
  expect(screen.queryByRole('listbox')).toBeNull()
  view.rerender(<form onSubmit={submit}><Select aria-label="Protocol" value="tcp" onChange={vi.fn()}><option value="tcp">TCP</option></Select></form>)
  await userEvent.click(screen.getByRole('combobox'))
  await userEvent.keyboard('{Enter}')
  expect(submit).not.toHaveBeenCalled()
})
