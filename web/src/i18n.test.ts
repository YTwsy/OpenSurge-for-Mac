import { afterEach, describe, expect, it } from 'vitest'
import { activateLanguage, prepareLanguage, resolveLanguage, resolveSystemLanguage, t } from './i18n'

describe('interface language', () => {
  afterEach(() => activateLanguage('zh-Hans'))

  it('uses Chinese for any Chinese system preference and English otherwise', () => {
    expect(resolveSystemLanguage(['zh-Hant-HK', 'en-US'])).toBe('zh-Hans')
    expect(resolveSystemLanguage(['en-GB', 'zh-Hans'])).toBe('en')
    expect(resolveSystemLanguage(['ja-JP'])).toBe('en')
    expect(resolveLanguage('system')).toMatch(/^(zh-Hans|en)$/)
  })

  it('translates interface and SVG messages with interpolation', async () => {
    await prepareLanguage('en')
    activateLanguage('en')
    expect(t('网络与 DHCP 接管')).toBe('Network & DHCP Takeover')
    expect(t('已关闭 {{count}} 个连接，等待客户端建立新连接。', { count: 3 })).toBe('Closed 3 connections; waiting for clients to establish new ones.')
    expect(t('主路由关闭 DHCP，OpenSurge 为现有局域网中的设备提供 DHCP、DNS 和默认网关。')).toContain('default gateway')
  })
})
