export type ProxyShareLinkErrorReason =
  | 'invalid'
  | 'unsupported'
  | 'missing_server'
  | 'missing_port'
  | 'missing_credentials'

export type ProxyShareLinkError = {
  line: number
  reason: ProxyShareLinkErrorReason
  scheme?: string
}

export type ProxyShareLinkResult = {
  proxies: Array<Record<string, unknown>>
  errors: ProxyShareLinkError[]
}

type ParsedAuthority = { server: string; port: number }

export function parseProxyShareLinks(value: string): ProxyShareLinkResult {
  const proxies: Array<Record<string, unknown>> = []
  const errors: ProxyShareLinkError[] = []
  const lines = value.split(/\r?\n/)

  lines.forEach((raw, index) => {
    const link = raw.trim()
    if (!link) return
    const scheme = link.match(/^([a-z][a-z\d+.-]*):\/\//i)?.[1]?.toLowerCase() ?? ''
    try {
      switch (scheme) {
        case 'ss':
          proxies.push(parseShadowsocks(link))
          break
        case 'vmess':
          proxies.push(parseVMess(link))
          break
        case 'vless':
          proxies.push(parseVLESS(link))
          break
        case 'trojan':
          proxies.push(parseTrojan(link))
          break
        case 'hysteria2':
        case 'hy2':
          proxies.push(parseHysteria2(link))
          break
        case 'http':
        case 'https':
          proxies.push(parseHTTP(link, scheme === 'https'))
          break
        case 'socks':
        case 'socks5':
          proxies.push(parseSOCKS5(link))
          break
        default:
          errors.push({ line: index + 1, reason: 'unsupported', scheme })
      }
    } catch (cause) {
      const reason = cause instanceof ShareLinkParseFailure ? cause.reason : 'invalid'
      errors.push({ line: index + 1, reason, scheme })
    }
  })

  return { proxies, errors }
}

class ShareLinkParseFailure extends Error {
  constructor(readonly reason: ProxyShareLinkErrorReason) {
    super(reason)
  }
}

function parseVLESS(link: string) {
  const url = parseURL(link)
  const uuid = decodeURLPart(url.username)
  if (!uuid) throw new ShareLinkParseFailure('missing_credentials')
  const authority = requiredAuthority(url)
  const proxy: Record<string, unknown> = {
    name: proxyName(url, 'VLESS', authority),
    type: 'vless',
    server: authority.server,
    port: authority.port,
    uuid,
    udp: true,
  }
  const flow = url.searchParams.get('flow')
  if (flow) proxy.flow = flow
  applyTLSAndTransport(proxy, url.searchParams)
  return proxy
}

function parseTrojan(link: string) {
  const url = parseURL(link)
  const password = decodeURLPart(url.username)
  if (!password) throw new ShareLinkParseFailure('missing_credentials')
  const authority = requiredAuthority(url)
  const proxy: Record<string, unknown> = {
    name: proxyName(url, 'Trojan', authority),
    type: 'trojan',
    server: authority.server,
    port: authority.port,
    password,
    udp: true,
  }
  applyTLSAndTransport(proxy, url.searchParams, true)
  return proxy
}

function parseHysteria2(link: string) {
  const url = parseURL(link.replace(/^hy2:\/\//i, 'hysteria2://'))
  const password = decodeURLPart(url.username || url.password)
  if (!password) throw new ShareLinkParseFailure('missing_credentials')
  const authority = requiredAuthority(url)
  const proxy: Record<string, unknown> = {
    name: proxyName(url, 'Hysteria2', authority),
    type: 'hysteria2',
    server: authority.server,
    port: authority.port,
    password,
  }
  const sni = firstValue(url.searchParams, 'sni', 'peer')
  if (sni) proxy.sni = sni
  if (isTrue(firstValue(url.searchParams, 'insecure', 'allowInsecure'))) proxy['skip-cert-verify'] = true
  const obfs = url.searchParams.get('obfs')
  const obfsPassword = firstValue(url.searchParams, 'obfs-password', 'obfsParam')
  if (obfs) proxy.obfs = obfs
  if (obfsPassword) proxy['obfs-password'] = obfsPassword
  const up = url.searchParams.get('up')
  const down = url.searchParams.get('down')
  if (up) proxy.up = up
  if (down) proxy.down = down
  return proxy
}

function parseVMess(link: string) {
  const encoded = link.slice(link.indexOf('://') + 3).split('#', 1)[0].split('?', 1)[0]
  let payload: Record<string, unknown>
  try {
    payload = JSON.parse(decodeBase64Text(encoded)) as Record<string, unknown>
  } catch {
    throw new ShareLinkParseFailure('invalid')
  }
  const server = textValue(payload.add)
  const port = numberValue(payload.port)
  const uuid = textValue(payload.id)
  if (!server) throw new ShareLinkParseFailure('missing_server')
  if (!port) throw new ShareLinkParseFailure('missing_port')
  if (!uuid) throw new ShareLinkParseFailure('missing_credentials')
  const proxy: Record<string, unknown> = {
    name: textValue(payload.ps) || `VMess ${server}:${port}`,
    type: 'vmess',
    server,
    port,
    uuid,
    alterId: numberValue(payload.aid) ?? 0,
    cipher: textValue(payload.scy) || 'auto',
    udp: true,
  }
  const network = textValue(payload.net) || 'tcp'
  if (network !== 'tcp') proxy.network = network
  const tls = textValue(payload.tls)
  if (tls && tls !== 'none') proxy.tls = true
  const servername = textValue(payload.sni)
  if (servername) proxy.servername = servername
  const fingerprint = textValue(payload.fp)
  if (fingerprint) proxy['client-fingerprint'] = fingerprint
  applyTransport(proxy, network, {
    path: textValue(payload.path),
    host: textValue(payload.host),
    serviceName: textValue(payload.path),
  })
  return proxy
}

function parseShadowsocks(link: string) {
  const body = link.slice(link.indexOf('://') + 3)
  const hashIndex = body.indexOf('#')
  const beforeHash = hashIndex >= 0 ? body.slice(0, hashIndex) : body
  const rawName = hashIndex >= 0 ? body.slice(hashIndex + 1) : ''
  const queryIndex = beforeHash.indexOf('?')
  const rawPayload = queryIndex >= 0 ? beforeHash.slice(0, queryIndex) : beforeHash
  const params = new URLSearchParams(queryIndex >= 0 ? beforeHash.slice(queryIndex + 1) : '')

  let credentials = ''
  let authorityText = ''
  const at = rawPayload.lastIndexOf('@')
  if (at >= 0) {
    credentials = decodeCredentials(rawPayload.slice(0, at))
    authorityText = rawPayload.slice(at + 1)
  } else {
    const decoded = decodeBase64Text(rawPayload)
    const decodedAt = decoded.lastIndexOf('@')
    if (decodedAt < 0) throw new ShareLinkParseFailure('invalid')
    credentials = decoded.slice(0, decodedAt)
    authorityText = decoded.slice(decodedAt + 1)
  }
  const separator = credentials.indexOf(':')
  if (separator <= 0) throw new ShareLinkParseFailure('missing_credentials')
  const cipher = credentials.slice(0, separator)
  const password = credentials.slice(separator + 1)
  if (!password) throw new ShareLinkParseFailure('missing_credentials')
  const authority = parseAuthority(authorityText)
  const proxy: Record<string, unknown> = {
    name: decodeURLPart(rawName) || `SS ${authority.server}:${authority.port}`,
    type: 'ss',
    server: authority.server,
    port: authority.port,
    cipher,
    password,
    udp: true,
  }
  const pluginSpec = params.get('plugin')
  if (pluginSpec) {
    const [rawPlugin, ...options] = pluginSpec.split(';')
    proxy.plugin = normalizeSSPlugin(rawPlugin)
    if (options.length) {
      proxy['plugin-opts'] = Object.fromEntries(options.map(option => {
        const [key, ...rest] = option.split('=')
        return [key, rest.length ? rest.join('=') : true]
      }))
    }
  }
  return proxy
}

function parseHTTP(link: string, tls: boolean) {
  const url = parseURL(link)
  const authority = requiredAuthority(url)
  const proxy: Record<string, unknown> = {
    name: proxyName(url, tls ? 'HTTPS' : 'HTTP', authority),
    type: 'http',
    server: authority.server,
    port: authority.port,
  }
  const username = decodeURLPart(url.username)
  const password = decodeURLPart(url.password)
  if (username) proxy.username = username
  if (password) proxy.password = password
  if (tls) proxy.tls = true
  return proxy
}

function parseSOCKS5(link: string) {
  const url = parseURL(link.replace(/^socks:\/\//i, 'socks5://'))
  const authority = requiredAuthority(url)
  const proxy: Record<string, unknown> = {
    name: proxyName(url, 'SOCKS5', authority),
    type: 'socks5',
    server: authority.server,
    port: authority.port,
    udp: true,
  }
  const username = decodeURLPart(url.username)
  const password = decodeURLPart(url.password)
  if (username) proxy.username = username
  if (password) proxy.password = password
  return proxy
}

function applyTLSAndTransport(proxy: Record<string, unknown>, params: URLSearchParams, forceTLS = false) {
  const security = params.get('security')?.toLowerCase()
  if (forceTLS || security === 'tls' || security === 'reality') proxy.tls = true
  const servername = firstValue(params, 'sni', 'servername', 'peer')
  if (servername) proxy.servername = servername
  if (isTrue(firstValue(params, 'allowInsecure', 'insecure'))) proxy['skip-cert-verify'] = true
  const fingerprint = firstValue(params, 'fp', 'fingerprint')
  if (fingerprint) proxy['client-fingerprint'] = fingerprint
  if (security === 'reality') {
    const publicKey = firstValue(params, 'pbk', 'public-key')
    const shortID = firstValue(params, 'sid', 'short-id')
    proxy['reality-opts'] = {
      ...(publicKey ? { 'public-key': publicKey } : {}),
      ...(shortID ? { 'short-id': shortID } : {}),
    }
  }
  const network = params.get('type') || 'tcp'
  if (network !== 'tcp') proxy.network = network
  applyTransport(proxy, network, {
    path: params.get('path') || '',
    host: firstValue(params, 'host', 'authority'),
    serviceName: firstValue(params, 'serviceName', 'service-name'),
  })
}

function applyTransport(proxy: Record<string, unknown>, network: string, options: { path: string; host: string; serviceName: string }) {
  if (network === 'ws') {
    proxy['ws-opts'] = {
      ...(options.path ? { path: options.path } : {}),
      ...(options.host ? { headers: { Host: options.host } } : {}),
    }
  } else if (network === 'grpc') {
    proxy['grpc-opts'] = options.serviceName ? { 'grpc-service-name': options.serviceName } : {}
  } else if (network === 'h2') {
    proxy['h2-opts'] = {
      ...(options.path ? { path: options.path } : {}),
      ...(options.host ? { host: [options.host] } : {}),
    }
  } else if (network === 'http') {
    proxy['http-opts'] = {
      ...(options.path ? { path: [options.path] } : {}),
      ...(options.host ? { headers: { Host: [options.host] } } : {}),
    }
  } else if (network === 'xhttp') {
    proxy['xhttp-opts'] = options.path ? { path: options.path } : {}
  }
}

function parseURL(link: string) {
  try {
    return new URL(link)
  } catch {
    throw new ShareLinkParseFailure('invalid')
  }
}

function requiredAuthority(url: URL): ParsedAuthority {
  const server = stripIPv6Brackets(url.hostname)
  const port = numberValue(url.port)
  if (!server) throw new ShareLinkParseFailure('missing_server')
  if (!port) throw new ShareLinkParseFailure('missing_port')
  return { server, port }
}

function parseAuthority(value: string): ParsedAuthority {
  const url = parseURL(`http://${value}`)
  return requiredAuthority(url)
}

function proxyName(url: URL, type: string, authority: ParsedAuthority) {
  return decodeURLPart(url.hash.slice(1)) || `${type} ${authority.server}:${authority.port}`
}

function decodeCredentials(value: string) {
  const decoded = decodeURLPart(value)
  if (decoded.includes(':')) return decoded
  return decodeBase64Text(decoded)
}

function decodeBase64Text(value: string) {
  try {
    const normalized = decodeURLPart(value).replaceAll('-', '+').replaceAll('_', '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const binary = atob(padded)
    const bytes = Uint8Array.from(binary, character => character.charCodeAt(0))
    return new TextDecoder().decode(bytes)
  } catch {
    throw new ShareLinkParseFailure('invalid')
  }
}

function decodeURLPart(value: string) {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function stripIPv6Brackets(value: string) {
  return value.startsWith('[') && value.endsWith(']') ? value.slice(1, -1) : value
}

function textValue(value: unknown) {
  return typeof value === 'string' ? value.trim() : typeof value === 'number' ? String(value) : ''
}

function numberValue(value: unknown) {
  const parsed = typeof value === 'number' ? value : Number(textValue(value))
  return Number.isInteger(parsed) && parsed >= 1 && parsed <= 65535 ? parsed : undefined
}

function firstValue(params: URLSearchParams, ...names: string[]) {
  for (const name of names) {
    const value = params.get(name)
    if (value) return value
  }
  return ''
}

function isTrue(value: string) {
  return ['1', 'true', 'yes'].includes(value.toLowerCase())
}

function normalizeSSPlugin(value: string) {
  return ['obfs-local', 'simple-obfs'].includes(value) ? 'obfs' : value
}
