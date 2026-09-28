import { Select } from '../components/Select'
import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { Empty, PageHeader } from '../components/Common'
import { useConnections } from '../hooks/useConnections'
import { connectionChain, connectionDeviceName, connectionDuration, connectionMetadata, connectionOwnerKey, connectionTarget, filterConnections, type ConnectionsViewState } from '../connections'
import { formatBytes, formatRate } from '../trafficFormat'
import type { DeviceTrafficRow, ObservedConnection, Overview } from '../types'
import { t } from '../i18n'
import './ConnectionsPage.css'

const pageSize = 50
type Props = { overview: Overview | null; view: ConnectionsViewState; onViewChange: (patch: Partial<ConnectionsViewState>) => void; restoreScrollY?: number | null }

export function ConnectionsPage({ overview, view, onViewChange, restoreScrollY }: Props) {
  const [paused, setPaused] = useState(false)
  const { snapshot, error } = useConnections(paused, overview?.status.gateway, overview?.revision)
  const restored = useRef(false)
  const lastSelection = useRef<ObservedConnection | null>(null)
  const devices = useMemo(() => snapshot ? [snapshot.gateway_local, ...snapshot.devices, snapshot.unclassified] : [], [snapshot])
  const owners = useMemo(() => new Map(devices.map(device => [connectionOwnerKey(device), device])), [devices])
  const selectedDevice = devices.find(device => connectionOwnerKey(device) === view.owner || (device.device_id && `device:${device.device_id}` === view.owner))
  const filtered = useMemo(() => filterConnections(snapshot?.connections ?? [], devices, view), [snapshot, devices, view])
  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize))
  const page = Math.min(view.page, pageCount - 1)
  const visible = filtered.slice(page * pageSize, (page + 1) * pageSize)
  const currentSelection = snapshot?.connections.find(connection => connection.id === view.connection)
  if (currentSelection) lastSelection.current = currentSelection
  const selection = currentSelection ?? (lastSelection.current?.id === view.connection ? lastSelection.current : null)
  const unavailable = Boolean(error || snapshot?.connection_error)
  const summary = view.owner === 'all' ? snapshot?.gateway_totals : selectedDevice
  const changeFilter = (patch: Partial<ConnectionsViewState>) => onViewChange({ ...patch, page: 0 })

  useEffect(() => {
    if (!snapshot || restored.current) return
    restored.current = true
    if (restoreScrollY != null) window.scrollTo?.({ top: restoreScrollY, behavior: 'instant' })
  }, [snapshot, restoreScrollY])

  const deviceButton = (device: DeviceTrafficRow) => {
    const key = connectionOwnerKey(device)
    const bypass = device.gateway_target === 'upstream_router'
    return <button type="button" className={`connection-owner ${selectedDevice === device ? 'selected' : ''}`} aria-label={t('查看 {{name}} 的连接', { name: connectionDeviceName(device) })} aria-pressed={selectedDevice === device} key={key} onClick={() => changeFilter({ owner: key, connection: '' })}>
      <span><strong>{connectionDeviceName(device)}</strong>{device.ip && <small>{device.ip}</small>}<small>{deviceObservationLabel(device, !unavailable)}</small></span>
      <span className="connection-owner-count"><strong>{unavailable ? '—' : device.active_connections}</strong><small>{unavailable || bypass ? '—' : `↓ ${formatRate(device.download_rate)}`}</small></span>
    </button>
  }

  return <div className="connections-page">
    <PageHeader eyebrow="CONNECTIONS" title="连接" description="按设备查看访问目标、命中规则与实际出口；统计仅覆盖当前活跃会话。" action={<button type="button" className="connection-pause" disabled={!snapshot} aria-pressed={paused} onClick={() => setPaused(value => !value)}>{t(paused ? '恢复实时更新' : '暂停画面更新')}</button>} />
    <div className="connection-sample-status" role="status"><span>{t(paused ? '画面已暂停' : '每 2 秒更新')}{snapshot && ` · ${t('采样时间 {{time}}', { time: new Date(snapshot.sampled_at).toLocaleTimeString() })}`}</span><span>{t('已登记、有租约和当前有流量是不同状态。')}</span></div>
    {error && <div className="notice warn" role="alert">{t(snapshot ? '更新失败，当前显示的是上次采样：{{error}}' : '读取连接失败：{{error}}', { error })}</div>}
    {snapshot?.connection_error && <div className="notice warn" role="status">{t('连接或身份数据暂不可用，设备清单仍保留：{{error}}', { error: snapshot.connection_error })}</div>}
    {snapshot?.inventory_error && <div className="notice warn" role="status">{t('设备清单不完整：{{error}}', { error: snapshot.inventory_error })}</div>}
    {!snapshot ? <Empty text={t(error ? '暂时无法读取连接。' : '正在读取连接…')} /> : <div className="connection-layout">
      <aside className="connection-owner-list" aria-label={t('按设备筛选连接')}>
        <button type="button" className={`connection-owner ${view.owner === 'all' ? 'selected' : ''}`} aria-pressed={view.owner === 'all'} onClick={() => changeFilter({ owner: 'all', connection: '' })}><span><strong>{t('全部连接')}</strong><small>{t('本机、下游设备与其他来源')}</small></span><strong>{unavailable ? '—' : snapshot.gateway_totals.active_connections}</strong></button>
        {deviceButton(snapshot.gateway_local)}
        <div className="connection-owner-heading">{t('下游设备 {{count}} 台', { count: snapshot.devices.length })}</div>
        {snapshot.devices.map(deviceButton)}
        {snapshot.devices.length === 0 && <p className="connection-owner-empty">{t('暂无已登记或观察到的下游设备')}</p>}
        <div className="connection-owner-heading">{t('其他来源')}</div>
        {deviceButton(snapshot.unclassified)}
      </aside>
      <section className="connection-main" aria-label={t('连接列表')}>
        <div className="connection-main-heading"><div><h2>{view.owner === 'all' ? t('全部连接') : selectedDevice ? connectionDeviceName(selectedDevice) : t('该设备当前不在清单中')}</h2><p>{summary ? t('{{count}} 个活跃连接 · ↑ {{upload}} · ↓ {{download}}', { count: unavailable ? '—' : summary.active_connections, upload: unavailable || selectedDevice?.gateway_target === 'upstream_router' ? '—' : formatRate(summary.upload_rate), download: unavailable || selectedDevice?.gateway_target === 'upstream_router' ? '—' : formatRate(summary.download_rate) }) : view.owner}</p></div></div>
        {selectedDevice && view.owner !== 'gateway-local' && view.owner !== 'unclassified' && <p className="connection-device-addresses"><span>MAC: {selectedDevice.mac || '—'}</span><span>{t('设备地址')}: {(selectedDevice.addresses?.length ? selectedDevice.addresses : [selectedDevice.ip]).filter(Boolean).join(' · ') || '—'}</span></p>}
        {selectedDevice?.gateway_target === 'upstream_router' && <p className="connection-context">{t('IPv4 直连主路由，该路径不在统计范围内；列表仅展示核心实际观察到的连接。')}</p>}
        <div className="connection-filters">
          <input type="search" aria-label={t('搜索连接')} placeholder={t('搜索域名、IP、设备、进程或出口')} value={view.search} onChange={event => changeFilter({ search: event.target.value })} />
          <Select aria-label={t('连接协议')} value={view.protocol} onChange={value => changeFilter({ protocol: value })}><option value="all">{t('全部协议')}</option><option value="tcp">TCP</option><option value="udp">UDP</option></Select>
          <Select aria-label={t('来源地址族')} value={view.family} onChange={value => changeFilter({ family: value })}><option value="all">{t('全部地址族')}</option><option value="ipv4">IPv4</option><option value="ipv6">IPv6</option></Select>
          <Select aria-label={t('出口类型')} value={view.route} onChange={value => changeFilter({ route: value })}><option value="all">{t('全部出口')}</option><option value="direct">DIRECT</option><option value="proxy">{t('代理')}</option><option value="reject">REJECT</option><option value="unknown">{t('未知')}</option></Select>
          <Select aria-label={t('连接排序')} value={view.sort} onChange={value => changeFilter({ sort: value as ConnectionsViewState['sort'] })}><option value="newest">{t('最新建立')}</option><option value="download">{t('下载速率')}</option><option value="upload">{t('上传速率')}</option></Select>
        </div>
        <div className="connection-table-wrap"><table className="connection-table"><thead><tr><th>{t('目标与来源')}</th><th>{t('协议')}</th><th>{t('实际出口链')}</th><th>{t('命中规则')}</th><th>{t('上下行速率')}</th><th>{t('持续时间')}</th></tr></thead><tbody>
          {visible.map(connection => <Fragment key={connection.id}><tr className={view.connection === connection.id ? 'selected' : ''}>
            <td><button type="button" className="connection-target" aria-expanded={view.connection === connection.id} onClick={() => onViewChange({ connection: view.connection === connection.id ? '' : connection.id })}>{connectionTarget(connection)}</button><small>{owners.get(connection.owner_key) ? connectionDeviceName(owners.get(connection.owner_key)!) : t('无法归属')} · {connectionMetadata(connection, 'sourceIP') || '—'}</small></td>
            <td>{connectionMetadata(connection, 'network').toUpperCase() || '—'}<small>{connection.source_family === 'unknown' ? '—' : connection.source_family === 'ipv4' ? 'IPv4' : 'IPv6'}</small></td>
            <td>{connectionChain(connection)}</td><td>{connection.rule || '—'}<small>{connection.rule_payload}</small></td>
            <td className="connection-numeric">↑ {unavailable ? '—' : formatRate(connection.upload_rate)}<small>↓ {unavailable ? '—' : formatRate(connection.download_rate)}</small></td>
            <td>{connectionDuration(connection, snapshot.sampled_at)}</td>
          </tr>{selection?.id === connection.id && <tr className="connection-detail-row"><td colSpan={6}><ConnectionDetail connection={selection} current device={owners.get(selection.owner_key)} onClose={() => onViewChange({ connection: '' })} /></td></tr>}</Fragment>)}
        </tbody></table></div>
        {!visible.length && <Empty text={t(unavailable ? '连接数据暂不可用，请等待恢复。' : view.owner !== 'all' && !selectedDevice ? '该设备可能已离开当前观察范围，可选择全部连接。' : '当前筛选下没有活跃连接；没有连接不代表设备离线。')} />}
        <div className="connection-pagination"><span>{t('筛选后 {{count}} 个连接 · 第 {{page}} / {{pages}} 页', { count: filtered.length, page: page + 1, pages: pageCount })}</span><div><button type="button" disabled={page === 0} onClick={() => onViewChange({ page: page - 1 })}>{t('上一页')}</button><button type="button" disabled={page + 1 >= pageCount} onClick={() => onViewChange({ page: page + 1 })}>{t('下一页')}</button></div></div>
        {selection && !visible.some(connection => connection.id === selection.id) && <ConnectionDetail connection={selection} current={Boolean(currentSelection)} device={owners.get(selection.owner_key)} onClose={() => onViewChange({ connection: '' })} />}
      </section>
    </div>}
  </div>
}

