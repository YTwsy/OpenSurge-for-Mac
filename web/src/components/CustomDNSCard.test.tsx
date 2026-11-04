// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useState } from 'react'
import { activateLanguage, prepareLanguage } from '../i18n'
import type { CustomDNSDraft, CustomDNSPreview } from '../types'

vi.mock('../api', () => ({ api: { customDNS: vi.fn(), saveCustomDNS: vi.fn(), importCustomDNS: vi.fn(), previewCustomDNS: vi.fn() } }))
vi.mock('../desktop', () => ({ isDesktop: vi.fn(() => false), desktopAction: vi.fn() }))
import { api } from '../api'
import { desktopAction, isDesktop } from '../desktop'
import { CustomDNSCard } from './CustomDNSCard'

const onNotify = vi.fn()

const initial: CustomDNSDraft = { schema_version: 1, revision: 'dns-1', document: { schema_version: 1, enabled: false, yaml: 'hosts: {}\ndns: {}\n' }, desired: true, applied: false, hosts: 0, policies: 0 }
const preview: CustomDNSPreview = { changes: [{ field: 'hosts.nas.home.arpa', before: '', after: '192.0.2.8', source: 'custom-dns' }], managed_changes: [], warnings: [], blockers: ['DHCP namespace conflict'], final_yaml: 'hosts: {nas.home.arpa: 192.0.2.8}', base_yaml: 'dns: {}', overlay_yaml: 'dns: {}', custom_yaml: '', mac_ready: true, downstream_ready: true, dns_references: [] }

function Card() {
  const [saved, setSaved] = useState(initial)
  return <CustomDNSCard onNotify={onNotify} saved={saved} onSaved={setSaved} running onOpenNetwork={vi.fn()} />
}

async function open() {
  render(<Card />)
  await screen.findByText('0 个映射 · 0 个上游策略')
  await userEvent.click(screen.getByText('自定义 DNS'))
}

