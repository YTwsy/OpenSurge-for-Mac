import { describe, expect, it } from 'vitest'
import { releaseDisplayVersion } from './release'

describe('release display version', () => {
  it.each([
    ['v0.3.0-rc.1', 'v0.3.0-rc.1 · Verdilion'],
    ['0.3.0', '0.3.0 · Verdilion'],
    ['v0.3.0-rc.2+build.42', 'v0.3.0-rc.2+build.42 · Verdilion'],
    ['v0.2.4-next', 'v0.2.4-next · Wind Rose'],
    ['v0.2.4', 'v0.2.4 · Wind Rose'],
    ['v0.4.0', 'v0.4.0'],
    ['unknown', 'unknown'],
  ])('labels %s without losing its release suffix', (tag, label) => {
    expect(releaseDisplayVersion(tag)).toBe(label)
  })
})
