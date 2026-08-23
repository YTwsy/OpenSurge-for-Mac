import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const sourceRoot = new URL('../src/', import.meta.url)
const sourceRootPath = fileURLToPath(sourceRoot)
const catalog = [
  readFileSync(new URL('../src/i18n.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../src/i18n.en.ts', import.meta.url), 'utf8'),
].join('\n')
const translated = new Set([...catalog.matchAll(/^\s*'([^']+)':/gm)].map(match => match[1]))
const missing = new Map()

for (const relativePath of readdirSync(sourceRoot, { recursive: true })) {
  if (!/\.(ts|tsx)$/.test(relativePath) || relativePath.includes('.test.') || relativePath.startsWith('i18n.')) continue
  const lines = readFileSync(join(sourceRootPath, relativePath), 'utf8').split('\n')
  lines.forEach((line, index) => {
    for (const match of line.matchAll(/(['"])((?:\\.|(?!\1).)*)\1/g)) {
      const source = match[2].replaceAll("\\'", "'").replaceAll('\\"', '"')
      if (!/[\u3400-\u9fff]/.test(source) || translated.has(source)) continue
      const locations = missing.get(source) ?? []
      locations.push(`${relativePath}:${index + 1}`)
      missing.set(source, locations)
    }
  })
}

if (missing.size) {
  console.error('English catalog is missing these Chinese source strings:')
  for (const [source, locations] of missing) console.error(`- ${JSON.stringify(source)} (${locations.join(', ')})`)
  process.exitCode = 1
} else {
  console.log(`i18n catalog covers all Chinese source strings (${translated.size} English messages)`)
}
