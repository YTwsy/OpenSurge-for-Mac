import { useEffect, useState } from 'react'
import { api } from '../api'
import { t } from '../i18n'
import type { CustomDNSDraft } from '../types'

export function customDNSStatus(value: CustomDNSDraft, running: boolean): string {
  if (!value.desired) return '未应用修改'
  if (value.applied) return '已应用'
  if (!running && !value.document.enabled) return '已禁用'
  return running ? '待重载' : '已保存，待启动'
}

export function CustomDNSSummary({ contextKey, running, onOpen }: { contextKey: string; running: boolean; onOpen: () => void }) {
  const [value, setValue] = useState<CustomDNSDraft | null>(null)
  const [error, setError] = useState(false)
  useEffect(() => {
    let active = true
    void api.customDNS().then(next => { if (active) { setValue(next); setError(false) } }).catch(() => { if (active) setError(true) })
    return () => { active = false }
  }, [contextKey])
  return <section className="section custom-dns-summary" aria-label={t('自定义 DNS 状态')}>
    <div><strong>{t('自定义 DNS')}</strong><p>{t('固定地址与域名解析策略在“代理与规则源”中管理。')}</p>
      {value && !error && <div className="custom-dns-actions"><span>{t(value.document.enabled ? '草稿已启用' : '草稿已禁用')}</span><span className="pill">{t(customDNSStatus(value, running))}</span><span>{t('{{hosts}} 个映射 · {{policies}} 个上游策略', { hosts: value.hosts, policies: value.policies })}</span></div>}
      {error ? <p role="status">{t('自定义 DNS 状态暂不可用，可前往管理页面重试。')}</p> : !value && <p>{t('正在加载…')}</p>}
    </div>
    <button type="button" onClick={onOpen}>{t('管理自定义 DNS')}</button>
  </section>
}
