# Mihomo profile overlay contract

This page owns import composition, global overlays, prepared cores and gateway-owned fields. The raw source, saved desired configuration and running applied configuration have distinct lifetimes.

## Modes

`mihomo.profile_mode: "managed"` is the default. OpenSurge renders its minimal
DIRECT/smoke mihomo config.

`mihomo.profile_mode: "imported"` reads `mihomo.profile` and imports only these
top-level mihomo engine sections. Relative `mihomo.profile` paths are resolved
from the OpenSurge config file's directory. Relative `path:` entries inside
imported `proxy-providers` and `rule-providers` are resolved from the imported
mihomo profile's directory. When starting or validating mihomo for an imported
profile, OpenSurge passes `-d <profile-dir>` so mihomo SAFE_PATHS accepts those
provider files. HTTP and file Providers retain their existing path semantics:
relative paths are anchored to the profile directory, absolute and URL-shaped
paths stay unchanged, and missing paths remain absent. The loader does not
namespace or migrate downloaded caches. Imported sections are:

- `proxies`
- `proxy-providers`
- `proxy-groups`
- `rule-providers`
- `rules`

These sections are parsed and composed as YAML nodes rather than line-oriented
text. Both block and flow collections are supported, including compact
`rules: ['MATCH,DIRECT']`, quoted top-level keys, and inline provider mappings.
OpenSurge does not rewrite the imported source snapshot; only the generated
runtime mihomo config may normalize collection style or indentation.

## Global profile overlay and optional sources

The Web GUI's **Global Extension** is an independent, versioned draft in
`global-profile-overlay.yaml`. An imported mihomo YAML is an optional input,
not a prerequisite. When a source is selected, the overlay is reapplied to its
raw snapshot for compatibility checks and composition without mutating that
snapshot. When no source exists, OpenSurge renders its minimal managed profile
as the base, then applies the same overlay operations. This makes an overlay
containing added proxies, groups and rules a complete usable configuration on
its own. Its schema is intentionally operation-based:

- prepend rules, or append rules immediately before the imported terminal
  `MATCH`; an overlay may not add its own `MATCH`;
- add or explicitly replace named proxies, proxy providers, proxy groups, and
  rule providers;
- patch an existing proxy group by appending proxies/groups or proxy providers;
- merge or append only retained DNS resolver/filter fields.

This schema is broader than the ordinary user interface. The Sources page has
one complete-profile import surface: the existing local YAML chooser/dropzone.
The global overlay appears after the imported-source list and is collapsed by
default. Its ordinary editor exposes only high-priority rules fixed before the
subscription and proxy additions from share links (plus a small HTTP/SOCKS5
form). Share links add proxy objects; they are not another subscription or YAML
source. Tail rules, proxy/rule providers, group add/replace/patch, DNS fields,
and proxy replacement remain visible only through Expert Overlay YAML. Existing
documents keep all of those operations and the collapsed summary counts them,
so the UI simplification does not discard compatible stored data.

`add` fails on a name collision, `replace` fails when its target is absent, and
group/rule references added by the overlay must resolve after composition.
OpenSurge-owned namespaces and gateway-facing fields are rejected. In
particular, an overlay cannot add or replace the managed
`open-surge/tailscale` proxy; the generated Tailscale proxy and its access
rules remain owned by OpenSurge and are composed after the imported profile.
Saving this document first creates a draft. A running gateway continues to use
its applied core and never consumes that draft implicitly; applying its desired
source remains the explicit full reload transaction. While stopped, either
opening the Policies page or starting from the Web GUI composes the saved draft
onto the current desired imported source, or onto the managed base when no
source exists. The Policies path validates and starts the prepared policy core
without changing desired config or base recovery metadata. App start hands the
candidate to Gateway Manager, which stops any prepared core, resolves final
network parameters, renders and validates once, then commits desired before
network takeover. Preview and startup reuse the same composition entry point.
Visiting Policies is useful for preview, selection, and latency testing, but is
not a startup prerequisite. An overlay-only draft saved while running takes
effect on the next App start; this path adds no running hot-apply mechanism. A final preview that
includes the managed Tailscale proxy must
replace its `auth-key` value with `<redacted>`. The desired config records both
the raw/base digest and the enabled overlay digest so the GUI can distinguish
source drift from overlay drift; legacy imported configs without this metadata
remain readable.

Overlay `add` operations are naturally suitable for a source-free document.
`replace` and `proxy-groups.patch` still require their target to exist in the
chosen base; a draft written for a particular subscription can therefore be
valid as an overlay document but incompatible with the source-free managed
base. The prepared Policies request reports that incompatibility instead of
silently discarding the operation or inventing an imported source.

## Prepared policy workspace

The Policies page reads one final, engine-backed policy graph in both gateway
states. While the gateway is running it talks to the running mihomo and ignores
source and overlay drafts. While stopped, the privileged Helper composes the
selected raw source when one exists, the saved global overlay when enabled,
and OpenSurge-managed device/Tailscale sections, then starts a prepared mihomo
with only a random authenticated loopback controller. Business proxy ports,
DNS listeners, TUN, custom listeners/tunnels, DHCP, pf, forwarding and IPv6
packet ingress remain disabled. Resolver policy stays loaded so provider and
proxy hostnames behave like the future gateway.

The prepared core uses the same cache directory as the next real gateway core,
so `profile.store-selected` choices, Provider caches, and the Tailscale state
directory survive the handoff. Only the profile filename contains a digest; the
working directory does not change with the composition digest.
There is never a second concurrent owner of that cache or embedded tsnet
identity: a shared lifecycle lock serializes policy requests and gateway
transitions, gateway start stops the prepared core first, Control Service EOF
releases its Helper lease, and Helper startup reconciles an engine left by a
previous Helper crash. The private controller address and secret are stored
only in root-owned prepared state and are never returned to the browser.

