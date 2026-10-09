# Desktop host contract

This page owns native windows, authentication, observation, capabilities and service lifecycle. API semantics belong to [Control API](../control-api.md), while gateway processes belong to [gateway lifecycle](gateway-lifecycle.md).

OpenSurge v0.3 ships a Wails v3 host in `apps/desktop/`. Its Go module is
independent of the root module so native UI dependencies do not enter backend CI.
Production GUI targets and the PKG ship the Wails host. Swift sources
remain for historical maintenance; their explicit build writes to `bin/legacy/`.
This does not establish PackageKit or macOS 13/Intel runtime acceptance.

## Ownership

- Wails owns native windows, the tray icon/popup, single-instance and reopen events,
  login registration, local service discovery, authentication, and desktop exit.
- The existing React/TypeScript code owns the main control panel. A dedicated React
  popup at `/desktop-tray` replaces the SwiftUI view and shares
  frontend language, status labels and theme with the main window.
- The independently managed Go Control Service owns API orchestration, state,
  operations, configuration, and recovery. The root Helper owns privileged actions.
- Closing a window or losing a WebView must not stop the gateway. Full exit must
  obey the existing stopped-gateway/recovery contract and preserve the loaded Helper.

## Native transport and authentication

The host embeds the existing React build through `internal/webui.FS()` and serves
it at `wails://localhost` over Wails' in-process asset transport. No extra TCP
listener is opened. The independent desktop module imports only this static-asset
package from the root module; root service builds do not depend on Wails.

The native HTTP relay forwards `/api/v1/` to the existing Control Service. It obtains
a one-time bootstrap grant and exchanges it for an HttpOnly session in native memory.
Neither the bearer token nor the service cookie enters JavaScript. A random per-process
capability injected into the bundled HTML authorizes private asset-transport requests;
SSE carries it only on the events route. Native credentials replace renderer credentials
before forwarding. Session/bootstrap endpoints, foreign origins and redirects are denied.
Desktop bindings remain limited to explicit capabilities: never expose general shell
execution or bind gateway managers into the GUI process.

The preview's native client accepts only explicit-port HTTP loopback discovery,
dials IPv4 loopback directly, and refuses redirects. It validates the returned
bootstrap URL against the discovered service origin before exchanging it. Discovery
and credentials are reread for every API request; port, process or token changes renew
the native session. Authentication failure retries GET/HEAD once. Mutations are never
replayed, including bodyless POSTs with an Idempotency-Key. Service loss produces a
reconnect error, not browser-session expiry, so the React tree and unsaved drafts stay
mounted. SSE flushes each chunk and propagates WebView cancellation upstream. The
browser host retains its existing bootstrap/cookie and session-expiry behavior.

Tray status must remain available without mounting the full main React application.
Hidden-window work must be reduced explicitly rather than relying on browser-tab
throttling. Reconnect must recover authentication and state without replaying an
unacknowledged privileged mutation.

Credentials are read from the current user Control Store `control-token` file (mode `0600`), without a Keychain fallback. Bootstrap `expires_at` accepts RFC3339 with or without fractional seconds; bootstrap URLs and credentials must never enter errors or durable logs.

## Main window and presentation

The main window initially requests 1440 × 900 points and fits the available screen.
It preserves user-resized frames (minimum 1080 × 640) rather than forcing the default
on every launch. Its transparent inset title bar keeps native traffic lights, hides
the duplicate title, and reserves a drag region above the React controls. Desktop-only
spacing must not affect browser or tray layouts. The window hides on close and reopens through Dock or a
second launch without replacing its React tree. Single-instance scope includes the
absolute discovery directory so fixture acceptance cannot redirect an installed host.
Native View-menu navigation preserves React's existing dirty-device guard; refresh
requests state without reloading. The host loads the pinned Wails runtime explicitly
so native-to-renderer events are delivered through the supported bridge.

Dock presence follows the main and Settings windows, using AppKit's regular/accessory
activation policies. Closing the last visible desktop window removes its Dock entry;
reopening from the menu bar or a second launch restores it. Closing the main window
while Settings is visible must retain Dock presence. A covered window stays in Dock, and a
minimised window retains its normal Dock restore path. Tray visibility does not
control Dock presence. Do not use Wails WindowHide alone for this decision: on macOS
that event also represents occlusion by another window.

Explicit launch/reopen requests use one native presentation path: restore Dock
presence, unhide/activate the application and order the main window to the front.
The macOS reopen event is intercepted by a cancellable application hook. Wails
beta.26 otherwise shows every hidden window when none are visible; an ordinary
listener races that default and can reveal Settings and briefly flash the tray.
Cancel before those listeners run, then present only the main window. Reopen must
leave closed Settings hidden and must not close Settings that are already visible.
Ignore intermediate visibility notifications during that activation-policy change,
and finish ordering on the next AppKit turn. Background status refreshes never
activate the application. Exit/uninstall warnings attach as sheets to the focused
Settings window when present, otherwise to the foreground main window; cancelling
restores the main window's previous hidden/minimised state when it was summoned.

