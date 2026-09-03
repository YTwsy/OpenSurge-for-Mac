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