export function deviceObservationLabel(device: DeviceTrafficRow, available = true): string {
  if (device.configuration_state === 'out_of_lan') return t('登记地址不在当前 LAN')
  if (device.configuration_state === 'pending') return t('登记尚未应用')
  if (device.gateway_target === 'upstream_router') return t('IPv4 主路由直连')
  if (device.identity_source === 'unclassified') return t('来源证据不足')
  const source = device.identity_source === 'dhcp_lease' ? t('DHCP 租约') : device.identity_source === 'registered_static' ? t('已应用登记') : device.identity_source === 'gateway_local' ? t('网关本机') : t('未登记 LAN 来源')
  if (!available) return source
  return `${source} · ${t(device.active_connections ? '流量已观察' : '暂无活跃连接')}`
}

function ConnectionDetail({ connection, device, current, onClose }: { connection: ObservedConnection; device?: DeviceTrafficRow; current: boolean; onClose: () => void }) {
  const fields = [
    [t('所属设备'), device ? connectionDeviceName(device) : t('无法归属')],
    [t('设备身份'), device ? deviceObservationLabel(device) : t('来源证据不足')],
    [t('源地址 / 端口'), `${connectionMetadata(connection, 'sourceIP') || '—'} / ${connectionMetadata(connection, 'sourcePort') || '—'}`],
    [t('目标地址 / 端口'), `${connectionMetadata(connection, 'destinationIP') || '—'} / ${connectionMetadata(connection, 'destinationPort') || '—'}`],
    [t('目标域名'), connectionMetadata(connection, 'host') || '—'],
    [t('入站'), [connectionMetadata(connection, 'type'), connectionMetadata(connection, 'inboundName'), connectionMetadata(connection, 'inboundUser')].filter(Boolean).join(' · ') || '—'],
    [t('实际出口链'), connectionChain(connection)],
    [t('命中规则'), [connection.rule, connection.rule_payload].filter(Boolean).join(' · ') || '—'],
    [t('建立时间'), connection.start && Number.isFinite(Date.parse(connection.start)) ? new Date(connection.start).toLocaleString() : '—'],
    [t('会话累计'), `↑ ${formatBytes(connection.upload)} · ↓ ${formatBytes(connection.download)}`],
    ...(connection.owner_key === 'gateway-local' ? [[t('本机进程'), connectionMetadata(connection, 'process') || '—'], [t('进程路径'), connectionMetadata(connection, 'processPath') || '—']] : []),
    [t('连接 ID'), connection.id],
  ]
  return <section className="connection-detail" aria-label={t('连接详情')}><div className="connection-detail-heading"><h3>{t('连接详情')}</h3><button type="button" onClick={onClose}>{t('收起详情')}</button></div>{!current && <p role="status">{t('当前快照中已无此连接，以下保留最后一次观察详情。')}</p>}<dl>{fields.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl></section>
}
