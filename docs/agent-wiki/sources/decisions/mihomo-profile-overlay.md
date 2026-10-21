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

- `hosts`
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

## File-based custom DNS

Sources exposes one optional **Custom DNS** draft as a sibling of the Global
Extension, with an independent enable flag. Network Settings shows a read-only
status summary and links to `/sources#custom-dns`, which expands and focuses the
editor. The control service stores `custom-dns.json` (schema version,
enabled flag and portable YAML); absence means disabled. Saving uses an atomic
Store `If-Match` comparison and never mutates the desired gateway config or a
running process. Import and preview are also non-applying operations.

Two UTF-8 formats are accepted, each limited to 2 MiB; BOM and LF/CRLF work:

- **hosts text**: literal IPv4/IPv6 followed by one or more names; `#` comments.
  Names are lowercased ASCII/punycode with a trailing root dot removed. Repeated
  names accumulate unique IPs in order. No wildcards, aliases, directives or URLs.
  Import replaces the draft's entire `hosts` section and preserves `dns`.
- **Mihomo YAML**: one mapping containing only `hosts` and/or `dns`, with native
  field values (no overlay `merge`/`append` wrapper). Each provided section
  replaces that entire draft section; omitted sections remain. Hosts supports IPs,
  IP lists, domain aliases and whole-label domain wildcards. Duplicate normalized
  keys, null deletion, custom tags, cyclic aliases and excessive expansion fail.
  Acyclic YAML anchors are expanded before saving/export. Import is atomic.

