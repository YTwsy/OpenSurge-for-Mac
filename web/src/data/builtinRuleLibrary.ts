import type { PolicyRuleSet, PolicyTemplate } from '../types'

export const CLAUDE_CODE_SOURCE = {
  label: 'Net.Coffee · Claude Code 域名分流规则大全',
  url: 'https://ip.net.coffee/claude/site.html',
  snapshot: '2026-08-22',
}

export const CLAUDE_CODE_RULE_SET_NAMES: Record<string, string> = {
  'claude-code-domains': 'Claude Code 核心域名',
  'claude-code-extra': 'Claude Code 扩展服务',
  'claude-code-network': 'Claude Code IP / ASN 兜底',
  'ntp-common': 'NTP 通用规则',
}

export const CLAUDE_CODE_RULE_SETS: PolicyRuleSet[] = [
  {
    id: 'claude-code-domains',
    type: 'inline',
    behavior: 'classical',
    payload: [
      'DOMAIN-SUFFIX,anthropic.com',
      'DOMAIN-SUFFIX,claude.ai',
      'DOMAIN-SUFFIX,claude.com',
      'DOMAIN-SUFFIX,clau.de',
      'DOMAIN-SUFFIX,claudemcpclient.com',
      'DOMAIN-SUFFIX,claudemcpcontent.com',
      'DOMAIN-SUFFIX,claudeusercontent.com',
      'DOMAIN,servd-anthropic-website.b-cdn.net',
      'DOMAIN,anthropic.com.cdn.cloudflare.net',
      'DOMAIN,anthropic.auth0.com',
      'DOMAIN,anthropic-com.ghost.io',
    ],
  },
  {
    id: 'claude-code-extra',
    type: 'inline',
    behavior: 'classical',
    payload: [
      'DOMAIN-SUFFIX,sentry.io',
      'DOMAIN-SUFFIX,statsigapi.net',
      'DOMAIN,browser-intake-us5-datadoghq.com',
      'DOMAIN-KEYWORD,datadog',
      'DOMAIN-KEYWORD,sentry',
      'DOMAIN-KEYWORD,sift',
      'DOMAIN-SUFFIX,intercom.io',
      'DOMAIN-SUFFIX,intercomcdn.com',
      'DOMAIN,cdn.usefathom.com',
    ],
  },
  {
    id: 'claude-code-network',
    type: 'inline',
    behavior: 'classical',
    payload: [
      'IP-CIDR,160.79.104.0/21,no-resolve',
      'IP-CIDR6,2607:6bc0::/32,no-resolve',
      'IP-ASN,399358,no-resolve',
    ],
  },
  {
    id: 'ntp-common',
    type: 'inline',
    behavior: 'classical',
    payload: ['DST-PORT,123'],
  },
]

export const CLAUDE_CODE_TEMPLATE: PolicyTemplate = {
  id: 'claude-code',
  rule_sets: CLAUDE_CODE_RULE_SETS.map(ruleSet => ruleSet.id),
}
