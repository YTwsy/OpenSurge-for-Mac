# Desktop host

The `Next` branch develops a Wails v3 host in `apps/desktop/`. Its Go module is
independent of the root module so native UI dependencies do not enter backend CI.
The installed Swift menu-bar host remains the production entry until the desktop
installer cutover is complete.

## Ownership

- Wails owns native windows, the tray icon/popup, single-instance and reopen events,
  login registration, local service discovery, authentication, and desktop exit.
- The existing React/TypeScript code owns the main control panel. A dedicated React
  popup will replace the SwiftUI menu-bar view and share frontend types and language.
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

The preview bundle uses a distinct identifier and build output. Production identity,
launchd and installer sequencing are a separate migration stage. Development preview
builds must not register login items or change installed network services on launch.

For native acceptance, `scripts/desktop-smoke-service.py DIR` runs a harmless fixture.
Launch the preview executable with `--control-dir DIR`; scenario changes and service
restarts exercise sessions, discovery and UI state without touching the installed
gateway. Its observations record mutation payloads and SSE lifetimes, never credentials.

The [migration plan](../../../desktop-migration.md) tracks stage boundaries;
[GUI control-plane](gui-control-plane.md) documents the existing contracts to carry
forward, and [validation gates](validation-gates.md) define required evidence.
