import type { ControlConfig } from '../types'
import { t } from '../i18n'

type NetworkMode = ControlConfig['gateway']['mode']

function DiagramDefinitions({ id }: { id: string }) {
  return <defs>
    <filter id={`${id}-shadow`} x="-20%" y="-20%" width="140%" height="150%"><feDropShadow dx="0" dy="9" stdDeviation="11" floodColor="var(--topology-shadow)" floodOpacity=".28" /></filter>
    <marker id={`${id}-traffic`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 10 5 0 10Z" className="topology-traffic-fill" /></marker>
    <marker id={`${id}-ipv4`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 10 5 0 10Z" className="topology-ipv4-fill" /></marker>
    <marker id={`${id}-ipv6`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 10 5 0 10Z" className="topology-ipv6-fill" /></marker>
  </defs>
}

function InternetNode({ shadow }: { shadow: string }) {
  return <g filter={`url(#${shadow})`}>
    <rect x="14" y="126" width="94" height="82" rx="18" className="topology-node" />
    <text className="title" x="61" y="160">Internet</text><text className="small" x="61" y="184">{t('公网')}</text>
  </g>
}

function SharedLANFlow({ id }: { id: string }) {
  return <>
    <path d="M558 190H518" className="topology-traffic" markerEnd={`url(#${id}-traffic)`} />
    <path d="M326 190H288" className="topology-traffic" markerEnd={`url(#${id}-traffic)`} />
    <path d="M154 167H110" className="topology-traffic" markerEnd={`url(#${id}-traffic)`} />
  </>
}

function SharedMacNode({ shadow, dhcp }: { shadow: string; dhcp: boolean }) {
  return <g filter={`url(#${shadow})`}>
    <rect x="326" y="78" width="190" height="176" rx="24" className="topology-mac" />
    <text className="title" x="421" y="110">Mac / OpenSurge</text>
    <rect x="346" y="129" width="150" height="52" rx="12" className="topology-module" />
    <text className="module" x="421" y="151">{dhcp ? 'DHCP + DNS' : t('DNS · 不发 DHCP')}</text><text className="micro ipv6" x="421" y="171">{dhcp ? 'IPv6 RA / DNS' : t('IPv6 不发 RA')}</text>
    <rect x="346" y="190" width="150" height="47" rx="12" className="topology-module" />
    <text className="module" x="421" y="209">{t('规则与转发')}</text><text className="micro" x="421" y="228">IPv4 TUN · <tspan className="ipv6">{t('IPv6 用户态')}</tspan></text>
  </g>
}

function SameWiFiDHCPDiagram() {
  const id = 'same-wifi'
  return <svg className="topology-diagram" viewBox="0 0 720 360" role="img" aria-labelledby={`${id}-title ${id}-desc`}>
    <title id={`${id}-title`}>{t('局域网 DHCP 接管')}</title>
    <desc id={`${id}-desc`}>{t('主路由关闭 DHCP，OpenSurge 为现有局域网中的设备提供 DHCP、DNS 和默认网关。')}</desc>
    <DiagramDefinitions id={id} />
    <rect x="116" y="36" width="590" height="232" rx="24" className="topology-zone" />
    <text className="micro" x="270" y="62">{t('现有局域网 · Wi-Fi 或以太网')}</text>
    <InternetNode shadow={`${id}-shadow`} />
    <g filter={`url(#${id}-shadow)`}>
      <rect x="154" y="105" width="132" height="124" rx="20" className="topology-node" />
      <text className="title" x="220" y="135">{t('主路由 / AP')}</text><path d="M184 153h72" className="topology-divider" />
      <text className="small" x="220" y="174">{t('DHCP 关闭')}</text><text className="small" x="220" y="195">{t('LAN 地址保留')}</text>
      <text className="micro ipv6" x="220" y="216">{t('IPv6：主路由 RA 关闭')}</text>
    </g>
    <SharedMacNode shadow={`${id}-shadow`} dhcp />
    <g filter={`url(#${id}-shadow)`}>
      <rect x="558" y="94" width="136" height="144" rx="22" className="topology-client" />
      <text className="title" x="626" y="128">{t('局域网设备')}</text>
      <text className="small" x="626" y="153">{t('手机 · 电视')}</text><text className="small" x="626" y="173">{t('游戏机 · IoT')}</text>
      <text className="micro ipv6" x="626" y="193">{t('IPv6 可选')}</text>
      <rect x="577" y="201" width="98" height="26" rx="13" className="topology-client-pill" /><text className="micro" x="626" y="219">{t('自动获取设置')}</text>
    </g>
    <SharedLANFlow id={id} />
    <path d="M496 151C526 119 544 119 558 137" className="topology-ipv4" markerEnd={`url(#${id}-ipv4)`} />
    <path d="M496 173C526 154 544 155 558 169" className="topology-ipv6" markerEnd={`url(#${id}-ipv6)`} />
    <g><rect x="488" y="77" width="151" height="28" rx="14" className="topology-ipv4-note" /><circle cx="502" cy="91" r="3" className="topology-ipv4-fill" /><text className="micro start" x="516" y="95">{t('自动下发 DHCP / DNS')}</text></g>
    <g><rect x="488" y="239" width="151" height="28" rx="14" className="topology-ipv6-note" /><circle cx="502" cy="253" r="3" className="topology-ipv6-fill" /><text className="micro ipv6 start" x="516" y="257">{t('可选 IPv6 RA / DNS')}</text></g>
    <g transform="translate(100 310)"><path d="M0 0h34" className="topology-traffic legend-line" /><text className="legend" x="86" y="5">{t('设备上网路径')}</text></g>
    <g transform="translate(306 310)"><path d="M0 0h34" className="topology-ipv4 legend-line" /><text className="legend" x="93" y="5">IPv4 DHCP / DNS</text></g>
    <g transform="translate(531 310)"><path d="M0 0h34" className="topology-ipv6 legend-line" /><text className="legend" x="91" y="5">{t('可选 IPv6 下发')}</text></g>
  </svg>
}

function SameLANDiagram() {
  const id = 'same-lan'
  return <svg className="topology-diagram" viewBox="0 0 720 360" role="img" aria-labelledby={`${id}-title ${id}-desc`}>
    <title id={`${id}-title`}>{t('旁路由模式')}</title>
    <desc id={`${id}-desc`}>{t('主路由保持 DHCP，只在部分设备上手工把固定 IPv4、默认网关和 DNS 指向 OpenSurge。')}</desc>
    <DiagramDefinitions id={id} />
    <rect x="116" y="36" width="590" height="232" rx="24" className="topology-zone" /><text className="micro" x="411" y="62">{t('现有局域网 · Wi-Fi 或以太网')}</text>
    <InternetNode shadow={`${id}-shadow`} />
    <g filter={`url(#${id}-shadow)`}><rect x="154" y="105" width="132" height="124" rx="20" className="topology-node" /><text className="title" x="220" y="135">{t('主路由 / AP')}</text><path d="M184 153h72" className="topology-divider" /><text className="small" x="220" y="174">{t('DHCP 保持开启')}</text><text className="small" x="220" y="195">{t('其他设备不变')}</text><text className="router-ipv6-note" x="220" y="216">{t('选定设备移除 IPv6 默认路由')}</text></g>
    <SharedMacNode shadow={`${id}-shadow`} dhcp={false} />
    <g filter={`url(#${id}-shadow)`}><rect x="558" y="94" width="136" height="144" rx="22" className="topology-client manual" /><text className="title" x="626" y="128">{t('部分设备')}</text><text className="small" x="626" y="153">{t('固定 IPv4')}</text><text className="small" x="626" y="173">{t('网关 / DNS → Mac')}</text><text className="micro ipv6" x="626" y="193">{t('IPv6 ULA · 手工')}</text><rect x="580" y="201" width="92" height="26" rx="13" className="topology-client-pill manual" /><text className="micro" x="626" y="219">{t('仅选定设备')}</text></g>
    <SharedLANFlow id={id} />
    <path d="M558 218C532 268 486 275 454 240" className="topology-ipv4 manual" markerEnd={`url(#${id}-ipv4)`} /><path d="M558 207C540 231 520 247 492 239" className="topology-ipv6" markerEnd={`url(#${id}-ipv6)`} />
    <g><rect x="436" y="269" width="205" height="28" rx="14" className="topology-ipv4-note manual" /><circle cx="450" cy="283" r="3" className="topology-ipv4-fill manual" /><text className="micro start" x="464" y="287">{t('手工设置固定 IPv4、网关与 DNS')}</text></g>
    <g><rect x="479" y="65" width="205" height="28" rx="14" className="topology-ipv6-note" /><circle cx="493" cy="79" r="3" className="topology-ipv6-fill" /><text className="ipv6-note start" x="507" y="83">{t('IPv6：ULA / 链路本地网关与 DNS')}</text></g>
    <g transform="translate(70 326)"><path d="M0 0h34" className="topology-traffic legend-line" /><text className="legend" x="86" y="5">{t('设备上网路径')}</text></g><g transform="translate(280 326)"><path d="M0 0h34" className="topology-ipv4 manual legend-line" /><text className="legend" x="88" y="5">{t('IPv4 手工设置')}</text></g><g transform="translate(505 326)"><path d="M0 0h34" className="topology-ipv6 legend-line" /><text className="legend" x="93" y="5">{t('可选 IPv6 手工接入')}</text></g>
  </svg>
}

function IsolatedLANDiagram() {
  const id = 'isolated-lan'
  return <svg className="topology-diagram isolated" viewBox="0 0 720 360" role="img" aria-labelledby={`${id}-title ${id}-desc`}>
    <title id={`${id}-title`}>{t('独立下游 LAN')}</title><desc id={`${id}-desc`}>{t('Mac 使用不同接口连接上游路由器和独立下游网络，并为全部下游设备提供 DHCP、DNS 和默认网关。')}</desc>
    <DiagramDefinitions id={id} />
    <rect x="4" y="42" width="226" height="210" rx="24" className="topology-zone" /><text className="micro" x="117" y="66">{t('上游网络')}</text><rect x="492" y="42" width="224" height="210" rx="24" className="topology-zone" /><text className="micro" x="604" y="66">{t('独立下游 LAN')}</text>
    <g filter={`url(#${id}-shadow)`}><rect x="16" y="126" width="76" height="78" rx="17" className="topology-node" /><text className="title" x="54" y="158">Internet</text><text className="small" x="54" y="181">{t('公网')}</text></g>
    <g filter={`url(#${id}-shadow)`}><rect x="122" y="104" width="94" height="124" rx="19" className="topology-node" /><text className="title" x="169" y="136">{t('上游路由')}</text><path d="M139 158h60" className="topology-divider" /><text className="small" x="169" y="181">{t('DHCP 开启')}</text><text className="small" x="169" y="204">{t('设置不变')}</text></g>
    <g filter={`url(#${id}-shadow)`}><rect x="272" y="78" width="176" height="174" rx="24" className="topology-mac" /><text className="title" x="360" y="110">Mac / OpenSurge</text><rect x="288" y="126" width="67" height="59" rx="11" className="topology-module" /><text className="module" x="321" y="149">{t('上游接口')}</text><text className="small" x="321" y="170">{t('连接路由器')}</text><rect x="365" y="126" width="67" height="59" rx="11" className="topology-module" /><text className="module" x="398" y="146">{t('下游接口')}</text><text className="micro" x="398" y="164">DHCP + DNS</text><text className="micro ipv6" x="398" y="180">IPv6 RA</text><rect x="288" y="194" width="144" height="42" rx="11" className="topology-module" /><text className="module" x="360" y="211">{t('规则与转发')}</text><text className="micro" x="360" y="229">IPv4 TUN · <tspan className="ipv6">{t('IPv6 用户态')}</tspan></text></g>
    <g filter={`url(#${id}-shadow)`}><rect x="506" y="104" width="86" height="124" rx="19" className="topology-node" /><text className="title" x="549" y="139">AP / VLAN</text><path d="M521 158h56" className="topology-divider" /><text className="small" x="549" y="181">{t('独立 SSID')}</text><text className="small" x="549" y="204">{t('独立网段')}</text></g>
    <g filter={`url(#${id}-shadow)`}><rect x="622" y="104" width="80" height="124" rx="19" className="topology-client" /><text className="title" x="662" y="139">{t('下游设备')}</text><text className="small" x="662" y="165">{t('自动获取')}</text><text className="small" x="662" y="184">{t('全部接入')}</text><text className="micro ipv6" x="662" y="207">{t('IPv6 可选')}</text></g>
    <path d="M622 191H594" className="topology-traffic" markerEnd={`url(#${id}-traffic)`} /><path d="M506 191H450" className="topology-traffic" markerEnd={`url(#${id}-traffic)`} /><path d="M272 191H218" className="topology-traffic" markerEnd={`url(#${id}-traffic)`} /><path d="M122 165H94" className="topology-traffic" markerEnd={`url(#${id}-traffic)`} /><path d="M432 150C466 113 486 113 506 139" className="topology-ipv4" markerEnd={`url(#${id}-ipv4)`} /><path d="M432 175C466 148 486 148 506 168" className="topology-ipv6" markerEnd={`url(#${id}-ipv6)`} />
    <g><rect x="439" y="73" width="177" height="28" rx="14" className="topology-ipv4-note" /><circle cx="452" cy="87" r="3" className="topology-ipv4-fill" /><text className="micro start" x="465" y="91">{t('仅向下游下发 DHCP / DNS')}</text></g><g><rect x="439" y="239" width="177" height="28" rx="14" className="topology-ipv6-note" /><circle cx="452" cy="253" r="3" className="topology-ipv6-fill" /><text className="micro ipv6 start" x="465" y="257">{t('可选下发 IPv6 RA / DNS')}</text></g>
    <g transform="translate(72 310)"><path d="M0 0h34" className="topology-traffic legend-line" /><text className="legend" x="84" y="5">{t('设备上网路径')}</text></g><g transform="translate(270 310)"><path d="M0 0h34" className="topology-ipv4 legend-line" /><text className="legend" x="106" y="5">{t('下游 IPv4 DHCP / DNS')}</text></g><g transform="translate(526 310)"><path d="M0 0h34" className="topology-ipv6 legend-line" /><text className="legend" x="88" y="5">{t('可选 IPv6 下发')}</text></g>
  </svg>
}

export function TopologyDiagram({ mode }: { mode: NetworkMode }) {
  if (mode === 'same_wifi_dhcp') return <SameWiFiDHCPDiagram />
  if (mode === 'same_lan') return <SameLANDiagram />
  return <IsolatedLANDiagram />
}
