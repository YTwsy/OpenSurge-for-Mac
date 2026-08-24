export type RequestedLanguage = 'system' | 'zh-Hans' | 'en'
export type ResolvedLanguage = 'zh-Hans' | 'en'

const languageCacheKey = 'opensurge-ui-language'

const english: Record<string, string> = {
  '语言': 'Language',
  '界面语言': 'Interface language',
  '选择 OpenSurge Web GUI 和菜单栏使用的语言': 'Choose the language used by the OpenSurge Web GUI and menu bar',
  '跟随系统': 'Follow System',
  '简体中文': '简体中文',
  '英语': 'English',
  '系统语言：{{language}}': 'System language: {{language}}',
  '正在保存语言…': 'Saving language…',
  '总览': 'Overview',
  '网络设置': 'Network',
  '代理与规则源': 'Sources',
  '设备': 'Devices',
  '策略': 'Policies',
  '连通性': 'Connectivity',
  '诊断': 'Diagnostics',
  '设备页还有尚未保存的修改，确定离开并放弃这些修改吗？': 'The Devices page has unsaved changes. Leave and discard them?',
  '阻止空闲睡眠和合盖睡眠。合盖运行可能明显增加耗电与发热，请勿放入不通风的包内。': 'Prevent idle and lid-closed sleep. Running with the lid closed can noticeably increase power use and heat; do not place the Mac in an unventilated bag.',
  '正在切换…': 'Changing…',
  '合盖保持运行': 'Keep running with lid closed',
  '系统睡眠已临时禁用': 'System sleep is temporarily disabled',
  '默认关闭 · 本次运行有效': 'Off by default · This session only',
  '切换为浅色模式': 'Switch to light mode',
  '切换为深色模式': 'Switch to dark mode',
  '浅色模式': 'Light mode',
  '深色模式': 'Dark mode',
  'Web GUI 与 OpenSurge 的安全连接已过期': 'The secure connection between the Web GUI and OpenSurge has expired',
  '请点击 macOS 菜单栏中的 OpenSurge 图标，然后选择“打开 OpenSurge 面板”。': 'Click the OpenSurge icon in the macOS menu bar, then choose “Open OpenSurge Dashboard”.',
  '重试': 'Retry',
  '局域网 DHCP 接管': 'Same-LAN DHCP Takeover',
  '让现有局域网设备自动接入 OpenSurge': 'Automatically connect devices on the existing LAN to OpenSurge',
  '自动接管': 'Automatic takeover',
  '保持自动获取': 'Keep automatic network settings',
  '关闭主路由 DHCP': 'Disable DHCP on the main router',
  '局域网中使用自动网络设置的设备': 'LAN devices using automatic network settings',
  'OpenSurge 会通过引导流程，协助你逐步完成网络设置、启动确认和停止后的网络恢复。': 'OpenSurge guides you through network setup, startup confirmation, and network recovery after stopping.',
  '主路由关闭 DHCP，OpenSurge 为现有局域网中的设备提供 DHCP、DNS 和默认网关。': 'The main router stops DHCP, and OpenSurge provides DHCP, DNS, and the default gateway to devices on the existing LAN.',
  '旁路由模式': 'Selective Gateway',
  '仅让局域网内的部分设备通过 OpenSurge 上网': 'Route only selected LAN devices through OpenSurge',
  '部分设备': 'Selected clients',
  '主路由': 'Main router',
  '在部分设备上手工设置网关与 DNS': 'Set the gateway and DNS manually on selected devices',
  '只修改需要接入的设备': 'Change only the devices you want to connect',
  '手工设置为使用 OpenSurge 的设备': 'Devices manually configured to use OpenSurge',
  '主路由和其他设备保持原有网络设置；没有手工设置网关的设备不受影响。': 'The main router and other devices keep their existing settings; devices not pointed to OpenSurge are unaffected.',
  '主路由保持 DHCP，只在部分设备上手工把固定 IPv4、默认网关和 DNS 指向 OpenSurge。': 'The main router keeps DHCP enabled; only selected devices manually use a fixed IPv4 and point their default gateway and DNS to OpenSurge.',
  '独立下游 LAN': 'Isolated Downstream LAN',
  '通过独立 AP、SSID 或 VLAN 接入 OpenSurge': 'Connect to OpenSurge through a dedicated AP, SSID, or VLAN',
  '独立网络': 'Isolated network',
  'OpenSurge（仅下游）': 'OpenSurge (downstream only)',
  '连接下游网络后自动获取': 'Connect downstream and obtain settings automatically',
  '准备独立下游网络和接口': 'Prepare a separate downstream network and interface',
  '连接到下游网络的设备': 'Devices connected to the downstream network',
  '上游路由器通常无需改变；OpenSurge 只接管独立下游网络中的设备。': 'The upstream router usually needs no changes; OpenSurge takes over only devices on the isolated downstream network.',
  'Mac 使用不同接口连接上游路由器和独立下游网络，并为全部下游设备提供 DHCP、DNS 和默认网关。': 'The Mac uses separate interfaces for the upstream router and isolated downstream network, providing DHCP, DNS, and the default gateway to all downstream devices.',
  'DHCP 来源': 'DHCP source',
  '设备设置': 'Device setup',
  '需要操作': 'Required action',
  '接入范围': 'Affected devices',
  '公网': 'Public network',
  '现有局域网 · Wi-Fi 或以太网': 'Existing LAN · Wi-Fi or Ethernet',
  '主路由 / AP': 'Router / AP',
  'DHCP 关闭': 'DHCP off',
  'LAN 地址保留': 'Keep LAN address',
  'IPv6：主路由 RA 关闭': 'IPv6: router RA off',
  '规则与转发': 'Rules & forwarding',
  'IPv6 用户态': 'IPv6 userspace',
  '局域网设备': 'LAN devices',
  '手机 · 电视': 'Phones · TVs',
  '游戏机 · IoT': 'Consoles · IoT',
  'IPv6 可选': 'Optional IPv6',
  '自动获取设置': 'Automatic setup',
  '自动下发 DHCP / DNS': 'Automatic DHCP / DNS',
  '可选 IPv6 RA / DNS': 'Optional IPv6 RA / DNS',
  '设备上网路径': 'Device traffic',
  '可选 IPv6 下发': 'Optional IPv6',
  'DHCP 保持开启': 'DHCP stays on',
  '其他设备不变': 'Others unchanged',
  '选定设备移除 IPv6 默认路由': 'No router IPv6 route',
  'DNS · 不发 DHCP': 'DNS · No DHCP',
  'IPv6 不发 RA': 'No IPv6 RA',
  '固定 IPv4': 'Fixed IPv4',
  '网关 / DNS → Mac': 'Gateway / DNS → Mac',
  'IPv6 ULA · 手工': 'IPv6 ULA · Manual',
  '仅选定设备': 'Selected only',
  '手工设置固定 IPv4、网关与 DNS': 'Set IPv4, gateway & DNS',
  'IPv6：ULA / 链路本地网关与 DNS': 'IPv6: ULA / link-local gateway & DNS',
  'IPv4 手工设置': 'Manual IPv4 setup',
  '可选 IPv6 手工接入': 'Optional manual IPv6',
  '上游网络': 'Upstream network',
  '上游路由': 'Router',
  'DHCP 开启': 'DHCP on',
  '设置不变': 'Unchanged',
  '上游接口': 'Upstream',
  '连接路由器': 'To router',
  '下游接口': 'Downstream',
  '独立 SSID': 'Dedicated SSID',
  '独立网段': 'Separate subnet',
  '下游设备': 'Clients',
  '自动获取': 'Automatic',
  '全部接入': 'All connected',
  '仅向下游下发 DHCP / DNS': 'Downstream DHCP / DNS only',
  '可选下发 IPv6 RA / DNS': 'Optional downstream IPv6',
  '下游 IPv4 DHCP / DNS': 'Downstream IPv4 DHCP / DNS',
}

