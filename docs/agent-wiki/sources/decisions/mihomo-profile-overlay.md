---
title: Mihomo profile import uses OpenSurge gateway overlay
kind: decision
status: seed
---

# mihomo profile import uses OpenSurge gateway overlay

OpenSurge for Mac should support importing existing mihomo profiles, but imported
profiles are not raw pass-through gateway configs.

In `mihomo.profile_mode: "imported"`, OpenSurge imports only mihomo engine
sections:

- `proxies`
- `proxy-providers`
- `proxy-groups`
- `rule-providers`
- `rules`

Relative `mihomo.profile` paths are resolved from the OpenSurge config file's
directory. Relative `path:` entries inside imported `proxy-providers` and
`rule-providers` are resolved from the imported mihomo profile's directory.
When starting or validating mihomo for an imported profile, OpenSurge passes
`-d <profile-dir>` so mihomo SAFE_PATHS accepts those provider files.

OpenSurge continues to render and own gateway-critical fields, including:

- `mixed-port`
- `allow-lan`
- `bind-address`
- `external-controller`
- DNS listener and fake-ip settings
- TUN settings and LAN/private route exclusions
- runtime config path

This preserves the user's existing mihomo proxy/rule investment without letting
a desktop-style mihomo profile break the OpenSurge LAN gateway contract.

`doctor` includes a `mihomo config render` check for imported profiles. Use
`go run ./cmd/omg render-mihomo --config <path>` to preview the final generated
mihomo config before running root-required gateway startup. Use
`go run ./cmd/omg validate-mihomo --config <path>` to run mihomo's own `-t`
validation with the same `-d` directory OpenSurge uses at startup. This command
requires `mihomo.binary` to point to an installed mihomo binary.

Use `make lab-test-tun-imported-profile` for a reproducible TUN lab gate that
starts OpenSurge with an imported profile fixture. The fixture keeps
`MATCH,DIRECT`, so it proves imported overlay compatibility with TUN startup and
transparent routing, not external proxy egress.

Do not treat imported profiles as permission to re-enable `redir-port` or PF TCP
redirection. macOS transparent proxying remains TUN-first.
