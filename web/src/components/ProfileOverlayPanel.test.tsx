// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProfileOverlay, ProfileOverlayDocument, ProfileOverlayPreview, Source } from '../types'
import { activateLanguage, prepareLanguage } from '../i18n'

vi.mock('../api', () => ({
  api: {
    saveProfileOverlayDocument: vi.fn(),
    saveProfileOverlayYAML: vi.fn(),
    sourcePreview: vi.fn(),
  },
}))

import { api } from '../api'
import { ProfileOverlayPanel } from './ProfileOverlayPanel'

const document: ProfileOverlayDocument = {
  schema_version: 1,
  enabled: false,
  rules: { prepend: [], append_before_match: [] },
  proxies: { add: [], replace: [] },
  proxy_providers: { add: {}, replace: {} },
  proxy_groups: { add: [], replace: [], patch: [] },
  rule_providers: { add: {}, replace: {} },
  dns: { merge: {}, append: {} },
}

const overlay: ProfileOverlay = {
  schema_version: 1,
  revision: 'overlay-revision',
  yaml: 'schema-version: 1\nenabled: false\n',
  document,
  desired: true,
  applied: false,
  validation: '附加配置结构有效',
}

const source: Source = {
  id: 'home',
  name: 'Home',
  kind: 'mihomo_profile',
  origin: 'file:home.yaml',
  digest: 'source-digest',
  size: 100,
  valid: true,
  validation: 'valid',
  desired: false,
  applied: false,
  versions: [],
  diff: { proxies_added: [], proxies_removed: [], groups_added: [], groups_removed: [], proxy_providers_added: [], proxy_providers_removed: [], rule_providers_added: [], rule_providers_removed: [], rule_count_delta: 0 },
  imported_at: '2026-08-24T00:00:00Z',
  inventory: { proxies: ['edge'], proxy_providers: [], proxy_groups: ['Main'], rule_providers: [], rule_count: 2, terminal_match: true, warnings: [] },
  effective_inventory: { proxies: ['edge'], proxy_providers: [], proxy_groups: ['Main'], rule_providers: [], rule_count: 2, terminal_match: true, warnings: [] },
  overlay_compatible: true,
  overlay_validation: 'compatible',
}

const preview: ProfileOverlayPreview = {
  schema_version: 1,
  source_id: 'home',
  source_yaml: 'rules:\n  - MATCH,DIRECT\n',
  overlay_yaml: 'schema-version: 1\nenabled: true\n',
  effective_profile_yaml: 'rules:\n  - DOMAIN,first.example,DIRECT\n  - MATCH,DIRECT\n',
  final_mihomo_yaml: 'mixed-port: 7890\nrules:\n  - DOMAIN,first.example,DIRECT\n  - MATCH,DIRECT\n',
  original_inventory: source.inventory,
  effective_inventory: { ...source.inventory, rule_count: 3 },
  diff: { ...source.diff, rule_count_delta: 1 },
  validation: '结构组合通过',
}

