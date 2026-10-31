// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { activateLanguage, prepareLanguage } from '../i18n'
import { clearOperations, getOperation, markOperationConnection, recordOperation } from '../operations'

vi.mock('../api', () => ({ watchOperations: vi.fn(() => () => {}), waitForOperation: vi.fn(async () => ({})) }))
import { waitForOperation, watchOperations } from '../api'
import { OperationProgress } from './OperationProgress'

function begin(kind = 'start') {
  const now = new Date().toISOString()
  recordOperation({ id: 'op-1', kind, state: 'running', phase: 'submitting', created_at: now, updated_at: now, phase_started_at: now })
}

describe('global operation progress', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-04T00:00:00Z'))
    clearOperations()
    activateLanguage('zh-Hans')
  })
  afterEach(() => { cleanup(); clearOperations(); vi.useRealTimers(); vi.clearAllMocks(); activateLanguage('zh-Hans') })

  it('shows initial startup immediately, real phases and elapsed time without a fake percentage', () => {
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.queryByLabelText('当前操作进度')).toBeNull()
    act(() => begin())
    expect(screen.getByText('启动网关')).toBeTruthy()
    expect(screen.getByText('正在提交操作')).toBeTruthy()
    expect(screen.getByRole('progressbar').getAttribute('aria-valuenow')).toBeNull()
    act(() => recordOperation({ id: 'op-1', kind: 'start', state: 'running', phase: 'validating_config' }))
    expect(screen.getByText('校验 Mihomo 配置')).toBeTruthy()
    act(() => vi.advanceTimersByTime(12_000))
    expect(screen.getByText('已用时 12 秒')).toBeTruthy()
    expect(screen.getByText(/仍在执行当前阶段/)).toBeTruthy()
    expect(screen.getByRole('status').textContent).not.toContain('已用时')
    expect(watchOperations).toHaveBeenCalledOnce()
  })

  it('keeps reload progress across unmounts and clearly reports rollback failure', () => {
    begin('reload')
    const first = render(<OperationProgress onOpenDiagnostics={() => {}} />)
    act(() => recordOperation({ id: 'op-1', kind: 'reload', state: 'running', phase: 'rolling_back' }))
    first.unmount()
    const diagnose = vi.fn()
    render(<OperationProgress onOpenDiagnostics={diagnose} />)
    expect(screen.getByText('操作未完成，正在回滚网络改动')).toBeTruthy()
    act(() => recordOperation({ id: 'op-1', kind: 'reload', state: 'failed', error: 'mihomo start failed' }))
    expect(screen.getByText('未完成')).toBeTruthy()
    expect(screen.getByText('mihomo start failed')).toBeTruthy()
    expect(screen.queryByRole('progressbar')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '查看诊断' }))
    expect(diagnose).toHaveBeenCalledOnce()
  })

  it('distinguishes lost contact from failure and only rechecks the original operation', () => {
    begin('reload')
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    act(() => markOperationConnection('op-1', 'unknown'))
    expect(screen.getByText('结果尚未确认')).toBeTruthy()
    expect(screen.queryByText('未完成')).toBeNull()
    expect(screen.getByText(/不会重新启动或重载网关/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '重新查询状态' }))
    expect(waitForOperation).toHaveBeenCalledWith('op-1')
    expect(getOperation('op-1')?.state).toBe('running')
  })

  it('does not declare Tailscale reachable on success and dismisses success after a short delay', () => {
    begin()
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    act(() => recordOperation({ id: 'op-1', kind: 'start', state: 'succeeded', notices: ['tailscale_warmup_started'] }))
    expect(screen.getByText('已完成')).toBeTruthy()
    expect(screen.getByText(/Tailscale 预热已发起，连接可能尚未就绪/)).toBeTruthy()
    act(() => vi.advanceTimersByTime(6000))
    expect(screen.queryByLabelText('当前操作进度')).toBeNull()
  })

  it.each([
    { kind: 'stop', language: 'zh-Hans' },
    { kind: 'restart-mihomo', language: 'zh-Hans' },
    { kind: 'stop', language: 'en' },
    { kind: 'restart-mihomo', language: 'en' },
  ] as const)('keeps the DNS recovery warning visible when $kind completes in $language', async ({ kind, language }) => {
    await prepareLanguage(language)
    activateLanguage(language)
    begin(kind)
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    act(() => recordOperation({ id: 'op-1', kind, state: 'running', notices: ['system_dns_restore_failed'] }))
    const message = language === 'en'
      ? 'System DNS could not be restored. Other operations will continue. Please check the system DNS settings.'
      : '系统 DNS 未能恢复，其他操作将继续。请检查系统 DNS 设置。'
    expect(screen.getByText(message)).toBeTruthy()
    expect(screen.getByRole('progressbar')).toBeTruthy()
    act(() => recordOperation({ id: 'op-1', kind, state: 'succeeded' }))
    expect(screen.getByText(language === 'en' ? 'Completed' : '已完成')).toBeTruthy()
    expect(screen.getByText(message)).toBeTruthy()
    expect(screen.queryByRole('progressbar')).toBeNull()
  })

  it.each(['manual', 'automatic'])('does not reveal an earlier failure after %s dismissal of the latest result', dismissal => {
    begin()
    recordOperation({ id: 'op-1', kind: 'start', state: 'failed', error: 'previous startup failed' })
    const view = render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.getByText('previous startup failed')).toBeTruthy()

    act(() => {
      vi.advanceTimersByTime(1000)
      const now = new Date().toISOString()
      recordOperation({ id: 'op-2', kind: 'reload', state: 'running', created_at: now, updated_at: now })
    })
    expect(screen.queryByText('previous startup failed')).toBeNull()
    act(() => recordOperation({ id: 'op-2', kind: 'reload', state: 'succeeded' }))
    expect(screen.getByText('已完成')).toBeTruthy()
    if (dismissal === 'manual') fireEvent.click(screen.getByRole('button', { name: '关闭操作进度' }))
    else act(() => vi.advanceTimersByTime(6000))
    expect(screen.queryByLabelText('当前操作进度')).toBeNull()

    // Polling and remounting must not turn completed history into a new notice.
    act(() => recordOperation({ id: 'op-1', kind: 'start', state: 'failed', error: 'previous startup failed' }))
    view.unmount()
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.queryByLabelText('当前操作进度')).toBeNull()
    expect(getOperation('op-1')?.error).toBe('previous startup failed')
  })

  it('keeps the latest result when creation timestamps tie and older status reads arrive later', () => {
    begin()
    recordOperation({ id: 'op-1', kind: 'start', state: 'succeeded' })
    const now = new Date().toISOString()
    recordOperation({ id: 'op-2', kind: 'stop', state: 'failed', error: 'latest stop failed', created_at: now, updated_at: now })
    recordOperation({ id: 'op-1', kind: 'start', state: 'succeeded' })
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.getByText('latest stop failed')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '关闭操作进度' }))
    expect(screen.queryByLabelText('当前操作进度')).toBeNull()
  })

  it('preserves an earlier unconfirmed operation without replaying its eventual result', () => {
    begin('reload')
    markOperationConnection('op-1', 'unknown')
    vi.advanceTimersByTime(1000)
    const now = new Date().toISOString()
    recordOperation({ id: 'op-2', kind: 'restart-mihomo', state: 'running', created_at: now, updated_at: now })
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.getByText('重启 Mihomo')).toBeTruthy()
    expect(screen.getByText('另有 1 个操作进行中')).toBeTruthy()
    act(() => recordOperation({ id: 'op-2', kind: 'restart-mihomo', state: 'succeeded' }))
    expect(screen.getByText('重载网关')).toBeTruthy()
    expect(screen.getByText('结果尚未确认')).toBeTruthy()
    act(() => vi.advanceTimersByTime(6000))
    act(() => recordOperation({ id: 'op-1', kind: 'reload', state: 'failed', error: 'earlier reload failed' }))
    expect(screen.queryByLabelText('当前操作进度')).toBeNull()
  })

  it.each([
    { kind: 'dhcp-probe', title: '检查路由器 DHCP 是否已关闭', result: '本次探测未收到 DHCP OFFER，可以继续启动 OpenSurge。' },
    { kind: 'router-dhcp-restored', title: '检查路由器 DHCP 是否已恢复', result: '已收到 DHCP OFFER，可以继续恢复 Mac 自动 DHCP。' },
  ])('shows $kind progress across page changes and explains the completed probe', ({ kind, title, result }) => {
    begin(kind)
    const first = render(<OperationProgress onOpenDiagnostics={() => {}} />)
    act(() => recordOperation({ id: 'op-1', kind, state: 'running', phase: 'probing_dhcp' }))
    act(() => vi.advanceTimersByTime(3000))
    first.unmount()
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.getByText(title)).toBeTruthy()
    expect(screen.getByText('正在探测 DHCP OFFER')).toBeTruthy()
    expect(screen.getByText('已用时 3 秒')).toBeTruthy()
    expect(screen.getByRole('progressbar')).toBeTruthy()
    act(() => recordOperation({ id: 'op-1', kind, state: 'succeeded' }))
    expect(screen.getByText(result)).toBeTruthy()
    expect(screen.queryByRole('progressbar')).toBeNull()
  })

  it.each(['dhcp-probe', 'router-dhcp-restored'])('renders $kind progress and completion in English', async kind => {
    await prepareLanguage('en')
    activateLanguage('en')
    begin(kind)
    recordOperation({ id: 'op-1', kind, state: 'running', phase: 'probing_dhcp' })
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.getByText('Probing for DHCP OFFER responses')).toBeTruthy()
    expect(screen.getByLabelText('Current operation progress').textContent).not.toMatch(/[\u3400-\u9fff]/)
    act(() => recordOperation({ id: 'op-1', kind, state: 'succeeded' }))
    expect(screen.getByText(/You can proceed to/)).toBeTruthy()
    expect(screen.getByLabelText('Current operation progress').textContent).not.toMatch(/[\u3400-\u9fff]/)
  })

  it('renders stages, notices and unknown outcomes in English', async () => {
    await prepareLanguage('en')
    activateLanguage('en')
    begin('save-device-policy')
    recordOperation({ id: 'op-1', kind: 'save-device-policy', state: 'running', phase: 'validating_device_policy', notices: ['tailscale_warmup_started'] })
    markOperationConnection('op-1', 'unknown')
    render(<OperationProgress onOpenDiagnostics={() => {}} />)
    expect(screen.getByText('Outcome unconfirmed')).toBeTruthy()
    expect(screen.getByText('Validating device identities and routing rules')).toBeTruthy()
    expect(screen.getByLabelText('Current operation progress').textContent).not.toMatch(/[\u3400-\u9fff]/)
  })
})
