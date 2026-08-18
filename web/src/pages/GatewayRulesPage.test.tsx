// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { GatewayRulesDocument, Overview } from '../types'

vi.mock('../api', () => ({
  api: {
    gatewayRules: vi.fn(),
    saveGatewayRules: vi.fn(),
    gateway: vi.fn(),
  },
  waitForOperation: vi.fn(),
}))

import { api, waitForOperation } from '../api'
import { GatewayRulesPage } from './GatewayRulesPage'

const document: GatewayRulesDocument = {
  schema_version: 1,
  revision: 'rules-r1',
  rules: {
    schema_version: 1,
    prepend: Array.from({ length: 30 }, (_, index) => `DOMAIN-SUFFIX,example-${index + 1}.com,Proxy`),
    append: ['DOMAIN,append.example,Proxy'],
    delete: [],
  },
}

const overview = {
  status: { gateway: 'running', mihomo: 'running' },
  policies: [{ name: 'Proxy', type: 'Selector', selected: 'Proxy-A', options: ['Proxy-A'] }],
  providers: { rule_providers: [{ name: 'Apple' }] },
} as unknown as Overview

describe('GatewayRulesPage', () => {
  beforeEach(() => {
    vi.mocked(api.gatewayRules).mockResolvedValue(document)
    vi.mocked(api.saveGatewayRules).mockResolvedValue(document)
    vi.mocked(api.gateway).mockResolvedValue({ id: 'reload-1', kind: 'reload', state: 'running' })
    vi.mocked(waitForOperation).mockResolvedValue({ id: 'reload-1', kind: 'reload', state: 'succeeded' })
  })

  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('keeps every rule category collapsed by default and bounds long lists inside the expanded panel', async () => {
    render(<GatewayRulesPage overview={overview} onChanged={vi.fn()} onNotify={vi.fn()} onDirtyChange={vi.fn()} />)

    expect(await screen.findByRole('heading', { name: '前置规则' })).toBeTruthy()
    expect(screen.getByText('30', { selector: '.gateway-rule-editor-summary strong' })).toBeTruthy()
    expect(screen.queryByLabelText('前置规则新规则')).toBeNull()
    expect(screen.getByRole('button', { name: '展开追加规则' }).getAttribute('aria-expanded')).toBe('false')
    expect(screen.getByRole('button', { name: '展开从订阅中删除' }).getAttribute('aria-expanded')).toBe('false')

    await userEvent.click(screen.getByRole('button', { name: '展开前置规则' }))
    const list = screen.getByRole('list', { name: '前置规则列表' })
    expect(within(list).getAllByRole('listitem')).toHaveLength(30)
    expect(list.classList.contains('gateway-rule-list')).toBe(true)

    await userEvent.click(screen.getByRole('button', { name: '收起前置规则' }))
    expect(screen.queryByRole('list', { name: '前置规则列表' })).toBeNull()
  })

  it('collapses the policy reference and presents reload as the standard primary action', async () => {
    const onChanged = vi.fn()
    const onNotify = vi.fn()
    render(<GatewayRulesPage overview={overview} onChanged={onChanged} onNotify={onNotify} onDirtyChange={vi.fn()} />)

    const reference = await screen.findByText('查看当前可用目标')
    expect(reference.closest('details')?.hasAttribute('open')).toBe(false)

    const reload = screen.getByRole('button', { name: '重载网关使规则生效' })
    expect(reload.classList.contains('primary')).toBe(true)
    await userEvent.click(reload)

    await waitFor(() => expect(api.gateway).toHaveBeenCalledWith('reload'))
    expect(waitForOperation).toHaveBeenCalledWith('reload-1')
    expect(onChanged).toHaveBeenCalledOnce()
    expect(onNotify).toHaveBeenCalledWith(expect.objectContaining({ tone: 'success', title: '重载网关成功' }))
  })

  it('treats text in an add-rule input as unsaved and includes it when saving', async () => {
    const onDirtyChange = vi.fn()
    vi.mocked(api.saveGatewayRules).mockImplementation(async rules => ({ ...document, revision: 'rules-r2', rules }))
    render(<GatewayRulesPage overview={overview} onChanged={vi.fn()} onNotify={vi.fn()} onDirtyChange={onDirtyChange} />)

    await screen.findByRole('heading', { name: '前置规则' })
    await userEvent.click(screen.getByRole('button', { name: '展开前置规则' }))
    await userEvent.type(screen.getByLabelText('前置规则新规则'), '  DOMAIN-KEYWORD,openai,Proxy  ')

    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true))
    await userEvent.click(screen.getByRole('button', { name: '保存规则' }))
    await waitFor(() => expect(api.saveGatewayRules).toHaveBeenCalledWith(
      expect.objectContaining({ prepend: [...document.rules.prepend, 'DOMAIN-KEYWORD,openai,Proxy'] }),
      'rules-r1',
    ))
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(false))
  })

  it('warns before closing or reloading while rule edits are unsaved', async () => {
    render(<GatewayRulesPage overview={overview} onChanged={vi.fn()} onNotify={vi.fn()} onDirtyChange={vi.fn()} />)

    await screen.findByRole('heading', { name: '前置规则' })
    const cleanEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(cleanEvent)
    expect(cleanEvent.defaultPrevented).toBe(false)

    await userEvent.click(screen.getByRole('button', { name: '展开前置规则' }))
    await userEvent.type(screen.getByLabelText('前置规则新规则'), 'DOMAIN,unsaved.example,DIRECT')
    await waitFor(() => {
      const dirtyEvent = new Event('beforeunload', { cancelable: true })
      window.dispatchEvent(dirtyEvent)
      expect(dirtyEvent.defaultPrevented).toBe(true)
    })
  })
})