The main window starts hidden until its native light-gradient/icon placeholder is
installed. An HTML bootstrap uses the same light palette while loading React. The
native cover is removed only after the renderer reports its first committed frame
through a main-frame, internal-origin WebKit message; API readiness is independent.
The cover also protects WebView process recovery. The isolated smoke host accepts
`--smoke-startup-delay 3s` with `--smoke-actions` to inspect slow asset loading without
adding a minimum splash duration to normal launches.

## Settings and shared preferences

App-wide preferences live in one reusable `/desktop-settings` window, opened by
**OpenSurge → Settings…**, `⌘,`, or the desktop sidebar. It hides on close, has no
minimise/maximise/fullscreen controls, and does not replace the main window or its
drafts. In both desktop and browser views, the flat sidebar gateway-status row
with a small settings gear expands the sleep-prevention, language, and appearance controls above it. It starts
collapsed, keeps the status/version visible, and removes collapsed controls from
keyboard navigation. Desktop views also retain the Settings shortcut in that area.
Language uses the existing Control API preference; Settings publishes changes only
after a successful save. Shared hooks fence late reads and synchronise windows through
the same-origin storage event. Appearance stores Light, Dark, or System locally; System
clears native appearance overrides and follows subsequent macOS changes. Login at
startup and stable-release checks reuse the tray's sequence-ordered native managers.
The tray's settings remain flat. Opening Settings neither starts a gateway nor
registers a login item; only an explicit switch change requests the latter.
Settings also exposes the session-only sleep control, UI-only/full exit, and uninstall
through the existing native capabilities. Status and action eligibility are read from
the same native monitor as the tray; failed reads disable full exit, uninstall, and
sleep changes. A late poll cannot overwrite an acknowledged sleep change. Quit and
uninstall keep their native confirmations and revalidation. Settings uses the same
transparent inset title bar and reserved drag area as the main window, allowing the
page background to extend behind the native traffic lights.

## Native capabilities and preview identity

A small AppKit/WKWebView delegate adapter supplies native JavaScript confirmation
sheets, external-navigation confinement, and actual NSWindow visibility. It forwards
the framework's other delegate methods, including file selection and renderer recovery.
Hidden, minimised or fully occluded windows close SSE and pause recurring reads;
becoming visible resumes them. Deliberate HTTP(S) links open the system browser.
The private `/desktop/v1/` routes share the renderer capability check and expose
bounded native actions: external links, clipboard, language, recovery-card saving,
window navigation and the status/lifecycle/settings capabilities described below.
Browser file/link behavior remains supported.

The popup's **Open in browser** action is a separate, argument-free native
capability. It rereads Control Service discovery/credentials, obtains a fresh
Dashboard bootstrap grant and passes the validated loopback URL directly to the
system browser. It does not consume the grant through the desktop relay or return
the URL/cookie/credential to JavaScript. Failure keeps the action available for an
explicit retry; it neither starts a gateway nor replays a previous request.

The preview bundle uses a distinct identifier and build output. The production build
uses `com.opensurge.menubar` at `bin/OpenSurge.app` with `OpenSurgeDesktop` as its
executable. Version/build/tag are stamped and verified together. Single-instance
scope includes both the bundle identity and discovery directory. Development preview
builds must not register login items or change installed network services on launch.

## Menu-bar observation and presentation

The menu-bar window uses a separate frontend entry and stylesheet; it does not load
the main App or start its subscriptions. A native monitor reads the existing
`/api/v1/menubar` DTO every 15 seconds in the background, every 2 seconds while the
popup is open, every second during a busy operation, and backs off failures up to
60 seconds. Unknown/degraded observations use a two-second refresh. The visible popup reads the
monitor's private snapshot. Failed reads clear actionable status, including quit
eligibility, instead of displaying an old healthy result. The Control API's shared
`presentation` contract distinguishes live transitions, pending configuration,
cached diagnostic warnings, confirmed failures and unknown observations. Main-window
operation start/completion refreshes the native snapshot too. Network-recovery
attention remains explicit once a transition finishes; an unchanged prepared recovery
card does not imply an interrupted network. Busy/unknown snapshots cannot authorize
full exit or uninstall, even when raw services are temporarily stopped during reload.

The popup presents a gateway overview: current rates and a short chart, this Mac's
routing summary, this Mac plus two active downstream devices, expandable network
state, and the session-only sleep control. Utilities and exit actions live in the
header's More actions panel, with settings and updates shown directly and matching
switch styles. Routing and device rows first expand inline summaries; their explicit
desktop links navigate to settings or the existing main connection view with an
encoded owner, preserving unsaved-device navigation guards. Native
navigation accepts only explicit page names and an optional bounded connection owner.

