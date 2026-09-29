import codenames from '../../packaging/release-codenames.json'

export function releaseCodename(tag: string): string {
  const series = /^v?(\d+\.\d+)\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.exec(tag)?.[1]
  return series ? (codenames as Record<string, string>)[series] ?? '' : ''
}

export function releaseDisplayVersion(tag: string): string {
  const codename = releaseCodename(tag)
  return codename ? `${tag} · ${codename}` : tag
}