`dns` uses the same top-level allowlist as `profileOverlayDNSFields`:
`cache-algorithm`, `default-nameserver`, `direct-nameserver`,
`direct-nameserver-follow-policy`, `fake-ip-filter`, `fake-ip-filter-mode`,
`fallback`, `fallback-filter`, `nameserver`, `nameserver-policy`, `prefer-h3`,
`proxy-server-nameserver`, `proxy-server-nameserver-policy`, `respect-rules`,
`use-hosts`, `use-system-hosts`. New domain selectors are literal/wildcard domains;
rule-mode filters accept DOMAIN, DOMAIN-SUFFIX, DOMAIN-KEYWORD and terminal MATCH,
with `real-ip`/`fake-ip` actions. Wildcard matching follows the pinned core
[domain trie](https://github.com/MetaCubeX/mihomo/blob/ac017cdd246ce8bd547653d927e7bf77d7ee73d5/component/trie/domain.go);
ambiguous equal-rank alias targets are checked conservatively. New geosite/RuleSet/provider/file dependencies,
full profiles, dnsmasq snippets and gateway-owned fields are rejected. Existing
subscription/overlay dependencies are retained. Engine validation still owns
resolver protocol/parameter semantics.

Composition order is raw source (or managed base), enabled global overlay,
enabled custom DNS, then gateway fields/system DNS coordination/reference binding.
The original snapshot is unchanged. Hosts and both nameserver-policy mappings
merge by normalized domain key; a custom value replaces the whole inherited IP
or resolver list. Other mappings merge recursively and ordinary arrays/scalars
replace when supplied. Missing fields and empty mappings do not delete inherited
entries. Removing a custom entry restores inheritance on the next composition.

Fake-IP filtering is mode dependent. The inherited default mode is `blacklist`.
With the same or omitted mode, blacklist/whitelist append unique custom entries;
rule mode prepends custom rules. A custom MATCH can shadow inherited rules and is
shown in preview. An explicit mode change must also supply a complete list; the
pair replaces the inherited mode/list. An empty list in the same mode adds nothing.
Whitelist matches mean Fake-IP, not real-IP exceptions. A draft containing custom
hosts forces `use-hosts: true`; explicit false in that draft is an application
conflict. Hosts precedence, wildcard specificity, alias-target filtering and
possible node/provider use remain Mihomo semantics. Preview shows known direct
node/provider host matches, direct-nameserver follow-policy state, before/after
values, lower-layer DNS and subsequent managed changes.

With hosts enabled, explicit IPv6 hosts mappings can answer AAAA even when
`dns.ipv6` is false. This is an intentional fixed-address override; that switch
controls ordinary upstream and Fake-IP AAAA answers, not removal of explicit
addresses. DNS answers do not establish Mac or downstream IPv6 reachability.
An IPv4-only fixed name should retain only IPv4 hosts mappings.

The authenticated preview endpoint sends its server-assembled draft snapshot to
the privileged Helper using the fixed `custom-dns-preview` action. The Helper
checks the desired-config revision and renders under the shared lifecycle lock,
using a private temporary directory below the managed runtime root. It reads the
existing root-only workspace base without changing its permissions, returns only
DNS/hosts sections and composition diagnostics, then removes temporary profiles.
It never starts/stops a prepared or gateway engine or commits configuration.
The browser cannot choose base, runtime or output paths. Control Service and
Helper must both include this action; an unavailable/older Helper fails explicitly.

The immutable candidate carries `x-opensurge-custom-dns` and the desired config
stores `mihomo.profile_custom_dns_digest`. This is control-plane metadata, stripped
from runtime/export output, and is not accepted as ownership metadata from raw
subscriptions. Workspace snapshots carry the normalized draft to the Helper.
Prepared views/start and source apply use the same composition order; running
policy views ignore drafts. Source-free and overlay-disabled composition work.
Repeated materialization starts from raw/root base metadata; when that information
is missing, changing an existing composition fails rather than stacking patches.

Application requires managed dnsmasq upstream, TUN auto-route and Mac system DNS
coordination. It disables dnsmasq's system hosts lookup with `no-hosts`, leaving
Mihomo `use-system-hosts` in charge. While DHCP is enabled, custom hosts and policy
keys may not intersect single-label names or the configured lease-domain namespace.
Alias reachability is checked through inherited hosts too; uncertain wildcard
intersections are conservatively rejected. Exact hosts take precedence over
wildcards; bounded alias traversal rejects cycles/excessive complexity. This
reserves possible future lease names without discarding DHCP hostnames or device
identity. DHCP settings changes are revalidated whenever a candidate is rendered.
These checks do not promise shared dynamic lease-name resolution.

A formatted draft may be saved despite application blockers. Activation/disable
uses existing full gateway start or source apply/reload; full reload restarts the
owned dnsmasq and its cache. Engine-only recovery and page navigation never load a
new DNS draft. Failed apply follows existing lifecycle rollback/state reporting.
Sources accounts for both overlay and custom DNS desired state when enabling
application of an otherwise current source. Disabling DNS still counts as pending
until applied. Saving refreshes source compatibility; successful application
refreshes source, overlay and DNS status. Applying uses only saved drafts and the
existing source confirmation/transaction. With no exact desired/applied source
row (including when only an older source version is current), the DNS editor links
to Network Settings for stop/start rather than implicitly selecting a new source.
Unsaved DNS edits survive status refreshes and guard navigation away from Sources.
Exports contain only the canonical custom YAML; desktop uses a native save dialog,
browser uses a file download. No original file paths or file watching are retained.

Validation requirements are maintained in [test gates](../validation/test-gates.md),
and current evidence scope in the [evidence map](../validation/evidence-map.md#文件式自定义-dns).
Environment-specific results, blockers and follow-up work remain in the linked tasks.

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
with a random authenticated loopback controller. When the final resolver graph
contains a supported self DNS reference, that same process also opens a private
loopback DNS port. Without that dependency DNS listening stays disabled.
Business proxy ports, TUN, custom listeners/tunnels, DHCP, pf, forwarding and
IPv6 packet ingress remain disabled. Resolver policy stays loaded so provider
and proxy hostnames behave like the future gateway.

The private DNS endpoint and probe identity are root-only prepared state, never
desired configuration or browser API fields. Both TCP and UDP are reserved
until immediately before process start; reservation alone does not prove a
successful bind. Readiness verifies that this process owns both sockets, then
queries both transports using a random exact `.invalid` TXT policy answered
inside the core (`rcode://success`). The probe does not depend on public DNS,
hosts enablement, Fake-IP or prior caches. At most three startup attempts are
allowed, with the previous process cleaned up before another attempt. Reusing
a prepared process checks DNS again without reallocating its port; a failed
reuse probe reports the failure and does not silently restart a live owner.

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

The top-level `hosts` mapping is retained alongside these DNS fields, including
IP values, IP lists, domain aliases, wildcard keys and supported YAML anchors.
It survives global overlay composition, final gateway rendering and prepared
policy rendering without changing `use-hosts` or the imported filtering policy.
Mihomo's candidate validation remains authoritative for engine-specific hosts
semantics. The source snapshot is not rewritten.

### DNS reference composition and binding

Self DNS compatibility is always part of rendering, independent of the global
overlay or a future custom-resolution editor. Before dropping owned DNS fields,
the importer captures the original `enable`/`listen`, source digest and resolver
field/value origins. Materialized profiles preserve them under the versioned
`x-opensurge-dns-source` extension, including the immediately overridden origin
when a final target changes. Raw source snapshots stay unchanged. This extension
is composition evidence, not authority to restore a subscription listener.

After overlay and managed DNS coordination, classify the final targets again.
Only confirmed self references in `proxy-server-nameserver` and
`proxy-server-nameserver-policy` are automatically rebound. Supported targets are
literal UDP/TCP loopback endpoints covered by an enabled original wildcard or
exact loopback listener of the same address family and port. IPv6 reference
forms bind to the same IPv4 loopback runtime service; this does not demonstrate
a downstream IPv6 packet/DNS path. Bare IPv4 endpoints and `udp://`/`tcp://`
forms are supported. Parameters, hostname aliases, scoped IPv6 and other URL
protocols are not rewritten as if they were equivalent.

| Field / condition | Self reference behavior |
| --- | --- |
| Node resolver and node resolver policy | Bind to `127.0.0.1:1053` in the formal config, or the current prepared loopback endpoint |
| `nameserver`, ordinary `nameserver-policy`, `default-nameserver`, `fallback` | Reject a recursive dependency before application |
| `direct-nameserver` | Self binding is outside the supported scope; reject explicitly |
| `respect-rules`, or an upstream proxy/interface fragment other than `#DIRECT`, alongside self DNS | Reject the unverified bootstrap dependency |
| Independent loopback / external resolver | Preserve the exact target and parameters; independent UDP/TCP loopback port 1053 conflicts with the managed service |
| Cross-family wildcard or `localhost` ambiguity | Report the field, source, target and original listener; require an explicit declaration or a supported literal form |

An exported source can declare the purpose of an exact reference without
enabling any optional editor:

```yaml
x-opensurge-dns-services:
  - field: 'dns.proxy-server-nameserver[0]'
    target: udp://127.0.0.1:7874
    service: independent
```

`service` is `self` or `independent`. A declaration belongs to that selected raw
source and exact field/target pair; it is retained with source metadata across
composition and no longer matches when the final target changes. `self` still
requires an enabled original DNS and a supported literal loopback transport;
it cannot waive unsupported field, protocol or bootstrap checks. Edit/export
and reimport the source rather than changing a managed snapshot in place.

The final configuration carries `x-opensurge-dns-bindings` diagnostics with the
original/final target, purpose, provenance and service classification. Preparation
checks those bindings against the actual fields before rebinding, then removes
the extension from its engine config. Preview exposes configuration inference,
not a claim that a live DNS query has succeeded. Unknown external recursion
cannot be ruled out by static address checks.

For a final configuration with self references, formal startup also requires
the started process to own both port 1053 listeners and answer UDP/TCP TXT
queries. A managed exact `.invalid` policy and `x-opensurge-dns-probe` identify
that check. The identity is deterministic for the final configuration, so
rendering does not cause needless prepared restarts. Preparation replaces the
formal policy with its private random identity. Neither policy enters the raw
source or desired configuration. A live controller alone cannot make a failed
DNS bind count as a successful startup; failure stops the process just started.

For legacy overlaid profiles with local resolver targets and missing listener
metadata, the stopped workspace retrieves the selected raw snapshot even when
the overlay digest is unchanged. Root-owned base metadata can also provide that
source. If neither exists, report the missing original metadata; do not infer
service identity from a port occupant or silently reuse a filtered profile.
Direct CLI rendering likewise diagnoses a legacy composed local resolver whose
origin cannot be recovered. An unchanged independent profile without such a
dependency remains readable.

Real-core prepared DNS tests cover aliases, filters, controlled upstreams,
node/Provider connectivity, both transports and bind-race cleanup. Formal port
1053 and host-network handoff still require the corresponding unoccupied-port
and Lab gates; see the [DNS task evidence](../../tasks/issue-93-dns-references-and-issue-82-overrides.md#本次开发与验证记录).

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