While the gateway is running/degraded, a native observation monitor reads the
existing `/api/v1/device-traffic` endpoint every two seconds, independently of both
WebViews. It retains at most 30 samples / 60 seconds in memory and backs off failed
reads up to 30 seconds. Stopped or unknown gateways clear the cache and cancel
pending reads; late results cannot overwrite a restarted gateway. Rate calculation
stays in the Control Service. Rates cover observed active
mihomo sessions, not interface bandwidth or daily usage. Active downstream counts
exclude the Mac and inactive inventory. Downstream ranking is held for ten seconds
to reduce moving click targets. The visible popup reads the private native cache
every second, and reads `/api/v1/local-routing` separately every two seconds so a
slow routing query cannot stall rates. Hidden popups pause renderer reads, retaining
native history for immediate display on reopen. Failed reads clear observations;
sleep/service gaps over the Control API's 15-second validity window require a new
baseline. The first sample is not shown as a zero rate.

Stopped gateways collapse the rate figures and chart into a compact start hint.
Its link opens the existing Network page, without issuing a start command. Unknown
status and initial sampling also use compact text rather than an empty chart; only
valid sample history expands the chart, including when the measured rates are zero.
The intrinsic-height transition shares the popup's native sizing bridge and respects
Reduced Motion, so its top remains anchored while the lower edge moves.