let englishCatalogPromise: Promise<void> | undefined

export function prepareLanguage(requested: RequestedLanguage): Promise<void> {
  if (resolveLanguage(requested) !== 'en') return Promise.resolve()
  englishCatalogPromise ??= import('./i18n.en').then(({ englishMessages }) => {
    Object.assign(english, englishMessages)
  })
  return englishCatalogPromise
}

let activeLanguage: ResolvedLanguage = import.meta.env.MODE === 'test' ? 'zh-Hans' : resolveSystemLanguage()

export function isRequestedLanguage(value: unknown): value is RequestedLanguage {
  return value === 'system' || value === 'zh-Hans' || value === 'en'
}

export function resolveSystemLanguage(languages: readonly string[] = typeof navigator === 'undefined' ? [] : navigator.languages): ResolvedLanguage {
  const first = languages[0]?.toLowerCase() ?? ''
  return first === 'zh' || first.startsWith('zh-') ? 'zh-Hans' : 'en'
}

export function resolveLanguage(requested: RequestedLanguage): ResolvedLanguage {
  return requested === 'system' ? resolveSystemLanguage() : requested
}

export function initialRequestedLanguage(): RequestedLanguage {
  if (import.meta.env.MODE === 'test') return 'zh-Hans'
  if (typeof window === 'undefined') return 'system'
  const cached = window.localStorage.getItem(languageCacheKey)
  return isRequestedLanguage(cached) ? cached : 'system'
}

export function activateLanguage(requested: RequestedLanguage) {
  activeLanguage = resolveLanguage(requested)
  if (typeof document !== 'undefined') document.documentElement.lang = activeLanguage
}

export function cacheRequestedLanguage(requested: RequestedLanguage) {
  if (typeof window !== 'undefined') window.localStorage.setItem(languageCacheKey, requested)
}

export function currentLanguage(): ResolvedLanguage {
  return activeLanguage
}

export function localeIdentifier(): string {
  return activeLanguage === 'zh-Hans' ? 'zh-CN' : 'en-US'
}

export function t(source: string, values: Record<string, string | number> = {}): string {
  const template = activeLanguage === 'en' ? english[source] ?? source : source
  return template.replace(/\{\{([A-Za-z0-9_]+)\}\}/g, (_, key: string) => String(values[key] ?? `{{${key}}}`))
}

export function languageDisplayName(language: ResolvedLanguage): string {
  return language === 'zh-Hans' ? '简体中文' : 'English'
}

export function hasEnglishTranslation(source: string): boolean {
  return Object.hasOwn(english, source)
}

export function registerEnglishMessages(messages: Record<string, string>) {
  Object.assign(english, messages)
}
