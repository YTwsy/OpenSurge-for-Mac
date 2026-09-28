# Desktop host

The `Next` branch develops a Wails v3 host in `apps/desktop/`. Its Go module is
independent of the root module so native UI dependencies do not enter backend CI.
On `Next`, production GUI targets and the PKG ship the Wails host. Swift sources
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

## Communication

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

Dock presence follows the main window, using AppKit's regular/accessory activation
policies. Closing or hiding the main UI removes its Dock entry; reopening from the
menu bar or a second launch restores it. A covered window stays in Dock, and a
minimised window retains its normal Dock restore path. Tray visibility does not
control Dock presence. Do not use Wails WindowHide alone for this decision: on macOS
that event also represents occlusion by another window.

Explicit launch/reopen requests use one native presentation path: restore Dock
presence, unhide/activate the application and order the main window to the front.
Ignore intermediate visibility notifications during that activation-policy change,
and finish ordering on the next AppKit turn. Background status refreshes never
activate the application. Exit/uninstall warnings attach as sheets to the foreground
main window; cancelling restores its previous hidden/minimised state.

The main window starts hidden until its native light-gradient/icon placeholder is
installed. An HTML bootstrap uses the same light palette while loading React. The
native cover is removed only after the renderer reports its first committed frame
through a main-frame, internal-origin WebKit message; API readiness is independent.
The cover also protects WebView process recovery. The isolated smoke host accepts
`--smoke-startup-delay 3s` with `--smoke-actions` to inspect slow asset loading without
adding a minimum splash duration to normal launches.

Main UI selects share the themed select-only combobox in `Select.tsx`, including
portal positioning, keyboard/typeahead navigation, cancellation, disabled options
and explicit WebKit pointer focus. Scrollbars and text selection share light/dark
control colours. The page canvas extends its background through the transparent
scrollbar track; thumbs use a low-opacity neutral green, including on hover.
Language saves fence background preference refreshes until the
mutation completes; async catalog preparation must recheck the request generation
before updating the rendered language.

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

The menu-bar window uses a separate frontend entry and stylesheet; it does not load
the main App or start its subscriptions. A native monitor reads the existing
`/api/v1/menubar` DTO every 15 seconds in the background, every 2 seconds while the
popup is open, and backs off failures up to 60 seconds. The visible popup reads the
monitor's private snapshot. Failed reads clear actionable status, including quit
eligibility, instead of displaying an old healthy result. Network-recovery attention
takes precedence over stopped/running display; an unchanged prepared recovery card
does not imply an interrupted network.

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

The native login-item manager uses `SMAppService.mainAppService` and serialises
status reads and explicit changes. The returned OS status is authoritative; approval
required and failed registration must never be presented as successful enablement.
No automatic registration occurs. Custom discovery directories disable real login
management, while smoke mode supplies a fixture provider. Settings and update results
have independent monotonic sequences so a late poll cannot overwrite a newer result.

The native update checker contacts only the official GitHub latest-release API,
with an eight-second timeout and a 1 MiB response limit. It rejects redirects,
draft/prerelease entries and links that do not exactly match the repository/tag.
Semantic version comparison includes Next and release-candidate builds. Checks run
at launch and every 24 hours (15-minute retry after failure), deduplicate concurrent
requests and clear obsolete download links on failure. The host only opens the
validated page on explicit action; download/install remain user actions. Both builds
take their version from `OPENSURGE_RELEASE_TAG`, defaulting to `v0.2.4-next`.

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

For native acceptance, `scripts/desktop-smoke-service.py DIR` runs a harmless fixture.
Launch the preview executable with `--control-dir DIR`; scenario changes and service
restarts exercise sessions, discovery and UI state without touching the installed
gateway. Its observations record mutation payloads and SSE lifetimes, never credentials.
The additional `--smoke-actions` flag is accepted only with a custom directory and
records fixed lifecycle commands through the fixture instead of executing launchctl.

The [migration plan](../../../desktop-migration.md) tracks stage boundaries;
[GUI control-plane](gui-control-plane.md) documents the existing contracts to carry
forward, and [validation gates](validation-gates.md) define required evidence.

## Installer boundary

The PKG preserves `/Applications/OpenSurge.app` and `com.opensurge.menubar` but
ships `OpenSurgeDesktop`. Full bundle replacement removes the old executable.
Preinstall recognises both installed host generations, excludes previews/developer
paths, stops hosts before the user Control Service, then retains the existing
recovery CLI / sleep ownership / Helper order. TERM stops only the Wails host with
no confirmation; interactive quit retains its confirmation and safety gates.

The installer does not register or unregister login items. The OS-reported state
remains authoritative after upgrade. Real login continuity and authorization require
installed acceptance, not a bundle-ID or fixture assertion. The uninstaller retains
its fixed path and data modes, and waits for its caller to exit itself on success.
`tests/packaging/test_installer.py` exercises script ordering/retention in temporary
roots with mocked system commands. It cannot establish host-network cleanup.