The background is AppKit `NSVisualEffectMaterialPopover` with behind-window
blending beneath a transparent WKWebView, enabled by Wails' `MacBackdropTranslucent`
and the build tag `production,private_mac_apis`. The private tag is required by the
pinned Wails version's WebKit transparency implementation; CSS alone cannot expose
the material. No Liquid Glass is used. The popup appearance tracks its React theme;
AppKit supplies label, separator, control, accent and chart colours. Use system fonts
and visible keyboard focus; respect increased contrast and reduced transparency.
These choices follow Apple's [materials](https://developer.apple.com/design/human-interface-guidelines/materials),
[buttons](https://developer.apple.com/design/human-interface-guidelines/buttons) and
[typography](https://developer.apple.com/design/human-interface-guidelines/typography)
guidance rather than sampling screenshot colours.

The popup's active-device **View all** action targets `dashboard#active-devices`;
the native navigation capability only accepts this section on Dashboard. React
scrolls and focuses the section after rendering, even for repeated requests while
Dashboard is already open. Individual device links retain Connections owner filters.

The popup fits its content instead of scrolling inside a fixed-height window.
Network details use a 220 ms disclosure transition and the historical Swift Grid's
leading columns, 14-point column gap, 7-point row gap and caption text. A tray-only
WKScriptMessageHandler accepts finite, bounded height measurements from the internal
main frame; an injected ResizeObserver follows intrinsic content height, so the
native window's lower edge follows the disclosure while its top stays anchored.
The height is capped to the current display's visible area; only an unusually short
display or long warning needs internal scrolling, without a visible scrollbar.
Reduced Motion disables the transition, following Apple's
[motion guidance](https://developer.apple.com/design/human-interface-guidelines/motion).

The menu-bar template image remains the existing brand asset at **18 × 18 points**,
matching the Swift host. Wails beta.26 otherwise scales it to the status bar's height.
Status-item windows are absent from `NSApp.windows`, so searching that list cannot
resize the image. Before tray creation, the native adapter installs a process-local
`NSStatusBarButton.setImage:` adaptation, gated to Wails' `StatusItemController` target.
It restores the 18-point template size on every image assignment, without raster
resampling or changing the superclass `NSButton` setter. Recheck this adaptation when
upgrading Wails; smoke mode logs the actual assigned image dimensions and opacity.
Match the Swift host's state appearance: brand image at 0.75 opacity while connecting,
0.55 when stopped, 0.35 when unreachable, and 1.0 while running. Degraded/recovery
use the original AppKit warning symbols at full opacity. Do not append text badges
that widen the status item. Keep the brand source separate from rendered warning images.
Lifecycle transitions use the circular-arrows symbol, rollback uses a return arrow,
and unknown observations use a question mark. Pending configuration and cached Doctor
warnings retain the running brand icon and appear as secondary text in the popup.
The adapter also applies the localised tooltip/accessibility label; the pinned Wails
macOS `SetTooltip` implementation is a no-op.

On opening, the popup focuses its non-tabbable main panel instead of letting WebKit
automatically highlight the first **More actions** control. Tab enters the controls
normally with visible keyboard focus; native Escape dismissal still hides the popup. Pointer
focus does not acquire the browser's fallback focus outline.

Both desktop windows route explicit sleep-prevention changes through the existing
Control API using one native serialisation boundary shared with menu-bar reads.
An older read cannot overwrite an acknowledged toggle. Browser requests continue
to use the Control API directly. Language comes from `ui_preferences`; the popup
has no independent language preference. Theme follows the same origin's localStorage
change event. The popup opens from the tray or `⌘⇧M`, dismisses on Escape or focus
loss, and opens the existing main window for panel/recovery/diagnostic actions.

## Service lifecycle, login, updates and uninstall

The menu-bar panel provides status and navigation. Gateway start/stop, provider
refresh and policy selection remain in the main control surface.

Service lifecycle uses a closed native capability. Explicit reconnect checks the
fixed user LaunchAgent, bootstraps its installed plist if needed, and kickstarts it
without `-k`. The production identity at `/Applications/OpenSurge.app` wakes that
job once on launch when using the default discovery directory; preview launch does
not. Failure leaves explicit reconnect available. Custom discovery directories cannot
operate launchd. A serial coordinator prevents reconnect from resurrecting a service
during exit. UI-only exit (including Cmd-Q and Dock Quit) confirms that the gateway
and Control Service continue. Full exit requires the existing `can_quit` contract
plus explicit stopped DHCP/mihomo and unloaded PF states, reads again after native
confirmation, and boots out only the user Control Service. Unknown/offline status
fails closed. The root Helper stays loaded. Pending exit blocks new renderer API
mutations; cancellation returns to the popup without replacing the main React tree.
Pre-existing host IPv4 forwarding alone does not block full exit and is not changed
by quitting the Control Service.

The native login-item manager uses `SMAppService.mainAppService` and serialises
status reads and explicit changes. The returned OS status is authoritative; approval
required and failed registration must never be presented as successful enablement.
No automatic registration occurs. `SMAppServiceStatusNotFound` is reported as
`not_found`, not `unavailable`: it can occur for a new ad-hoc-signed bundle or after
an update changes its code signature. The installed App keeps the switch available
so an explicit enable can register the current bundle. Only the subsequent OS status
can show enabled or approval; native error domain, code and description survive a
failed change. Settings and the tray share this feedback. Missing registration must
not silently bypass the uninstaller's login-cleanup guard.
Only the production bundle at `/Applications/OpenSurge.app` with the default
discovery directory can manage real login items; smoke mode supplies a fixture
provider. Settings and update results
have independent monotonic sequences so a late poll cannot overwrite a newer result.

The native update checker contacts only the official GitHub latest-release API,
with an eight-second timeout and a 1 MiB response limit. It rejects redirects,
draft/prerelease entries and links that do not exactly match the repository/tag.
Semantic version comparison includes Next and release-candidate builds. Checks run
at launch and every 24 hours (15-minute retry after failure), deduplicate concurrent
requests and clear obsolete download links on failure. The host only opens the
validated page on explicit action; download/install remain user actions. Both builds
take their version from `OPENSURGE_RELEASE_TAG`, defaulting to `v0.3.0`.

Uninstall is a closed native capability with two fixed modes. Production execution
requires the installed App identity/path and a root-owned, non-writable script and
parent chain. The preview cannot uninstall the production App. The host checks fresh
stopped-service evidence before and after its native three-choice confirmation. The
existing privileged script rechecks gateway state after administrator authorisation
and remains the only cleanup implementation. Uninstall can preserve data, including
credentials and recovery records, or remove it; pending manual network recovery and
existing IPv4 forwarding are explicitly described and not silently changed.

The host unregisters its own login item before uninstall, restores it on cancellation
or failure, and reports restoration failures separately. Renderer writes, reconnect
and quit are blocked while this action is pending. A WebView disconnect cannot cancel
an already authorised cleanup halfway through. Production uninstall acceptance belongs
to the installer cutover gate; fixture success establishes only the host interaction.

## Icons

原生应用图标由 `apps/desktop/Resources/OpenSurgeAppIcon.png` 经
`scripts/build-desktop-app.sh` 等比生成各档 `.icns` 资源。1024 × 1024 源图已包含
透明留白：白色底板主体宽约 824 px、每侧留白约 100 px；构建时不要再次补同样的边距。
此比例用于传统 `.icns` 的视觉对齐，不是所有 macOS 图标格式的通用尺寸契约。
未来采用 Icon Composer 时应按其模板重新校准并验证系统实际渲染。菜单栏状态项使用
独立的 `OpenSurgeMenuBarIcon.png`，显示尺寸为 18 × 18 pt，不跟随应用图标的留白调整。

## Adjacent contracts

Installer ownership and TERM handling: [distribution](../distribution.md). Release metadata: [releasing](../releasing.md). Native scenarios and fixture limitations: [desktop smoke](../validation/desktop-smoke.md). Platform acceptance: [installer matrix](../validation/evidence-map.md#桌面与安装). Migration rationale: [v0.3 history](../../tasks/finished_archived/desktop-migration-v0.3.md).
