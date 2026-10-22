// @vitest-environment jsdom
import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { ExternalDNSFilterSettings, defaultExternalDNSFilter } from './ExternalDNSFilterSettings'

describe('external DNS filter settings', () => {
  it('keeps the draft across disable and re-enable, including server and bypass domains', async () => {
    function Editor() { const [value, setValue] = useState(defaultExternalDNSFilter); return <ExternalDNSFilterSettings value={value} onChange={setValue} /> }
    render(<Editor />)
    expect(screen.queryByLabelText('过滤服务地址与端口')).toBeNull()
    await userEvent.click(screen.getByLabelText('启用外部 DNS 过滤'))
    await userEvent.type(screen.getByLabelText('过滤服务地址与端口'), '127.0.0.1:5353')
    await userEvent.type(screen.getByLabelText('跳过过滤的域名后缀'), 'internal.example, second.example')
    await userEvent.click(screen.getByLabelText('启用外部 DNS 过滤'))
    expect(screen.queryByLabelText('过滤服务地址与端口')).toBeNull()
    await userEvent.click(screen.getByLabelText('启用外部 DNS 过滤'))
    expect((screen.getByLabelText('过滤服务地址与端口') as HTMLInputElement).value).toBe('127.0.0.1:5353')
    expect((screen.getByLabelText('跳过过滤的域名后缀') as HTMLInputElement).value).toBe('internal.example, second.example')
  })
})
