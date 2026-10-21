import { useEffect, useState } from 'react'
import type { ExternalDNSFilterConfig } from '../types'
import { t } from '../i18n'

export const defaultExternalDNSFilter: ExternalDNSFilterConfig = { enabled: false, server: '', timeout_ms: 2000, blocked_addresses: [], bypass_domains: [] }

export function ExternalDNSFilterSettings({ value = defaultExternalDNSFilter, onChange }: { value?: ExternalDNSFilterConfig; onChange: (value: ExternalDNSFilterConfig) => void }) {
  const update = (patch: Partial<ExternalDNSFilterConfig>) => onChange({ ...value, ...patch })
  return <div className="config-field wide" role="group" aria-label={t('外部 DNS 过滤')}>
    <div className="config-field-title"><strong>{t('外部 DNS 过滤')}</strong></div>
    <label className={`config-switch ${value.enabled ? 'is-on' : ''}`}>
      <input className="config-switch-input" type="checkbox" aria-label={t('启用外部 DNS 过滤')} checked={value.enabled} onChange={event => update({ enabled: event.target.checked })} />
      <span className="config-switch-copy"><strong>{t('先过滤，再生成 Fake-IP')}</strong><small>{t(value.enabled ? '已开启' : '已关闭')}</small></span>
      <span className="config-switch-toggle" aria-hidden="true"><i /></span>
    </label>
    <small>{t('接入自行部署的 DNS 过滤服务，保留 OpenSurge 设备策略与代理分流。保存后在下次启动网关时应用。')}</small>
    {value.enabled && <>
      <label>{t('过滤服务地址与端口')}<input aria-label={t('过滤服务地址与端口')} placeholder="192.168.1.2:5353" value={value.server} onChange={event => update({ server: event.target.value })} /></label>
      <small>{t('填写 IPv4:端口或 [IPv6]:端口；服务须支持普通 UDP DNS，截断应答会改用 TCP。不要反向转发给 OpenSurge。')}</small>
      <label>{t('过滤超时（毫秒）')}<input type="number" min={100} max={10000} aria-label={t('过滤超时（毫秒）')} value={value.timeout_ms} onChange={event => update({ timeout_ms: Number(event.target.value) })} /></label>
      <label>{t('额外屏蔽 IP')}<CommaListInput label="额外屏蔽 IP" placeholder="192.0.2.99" value={value.blocked_addresses ?? []} onChange={blocked_addresses => update({ blocked_addresses })} /></label>
      <small>{t('默认识别 0.0.0.0 和 ::。仅填写过滤服务约定的屏蔽 IP，以逗号分隔。')}</small>
      <label>{t('跳过过滤的域名后缀')}<CommaListInput label="跳过过滤的域名后缀" placeholder="internal.example" value={value.bypass_domains ?? []} onChange={bypass_domains => update({ bypass_domains })} /></label>
      <small>{t('后缀包含其子域，以逗号分隔；单标签、本地域、反向查询及受管 DHCP、系统搜索域和 MagicDNS 名称自动跳过。')}</small>
      <p className="notice">{t('下游需使用 mihomo DNS 上游；Mac 覆盖范围取决于系统 DNS 与 TUN 接管。过滤服务不可用时查询失败，不自动放行；开启时停用 dnsmasq 缓存。自带 DoH、已有连接及客户端缓存不在本功能范围内。')}</p>
    </>}
  </div>
}

function CommaListInput({ label, placeholder, value, onChange }: { label: string; placeholder: string; value: string[]; onChange: (value: string[]) => void }) {
  const [text, setText] = useState(value.join(', '))
  const canonical = value.join(', ')
  const parse = (input: string) => input.split(',').map(item => item.trim()).filter(Boolean)
  useEffect(() => { if (parse(text).join(', ') !== canonical) setText(canonical) }, [canonical])
  return <input aria-label={t(label)} placeholder={placeholder} value={text} onChange={event => { setText(event.target.value); onChange(parse(event.target.value)) }} />
}
