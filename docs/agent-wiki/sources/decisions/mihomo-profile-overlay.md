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
- `profile.store-selected: true`
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

When mihomo is running, `omg policies --config <path>` lists policy groups from
the mihomo external-controller API, `omg policy-select --config <path> --group
<name> --policy <name>` switches a group selection, and `omg connections
--config <path>` inspects current mihomo connections. This control surface is
API/config behavior: `policy-select` first reads live groups and rejects unknown
group or policy names before sending the selection change. Cover this layer with
unit or fixture-level tests first; real-device tests are only needed when
proving the gateway path or whole-LAN client behavior.

`make policy-control-test` is the non-root live mihomo gate for this layer. It
starts mihomo without dnsmasq, pf, or TUN, switches the imported fixture's
`Proxy` group to `DIRECT`, restarts mihomo in the same runtime directory, and
requires the `DIRECT` selection to be restored through `profile.store-selected`.

Use `make lab-test-tun-imported-profile` for a reproducible TUN lab gate that
starts OpenSurge with an imported profile fixture. The fixture keeps
`MATCH,DIRECT`, so it proves imported overlay compatibility with TUN startup and
transparent routing, not external proxy egress.

Do not treat imported profiles as permission to re-enable `redir-port` or PF TCP
redirection. macOS transparent proxying remains TUN-first.
