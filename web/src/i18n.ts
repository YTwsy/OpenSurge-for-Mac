export type RequestedLanguage = 'system' | 'zh-Hans' | 'en'
export type ResolvedLanguage = 'zh-Hans' | 'en'

export const languageCacheKey = 'opensurge-ui-language'

const english: Record<string, string> = {
  '网络配置已保存，检查未完成': 'Network configuration saved; checks incomplete',
  '配置已经保存，但无法刷新当前网络检查：{{error}}': 'The configuration was saved, but current network checks could not be refreshed: {{error}}',
  '提醒': 'Reminders',
  '需要处理': 'Needs attention',
  '{{count}} 项提醒': '{{count}} reminders',
  '{{count}} 项需处理': '{{count}} to resolve',
  '配置检查': 'Configuration checks',
  '当前网络检查': 'Current network checks',
  'DHCP 接管已放弃': 'DHCP takeover abandoned',
  '保存网络配置失败': 'Network configuration could not be saved',
  '网络配置已保存': 'Network configuration saved',
  '网络配置已保存，将在下次启动网关时使用。': 'Network configuration saved. It will be used the next time the gateway starts.',
  '自动填写网络失败': 'Could not fill in network settings',
  '网络建议值已填入': 'Network suggestions filled in',
  '来源列表未能更新': 'Source list could not be updated',
  '导入未完成': 'Import failed',
  '配置应用失败': 'Configuration could not be applied',
  '已设为下次启动版本': 'Saved for next start',
  '草稿已导入': 'Draft imported',
  '草稿已刷新': 'Draft refreshed',
  '字段': 'Field',
  '预览': 'Preview',
  '正在加载…': 'Loading…',
  '已禁用': 'Disabled',
  '自定义 DNS': 'Custom DNS',
  '固定域名地址，或为指定域名设置 DNS 上游。': 'Map domains to fixed addresses or choose their upstream DNS.',
  '同时用于 Mac TUN 与使用 OpenSurge DNS 的下游设备。': 'For Mac TUN and downstream devices using OpenSurge DNS.',
  '仅支持 hosts / dns 区块': 'Supports hosts / dns sections only',
  '导入格式与覆盖规则': 'Import formats and override rules',
  '保存草稿后，再应用到网关。': 'Save your draft, then apply it to the gateway.',
  '正在处理…': 'Processing…',
  '应用到网关': 'Apply to the gateway',
  '自定义 DNS 状态': 'Custom DNS status',
  '自定义 DNS 还有未保存的修改，确定离开并放弃这些修改吗？': 'Custom DNS has unsaved changes. Leave and discard them?',
  '固定地址与域名解析策略在“代理与规则源”中管理。': 'Manage fixed addresses and domain resolution policies in Sources.',
  '自定义 DNS 状态暂不可用，可前往管理页面重试。': 'Custom DNS status is unavailable. Open its management page to retry.',
  '管理自定义 DNS': 'Manage custom DNS',
  '草稿已启用': 'Draft enabled',
  '草稿已禁用': 'Draft disabled',
  '请先保存自定义 DNS 草稿，再应用配置。': 'Save the custom DNS draft before applying the configuration.',
  '自定义 DNS 有待应用的修改；关闭开关也需要应用。': 'Custom DNS changes are pending. Disabling custom DNS also requires applying.',
  '校验并应用自定义 DNS': 'Validate and apply custom DNS',
  '校验并保存到下次启动': 'Validate and save for next start',
  '保持当前配置来源，前往网络设置停止后启动网关，应用已保存的草稿。': 'Keep the current configuration source. Stop and start the gateway in Network Settings to apply saved drafts.',
  '下次启动会组合已保存的自定义 DNS 和全局附加配置，并校验后生效。': 'The next start composes saved custom DNS and the global overlay, validates them, and applies the result.',
  '前往网络设置应用': 'Open Network Settings to apply',
  '前往网络设置启动': 'Open Network Settings to start',
  '前往网络设置检查': 'Check Network Settings',
  '附加配置与 DNS 待应用': 'Overlay and DNS changes pending',
  '自定义 DNS 待应用': 'Custom DNS changes pending',
  '应用配置与 DNS 并重载': 'Apply configuration and DNS, then reload',
  '保存配置与 DNS 到下次启动': 'Save configuration and DNS for next start',
  '应用来源、附加配置与自定义 DNS 并重载网关？': 'Apply the source, overlay, and custom DNS, then reload the gateway?',
  '本次会组合所选来源、已保存的全局附加配置和自定义 DNS 草稿；校验失败时保留现有配置。': 'This combines the selected source, saved global overlay, and custom DNS draft. Validation failure preserves the existing configuration.',
  '当前包含 {{count}} 项专家操作。Provider、策略组、DNS 高级选项和低优先级规则在此维护。': 'There are {{count}} expert operations. Manage providers, policy groups, advanced DNS options, and lower-priority rules here.',
  'Provider、策略组、DNS 高级选项和低优先级规则在此维护。': 'Manage providers, policy groups, advanced DNS options, and lower-priority rules here.',
  '固定地址和域名解析策略也可使用下方独立的“自定义 DNS”。': 'Fixed addresses and domain resolution policies can also use the separate Custom DNS section below.',
  '{{hosts}} 个映射 · {{policies}} 个上游策略': '{{hosts}} host mappings · {{policies}} upstream policies',
  '已保存，待启动': 'Saved, pending start',
  '未应用修改': 'Unapplied changes',
  '导入 hosts 或仅含 hosts / dns 的 YAML，同时用于 Mac TUN 与使用 OpenSurge DNS 的下游设备。': 'Import a hosts file or YAML containing only hosts / dns for Mac TUN and downstream devices using OpenSurge DNS.',
  '保存只更新草稿。通过停止后启动网关，或重新应用来源使其生效；禁用也需应用。': 'Saving updates the draft. Stop and start the gateway, or reapply a source to apply it. Disabling also requires applying.',
  '启用自定义 DNS': 'Enable custom DNS',
  '导入 hosts': 'Import hosts',
  '导入 YAML': 'Import YAML',
  '下载示例': 'Download example',
  '导出草稿': 'Export draft',
  '自定义 DNS YAML': 'Custom DNS YAML',
  '同名自定义项覆盖订阅及全局附加配置；删除覆盖项恢复继承。DHCP 名称保留给租约解析。': 'Custom entries override matching subscription and global overlay keys. Removing an override restores inheritance. DHCP names remain reserved for lease resolution.',
  '保存草稿': 'Save draft',
  '草稿已保存，运行中的 DNS 配置未改变。': 'Draft saved. The running DNS configuration has not changed.',
  '重新载入草稿': 'Reload draft',
  '导入替换预览': 'Import replacement preview',
  '文件 {{name}} 将替换草稿中的 {{sections}}，其他区块保留。': '{{name}} will replace {{sections}} in the draft and retain the other sections.',
  '当前草稿': 'Current draft',
  '导入后草稿': 'Draft after import',
  '采用导入内容': 'Accept imported content',
  '最终 DNS 配置': 'Final DNS configuration',
  '预览以已保存的网络设置为准；配置检查不代表真实查询验收。': 'Preview uses saved network settings. Configuration checks do not verify live DNS queries.',
  '配置前提满足': 'Prerequisites met',
  '配置前提未满足': 'Prerequisites not met',
  '下游 DNS': 'Downstream DNS',
  '配置检查通过；应用时仍需通过引擎校验。': 'Configuration checks passed. Engine validation is still required when applying.',
  '自定义内容变化': 'Custom changes',
  '受管设置与系统 DNS 协同': 'Managed settings and system DNS coordination',
  '基础 DNS 内容': 'Base DNS content',
  '没有可用的原始快照': 'Original snapshot unavailable',
  'hosts 先于上游策略匹配；更具体的域名可能优先于通配。节点和 Provider 也可能使用 hosts。': 'Hosts take precedence over upstream policies. More specific domains can take precedence over wildcards. Nodes and providers may also use hosts.',
  'DNS 服务引用': 'DNS service references',
  '覆盖前': 'Before override',
  '覆盖后': 'After override',
  '文件不能超过 2 MiB。': 'Files must not exceed 2 MiB.',
  '文件必须使用 UTF-8 编码。': 'Files must use UTF-8 encoding.',
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
  '正在重新连接后台服务，当前页面已保留。连接恢复后，请重新执行未完成的操作。': 'Reconnecting to the background service. Your current page is preserved. Retry unfinished actions after the connection is restored.',
  '桌面操作未完成，请重试。': 'The desktop action could not be completed. Please try again.',
  '恢复卡已保存。': 'Recovery card saved.',
  '关闭预览': 'Close preview',
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
  }).catch(error => {
    englishCatalogPromise = undefined
    throw error
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