describe('CustomDNSCard', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.mocked(api.customDNS).mockResolvedValue(structuredClone(initial)); vi.mocked(isDesktop).mockReturnValue(false) })
  afterEach(() => { cleanup(); activateLanguage('zh-Hans') })

  it('saves a draft while running without applying and retains input on conflicts', async () => {
    await open()
    await userEvent.click(screen.getByLabelText('启用自定义 DNS'))
    const editor = screen.getByRole('textbox', { name: '自定义 DNS YAML' })
    fireEvent.change(editor, { target: { value: 'hosts: {nas.home.arpa: 192.0.2.8}' } })
    vi.mocked(api.saveCustomDNS).mockRejectedValueOnce(new Error('revision conflict'))
    await userEvent.click(screen.getByRole('button', { name: '保存草稿' }))
    await waitFor(() => expect(onNotify).toHaveBeenCalledWith({ tone: 'error', title: '操作未完成', message: 'revision conflict' }))
    expect(onNotify).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('alert')).toBeNull()
    expect((editor as HTMLTextAreaElement).value).toContain('nas.home.arpa')
    const doc = { schema_version: 1, enabled: true, yaml: 'hosts: {nas.home.arpa: 192.0.2.8}' }
    vi.mocked(api.saveCustomDNS).mockResolvedValue({ ...initial, revision: 'dns-2', document: doc, desired: false })
    await userEvent.click(screen.getByRole('button', { name: '保存草稿' }))
    await waitFor(() => expect(onNotify).toHaveBeenLastCalledWith({ tone: 'success', title: '草稿已保存', message: '草稿已保存，运行中的 DNS 配置未改变。' }))
    expect(onNotify).toHaveBeenCalledTimes(2)
    expect(screen.queryByText('草稿已保存，运行中的 DNS 配置未改变。')).toBeNull()
    expect(api.saveCustomDNS).toHaveBeenLastCalledWith(doc, 'dns-1')
    expect(api.previewCustomDNS).not.toHaveBeenCalled()
  })

  it('shows replacement before accepting an imported file and does not save it', async () => {
    await open()
    const imported = { ...initial.document, yaml: 'hosts: {nas.home.arpa: 192.0.2.8}\ndns: {}\n' }
    vi.mocked(api.importCustomDNS).mockResolvedValue({ document: imported, sections: ['hosts'] })
    const file = new File(['192.0.2.8 nas.home.arpa'], 'my-hosts')
    Object.defineProperty(file, 'arrayBuffer', { value: async () => new TextEncoder().encode('192.0.2.8 nas.home.arpa').buffer })
    fireEvent.change(document.querySelector('input[type=file]')!, { target: { files: [file] } })
    await screen.findByRole('region', { name: '导入替换预览' })
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(initial.document.yaml)
    await userEvent.click(screen.getByRole('button', { name: '采用导入内容' }))
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(imported.yaml)
    expect(api.saveCustomDNS).not.toHaveBeenCalled()
  })

  it('preserves the editor when a file is invalid', async () => {
    await open()
    const file = new File([new Uint8Array([0xff])], 'invalid-hosts')
    Object.defineProperty(file, 'arrayBuffer', { value: async () => new Uint8Array([0xff]).buffer })
    fireEvent.change(document.querySelector('input[type=file]')!, { target: { files: [file] } })
    await waitFor(() => expect(onNotify).toHaveBeenCalledWith({ tone: 'error', title: '操作未完成', message: '文件必须使用 UTF-8 编码。' }))
    expect(api.importCustomDNS).not.toHaveBeenCalled()
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(initial.document.yaml)
  })

  it('displays prerequisites and blockers without claiming a live query passed', async () => {
    await open(); vi.mocked(api.previewCustomDNS).mockResolvedValue(preview)
    await userEvent.click(screen.getByRole('button', { name: '预览' }))
    await userEvent.click(await screen.findByText('配置检查'))
    await screen.findByText('DHCP namespace conflict')
    expect(screen.getByText('预览以已保存的网络设置为准；配置检查不代表真实查询验收。')).toBeTruthy()
    expect(screen.queryByText('配置检查通过；应用时仍需通过引擎校验。')).toBeNull()
    expect(api.saveCustomDNS).not.toHaveBeenCalled()
  })

  it('exports through the native save panel in the desktop host', async () => {
    await open(); vi.mocked(isDesktop).mockReturnValue(true)
    vi.mocked(api.importCustomDNS).mockResolvedValue({ document: initial.document, sections: ['hosts', 'dns'] })
    vi.mocked(desktopAction).mockResolvedValue({ saved: true })
    await userEvent.click(screen.getByRole('button', { name: '导出草稿' }))
    await waitFor(() => expect(desktopAction).toHaveBeenCalledWith('save-dns-file', { filename: 'custom-dns.yaml', text: initial.document.yaml }))
  })

  it('renders the editor and preview in English', async () => {
    await prepareLanguage('en'); activateLanguage('en')
    render(<Card />)
    await screen.findByText('0 host mappings · 0 upstream policies')
    await userEvent.click(screen.getByText('Custom DNS'))
    vi.mocked(api.previewCustomDNS).mockResolvedValue(preview)
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }))
    await screen.findByRole('region', { name: 'Final DNS configuration' })
    expect(document.body.textContent).not.toMatch(/[\u3400-\u9fff]/)
  })

  it('keeps a failed initial load visible in the editor', async () => {
    render(<CustomDNSCard onNotify={onNotify} saved={null} onSaved={vi.fn()} running={false} onOpenNetwork={vi.fn()} loadError="DNS draft unavailable" />)
    await userEvent.click(screen.getByText('自定义 DNS'))
    expect(screen.getByRole('alert').textContent).toBe('DNS draft unavailable')
    expect(screen.getByRole('button', { name: '重新载入草稿' })).toBeTruthy()
    expect(onNotify).not.toHaveBeenCalled()
  })

  it('keeps unsaved YAML when the parent refreshes application status', async () => {
    const onDirty = vi.fn()
    const props = { onNotify, saved: initial, onSaved: vi.fn(), running: false, onOpenNetwork: vi.fn(), onDirtyChange: onDirty }
    const view = render(<CustomDNSCard {...props} />)
    await userEvent.click(screen.getByText('自定义 DNS'))
    const editor = screen.getByRole('textbox', { name: '自定义 DNS YAML' })
    fireEvent.change(editor, { target: { value: 'hosts: {nas.home.arpa: 192.0.2.30}' } })
    view.rerender(<CustomDNSCard {...props} saved={{ ...initial, applied: true }} running />)
    expect((editor as HTMLTextAreaElement).value).toContain('192.0.2.30')
    expect(onDirty).toHaveBeenLastCalledWith(true)
  })
})