describe('ProfileOverlayPanel', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => { cleanup(); activateLanguage('zh-Hans') })

  it('renders the guided editor and preview in English', async () => {
    await prepareLanguage('en')
    activateLanguage('en')
    vi.mocked(api.sourcePreview).mockResolvedValue(preview)
    render(<ProfileOverlayPanel overlay={overlay} sources={[source]} onSaved={vi.fn()} />)

    expect(screen.getByRole('heading', { name: 'Advanced: Global Profile Overlay' })).toBeTruthy()
    const panel = documentQuery('details.profile-overlay-panel')
    expect(panel.open).toBe(false)
    await userEvent.click(screen.getByRole('heading', { name: 'Advanced: Global Profile Overlay' }))
    expect(panel.open).toBe(true)
    await userEvent.selectOptions(screen.getByLabelText('Select a source to preview'), 'home')
    await userEvent.click(screen.getByRole('button', { name: 'View composed result' }))
    const dialog = await screen.findByRole('dialog', { name: 'Final configuration preview' })
    expect(within(dialog).getByText('Composition order')).toBeTruthy()
    expect(globalThis.document.body.textContent).not.toMatch(/[\u3400-\u9fff]/)
  })

  it('guides a user from enabling and adding a rule to a saved draft', async () => {
    const onSaved = vi.fn()
    vi.mocked(api.saveProfileOverlayDocument).mockImplementation(async candidate => ({
      ...overlay,
      revision: 'saved-revision',
      document: candidate,
      yaml: 'schema-version: 1\nenabled: true\n',
      desired: false,
    }))

    render(<ProfileOverlayPanel overlay={overlay} sources={[source]} onSaved={onSaved} />)

    expect(screen.getByRole('heading', { name: '高级：全局附加配置' })).toBeTruthy()
    expect(screen.getByText('1/1')).toBeTruthy()
    await userEvent.click(screen.getByRole('heading', { name: '高级：全局附加配置' }))
    await userEvent.click(screen.getByRole('switch', { name: '已停用' }))
    await userEvent.type(screen.getByLabelText('高优先级自定义规则'), 'DOMAIN,first.example,DIRECT')
    await userEvent.click(screen.getByRole('button', { name: '保存附加配置草稿' }))

    await waitFor(() => expect(api.saveProfileOverlayDocument).toHaveBeenCalled())
    const saved = vi.mocked(api.saveProfileOverlayDocument).mock.calls[0][0]
    expect(saved.enabled).toBe(true)
    expect(saved.rules.prepend).toEqual(['DOMAIN,first.example,DIRECT'])
    expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ revision: 'saved-revision' }))
    expect(await screen.findByText(/运行网关才会改变/)).toBeTruthy()
  })

  it('previews every composition layer and the final mihomo config', async () => {
    vi.mocked(api.sourcePreview).mockResolvedValue(preview)
    render(<ProfileOverlayPanel overlay={overlay} sources={[source]} onSaved={vi.fn()} />)

    await userEvent.click(screen.getByRole('heading', { name: '高级：全局附加配置' }))
    await userEvent.selectOptions(screen.getByLabelText('选择要预览的来源'), 'home')
    await userEvent.click(screen.getByRole('button', { name: '查看组合结果' }))

    const dialog = await screen.findByRole('dialog', { name: '最终配置预览' })
    expect(within(dialog).getByText(/高优先级自定义规则.*订阅规则.*专家低优先级规则.*最终 MATCH/)).toBeTruthy()
    await userEvent.click(within(dialog).getByRole('button', { name: '原始来源' }))
    expect(within(dialog).getByText(/rules:\s+- MATCH,DIRECT/)).toBeTruthy()
    await userEvent.click(within(dialog).getByRole('button', { name: '最终 mihomo' }))
    expect(within(dialog).getByText(/mixed-port: 7890/)).toBeTruthy()
    await userEvent.click(within(dialog).getByRole('button', { name: '完成' }))
    expect(screen.queryByRole('dialog', { name: '最终配置预览' })).toBeNull()
  })

  it('adds proxies from share links without exposing another YAML import', async () => {
    vi.mocked(api.saveProfileOverlayDocument).mockImplementation(async candidate => ({ ...overlay, document: candidate }))
    const onSaved = vi.fn()
    render(<ProfileOverlayPanel overlay={overlay} sources={[source]} onSaved={onSaved} />)

    await userEvent.click(screen.getByRole('heading', { name: '高级：全局附加配置' }))
    expect(screen.queryByText('代理 Provider')).toBeNull()
    expect(screen.queryByText('策略组扩展')).toBeNull()
    expect(screen.queryByLabelText(/YAML.*文件|文件.*YAML/)).toBeNull()

    await userEvent.type(screen.getByLabelText('节点分享链接'), 'vless://user-id@proxy.example:443?security=tls&type=ws&path=%2Fedge#Personal')
    await userEvent.click(screen.getByRole('button', { name: '解析并添加' }))
    expect(screen.getByText('Personal')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: '保存附加配置草稿' }))

    await waitFor(() => expect(api.saveProfileOverlayDocument).toHaveBeenCalled())
    expect(vi.mocked(api.saveProfileOverlayDocument).mock.calls[0][0].proxies.add).toEqual([
      expect.objectContaining({ name: 'Personal', type: 'vless', server: 'proxy.example', port: 443, uuid: 'user-id', tls: true, network: 'ws' }),
    ])
  })

  it('keeps existing provider and tail-rule operations in expert YAML', async () => {
    const expertDocument: ProfileOverlayDocument = {
      ...structuredClone(document),
      rules: { prepend: [], append_before_match: ['DOMAIN,late.example,DIRECT'] },
      proxy_providers: { add: { Airport: { type: 'http', url: 'https://example.com/proxies.yaml' } }, replace: {} },
    }
    render(<ProfileOverlayPanel overlay={{ ...overlay, document: expertDocument }} sources={[source]} onSaved={vi.fn()} />)

    await userEvent.click(screen.getByRole('heading', { name: '高级：全局附加配置' }))
    expect(screen.getByText('2', { selector: '.overlay-summary strong' })).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: /专家 Overlay YAML/ }))
    expect(screen.getByLabelText('附加配置 YAML')).toBeTruthy()
  })
})

function documentQuery(selector: string) {
  const element = globalThis.document.querySelector(selector)
  if (!(element instanceof HTMLDetailsElement)) throw new Error(`missing ${selector}`)
  return element
}
