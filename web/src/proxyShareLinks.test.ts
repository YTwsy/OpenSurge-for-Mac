// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { parseProxyShareLinks } from './proxyShareLinks'

describe('parseProxyShareLinks', () => {
  it('parses modern TLS and transport share links into mihomo proxies', () => {
    const result = parseProxyShareLinks([
      'vless://user-id@vless.example:443?security=reality&type=ws&path=%2Fedge&sni=front.example&pbk=public-key&sid=abcd#VLESS%20Edge',
      'trojan://secret@trojan.example:443?type=grpc&serviceName=tunnel&sni=front.example#Trojan',
      'hy2://auth-token@hy.example:8443?sni=hy.example&obfs=salamander&obfs-password=mask#HY2',
    ].join('\n'))

    expect(result.errors).toEqual([])
    expect(result.proxies).toEqual([
      expect.objectContaining({ name: 'VLESS Edge', type: 'vless', server: 'vless.example', port: 443, uuid: 'user-id', tls: true, network: 'ws' }),
      expect.objectContaining({ name: 'Trojan', type: 'trojan', password: 'secret', tls: true, network: 'grpc' }),
      expect.objectContaining({ name: 'HY2', type: 'hysteria2', password: 'auth-token', obfs: 'salamander', 'obfs-password': 'mask' }),
    ])
    expect(result.proxies[0]['reality-opts']).toEqual({ 'public-key': 'public-key', 'short-id': 'abcd' })
    expect(result.proxies[0]['ws-opts']).toEqual({ path: '/edge' })
  })

  it('parses VMess, Shadowsocks, HTTP, and SOCKS5 links', () => {
    const vmess = btoa(JSON.stringify({ ps: 'VMess', add: 'vmess.example', port: '443', id: 'uuid', aid: '0', scy: 'auto', net: 'ws', path: '/ws', host: 'cdn.example', tls: 'tls', sni: 'cdn.example' }))
    const shadowsocks = btoa('aes-128-gcm:secret')
    const result = parseProxyShareLinks([
      `vmess://${vmess}`,
      `ss://${shadowsocks}@ss.example:8388#SS`,
      'https://alice:secret@http.example:8443#HTTPS',
      'socks5://bob:secret@socks.example:1080#SOCKS',
    ].join('\n'))

    expect(result.errors).toEqual([])
    expect(result.proxies).toEqual([
      expect.objectContaining({ name: 'VMess', type: 'vmess', uuid: 'uuid', tls: true, network: 'ws' }),
      expect.objectContaining({ name: 'SS', type: 'ss', cipher: 'aes-128-gcm', password: 'secret' }),
      expect.objectContaining({ name: 'HTTPS', type: 'http', username: 'alice', password: 'secret', tls: true }),
      expect.objectContaining({ name: 'SOCKS', type: 'socks5', username: 'bob', password: 'secret', udp: true }),
    ])
  })

  it('reports the line and reason without accepting partial input', () => {
    const result = parseProxyShareLinks('ftp://example.com/file\nvless://uuid@example.com')

    expect(result.proxies).toEqual([])
    expect(result.errors).toEqual([
      { line: 1, reason: 'unsupported', scheme: 'ftp' },
      { line: 2, reason: 'missing_port', scheme: 'vless' },
    ])
  })
})
