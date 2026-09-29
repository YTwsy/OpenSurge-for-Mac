import { useId, useState } from 'react'
import { connectionDeviceName, connectionOwnerKey } from '../connections'
import { t } from '../i18n'
import { buildSmoothChart } from '../trafficChart'
import { formatRate } from '../trafficFormat'
import { useTrayActivity } from './useTrayActivity'

export function TrayActivity({ gateway, onOpen }: { gateway?: string; onOpen: (page: string, options?: { owner?: string; section?: 'active-devices' }) => void }) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const detailID = useId()
  const toggle = (key: string) => setExpanded(current => current === key ? null : key)
  const { traffic, routing, history, deviceKeys, trafficFailed, routingFailed } = useTrayActivity(gateway)
  const running = gateway === 'running' || gateway === 'degraded'
  const sampled = Boolean(traffic && history.length > 1)
  const maximum = Math.max(1, ...history.flatMap(point => [point.download, point.upload]))
  const download = buildSmoothChart(history.map(point => point.download), maximum, 4, 46)
  const upload = buildSmoothChart(history.map(point => point.upload), maximum, 4, 46)
  const mode = routing ? ({ rule: '按规则', global: '固定出口', direct: '本机直连' } as const)[routing.mode] : null
  const devices = traffic ? [traffic.gateway_local, ...deviceKeys.flatMap(key => traffic.devices.filter(device => connectionOwnerKey(device) === key))] : []
  const activeDevices = traffic?.devices.filter(device => device.active_connections > 0).length ?? 0
  const connections = traffic ? traffic.gateway_local.active_connections + traffic.totals.active_connections + (traffic.unclassified_connections ?? 0) : 0
  return <>
    <section className={`tray-traffic ${running && sampled ? 'expanded' : ''}`} aria-label={t('经 OpenSurge 的实时流量')}>
      <div className="tray-traffic-reveal" aria-hidden={!running || !sampled}><div className="tray-detail-inner"><div className="tray-rates">
        <div><span><i className="download-key" />{t('下载')}</span><strong>{sampled ? formatRate(traffic!.gateway_rates.download) : '—'}</strong></div>
        <div><span><i className="upload-key" />{t('上传')}</span><strong>{sampled ? formatRate(traffic!.gateway_rates.upload) : '—'}</strong></div>
      </div>
      <svg className="tray-chart" viewBox="0 0 100 50" preserveAspectRatio="none" aria-hidden="true">
        <path className="chart-baseline" d="M 0 46 H 100" />
        {sampled && <><path className="download-area" d={download.areaPath} /><path className="download-line" d={download.linePath} /><path className="upload-line" d={upload.linePath} /></>}
      </svg></div></div>
      <div className="tray-traffic-caption"><p className="tray-caption" role="status">{t(gateway === 'stopped' ? '启动网关后显示实时流量' : !running ? '等待网关状态…' : trafficFailed ? '流量暂不可用' : !sampled ? '正在采样流量…' : '近 60 秒 · 经 OpenSurge 的流量')}</p>{gateway === 'stopped' && <button className="tray-text-button" onClick={() => onOpen('network')}>{t('前往启动')} <span aria-hidden="true">›</span></button>}</div>
    </section>
    <div className={`tray-routing-group ${expanded === 'routing' ? 'expanded' : ''}`}>
      <button className="tray-link-row tray-routing" aria-expanded={expanded === 'routing'} aria-controls={`${detailID}-routing`} onClick={() => toggle('routing')}>
        <span>{t('本机出口')}</span><span className="tray-row-value">{mode ? t(mode) : t(!running ? '网关未运行' : routingFailed ? '暂不可用' : '正在读取…')}{routing?.mode === 'global' && routing.global_group && <small title={routing.global_group.selected}>{routing.global_group.selected}</small>}</span><svg className="tray-disclosure" viewBox="0 0 12 12" aria-hidden="true"><path d="m4 2 4 4-4 4" /></svg>
      </button>
      <div className="tray-detail-reveal" id={`${detailID}-routing`} aria-hidden={expanded !== 'routing'} inert={expanded !== 'routing'}><div className="tray-detail-inner"><div className="tray-routing-details">
        <p className="tray-hint">{t('仅影响本机 Mac，下游设备保持各自的出口设置。')}</p>
        {routing && <dl className="tray-status">
          <div><dt>{t('出口方式')}</dt><dd>{mode ? t(mode) : '—'}</dd></div>
          {routing.global_group && <div><dt>{t('当前出口')}</dt><dd title={routing.global_group.selected}>{routing.global_group.selected}</dd></div>}
          <div><dt>UDP</dt><dd>{t(({ rules: '按规则', proxy: '代理', direct: '直连', reject: '拒绝' } as const)[routing.udp_behavior])}</dd></div>
          <div><dt>{t('接入方式')}</dt><dd>{routing.transports.map(transport => transport === 'tun' ? 'TUN' : t('显式代理')).join(' · ') || '—'}</dd></div>
        </dl>}
        {(routing?.warning || (routing && !routing.consistent)) && <p className="tray-inline-warning">{t('本机出口需要检查')}</p>}
        <button className="tray-text-button" onClick={() => onOpen('devices')}>{t('在桌面窗口中设置')} <span aria-hidden="true">↗</span></button>
      </div></div></div>
    </div>
    <section className="tray-devices" aria-label={t('活跃设备')}>
      <div className="tray-section-heading"><h2>{t('活跃设备')}</h2><button className="tray-text-button" onClick={() => onOpen('dashboard', { section: 'active-devices' })}>{t('查看全部')} <span aria-hidden="true">›</span></button></div>
      {devices.length ? <div className="tray-device-list">{devices.map((device, index) => {
        const key = connectionOwnerKey(device)
        const name = index === 0 ? t('本机 Mac') : connectionDeviceName(device)
        const open = expanded === key
        return <div className={`tray-device-group ${open ? 'expanded' : ''}`} key={key}><button className="tray-device" onClick={() => toggle(key)} aria-expanded={open} aria-controls={`${detailID}-${index}`} aria-label={t('设备详情：{{name}}', { name })}>
        <span className={`tray-device-glyph ${index === 0 ? 'mac' : 'device'}`} aria-hidden="true"><svg viewBox="0 0 20 20"><rect x={index === 0 ? 2 : 5} y="3" width={index === 0 ? 16 : 10} height="12" rx="2" /><path d={index === 0 ? 'M 7 18 H 13 M 10 15 V 18' : 'M 9 13 H 11'} /></svg></span>
        <span className="tray-device-name">{name}</span><span className="tray-device-rate">↓ {sampled ? formatRate(device.download_rate) : '—'}</span><svg className="tray-disclosure" viewBox="0 0 12 12" aria-hidden="true"><path d="m4 2 4 4-4 4" /></svg>
      </button><div className="tray-detail-reveal" id={`${detailID}-${index}`} aria-hidden={!open} inert={!open}><div className="tray-detail-inner"><div className="tray-device-details">
        <dl className="tray-status">
          <div><dt>{t('设备地址')}</dt><dd title={device.addresses?.join(' · ') || device.ip}>{device.addresses?.join(' · ') || device.ip || '—'}</dd></div>
          <div><dt>{t('活跃连接')}</dt><dd>{device.active_connections}</dd></div>
          <div><dt>{t('上传')}</dt><dd>{sampled ? formatRate(device.upload_rate) : '—'}</dd></div>
          <div><dt>{t('下载')}</dt><dd>{sampled ? formatRate(device.download_rate) : '—'}</dd></div>
          {device.primary_egress && <div><dt>{t('当前出口')}</dt><dd title={device.primary_egress}>{device.primary_egress}</dd></div>}
        </dl>
        <button className="tray-text-button" onClick={() => onOpen('connections', { owner: key })} aria-label={t('查看 {{name}} 的连接', { name })}>{t('在桌面窗口中查看连接')} <span aria-hidden="true">↗</span></button>
      </div></div></div></div>
      })}</div> : <p className="tray-empty">{t(!running ? '启动网关后显示活跃设备' : trafficFailed ? '设备流量暂不可用' : '正在读取活跃设备…')}</p>}
      <p className="tray-caption">{traffic ? t('活跃下游 {{devices}} 台 · 总连接 {{connections}}', { devices: activeDevices, connections }) : t('本机 Mac 与下游设备分别统计')}</p>
    </section>
  </>
}