The authenticated Web GUI start endpoint always supplies a server-authoritative
workspace snapshot to the privileged Helper. The browser cannot supply raw
source, overlay, credential, binary, controller, or runtime paths. Composition,
final network resolution, one real `mihomo -t` validation, desired-config commit,
and `StartCandidateLocked` takeover are
one privileged transaction under the cross-process lifecycle lock, so a
concurrent CLI cannot slip in and start the previous graph between persistence
and handoff. A raw `sudo omg start` intentionally starts only the already
persisted root-owned desired config; it does not discover or activate a draft
from a user's Control Store.

No individual optional input is special-cased as required. An absent source,
absent/disabled overlay, or disabled Tailscale config is normalized into the
same composition path. In particular, the source-free overlay path must keep
working after repeated page polls, selection changes and Control/Helper
handoffs; the immutable `workspace-profile-<digest>.yaml` and root-only base
metadata prevent overlay operations from being applied twice. Only explicit
commit updates the original base and managed upstream recovery record; a failed
commit restores that record and desired config. Missing raw source is tolerated
only for an unchanged existing composition. A requested recomposition without
its original base fails explicitly. Library refreshes and unselected imports do
not replace the selected source version. Base metadata
lives below the runtime control directory, outside mihomo's writable `-d`
directory, so a valid Provider cache path cannot overwrite it. Every request
also records whether the gateway was running or stopped when its snapshot was
captured; the Helper rechecks that state under the lifecycle lock and rejects a
snapshot that crossed a concurrent start/stop transition.

## Imported DNS policy

The profile's top-level `dns` section is merged at field level. OpenSurge
rejects the imported values for `enable`, `listen`, `ipv6`, `enhanced-mode`,
and `fake-ip-range`, but preserves the remaining resolver and filtering fields.
This includes `default-nameserver`, `nameserver`, `nameserver-policy`,
`proxy-server-nameserver`, `direct-nameserver`, `fake-ip-filter`, and fallback
settings. Proxy server hostnames can depend on these fields, so discarding them
can leave mihomo healthy while every imported node resolves to an unreachable
address.

## Gateway-owned fields

Imported profiles must not become raw pass-through configs. OpenSurge still
renders and owns:

- LAN binding through `mixed-port`, `allow-lan`, and `bind-address`;
- `external-controller`, so `status`, `doctor`, and policy-group CLI commands
  have the expected API target;
- `profile.store-selected: true`, so mihomo can persist selected policy-group
  members across restarts;
- configurable `profile.store-fake-ip`, defaulting to `true` through
  `mihomo.store_fake_ip`, so existing fake-IP mappings survive apply/restart;
- DNS enablement, listener, AAAA policy, fake-ip mode/range, and TUN DNS
  hijack; imported resolver/filter policy is merged around these owned fields;
- TUN device, stack, routing flags, and LAN/private route exclusions.

This prevents a desktop mihomo profile from disabling LAN access, turning off
DNS/TUN, changing controller ports, or reintroducing unsupported transparent
proxy paths.

## Device and local-Mac composition

When `device_policy.file` is configured, its device overrides are inserted
before imported global rules. A `dedicated` device default is also inserted
before global rules after source-scoped local/private `DIRECT` guards; an
`inherit_global` device has no default selector. Only a document with no
explicit mode retains the legacy default after global rules and before terminal
`MATCH`. An imported profile with rules after `MATCH` is rejected. Device identity and the selector data path use
`make lab-test-tun-device-policy`; template and rule-provider compilation use
`make test`.

The local-Mac Rule/Global/Direct overlay is inserted before device overrides
without changing the imported top-level rule mode. It owns hidden
`open-surge/mac-*` selectors and matches both inbound type and local source
identity, so downstream device sources continue into their existing path.
Imported proxy/group targets may not occupy that reserved namespace. See the
[local-routing contract](local-mac-routing-modes.md) and the real isolation gate
linked below.

## Configuration and control inspection

Use `make test` for code-level coverage. `doctor` includes a `mihomo config render` check so an
unreadable imported profile or missing `rules` section fails before gateway
startup. Use `go run ./cmd/omg render-mihomo --config <path>` to inspect the
final overlaid mihomo config without root or service startup. Use
`go run ./cmd/omg validate-mihomo --config <path>` for a stronger non-root check
that renders the final config and runs mihomo's own `-t` validation with the same
`-d` directory OpenSurge uses at startup. This command requires `mihomo.binary`
in the OpenSurge config to point to an installed mihomo binary.

When mihomo is running, use `omg policies --config <path>` to list policy groups,
`omg policy-select --config <path> --group <name> --policy <name>` to switch the
selected member, and `omg connections --config <path>` to inspect active mihomo
connections. Use `omg providers --config <path>` to inspect proxy/rule
providers, and `omg provider-update --config <path> --provider <name>` to
refresh one proxy provider. `policy-select` first reads live groups and rejects
unknown group or policy names before sending the selection change. These are
control-plane checks. `make policy-control-test` also proves source-scoped
local/private guards keep a local mixed-port target `DIRECT`, exercises the
dedicated local-routing selectors, and verifies both file and locally served
HTTP proxy-provider refresh. It still does not require real-device validation
unless the change also touches gateway, DNS, TUN, or traffic-capture behavior.

The runtime config path remains owned by OpenSurge. Importing a profile does not reopen the retired redir/PF TCP redirect path.

## Validation entry

Use the [policy control gate](../validation/test-gates.md#策略控制面门槛) for live API and cache checks, and [TUN/profile/workspace gates](../validation/test-gates.md#透明代理门槛) for capture and candidate-start evidence.
