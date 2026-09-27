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

Start with the Control Service's loopback-served frontend, HTTP API, and SSE. The
host obtains a one-time bootstrap grant using the native credential; the WebView
receives an HttpOnly session, never the long-lived bearer token. Desktop bindings
are limited to explicit desktop capabilities. Do not expose general shell execution
or bind gateway managers into the GUI process.

The preview's native client accepts only explicit-port HTTP loopback discovery,
dials IPv4 loopback directly, and refuses redirects. It validates the returned
bootstrap URL against the discovered service origin before navigating. Discovery
and credentials are reread for each connection attempt. The initial main-window
increment reconnects through the native menu; it does not yet renew a WebView
session automatically or launch/stop the installed Control Service.

Tray status must remain available without mounting the full main React application.
Hidden-window work must be reduced explicitly rather than relying on browser-tab
throttling. Reconnect must recover authentication and state without replaying an
unacknowledged privileged mutation.

The preview bundle uses a distinct identifier and build output. Production identity,
launchd and installer sequencing are a separate migration stage. Development preview
builds must not register login items or change installed network services on launch.

The [migration plan](../../../desktop-migration.md) tracks stage boundaries;
[GUI control-plane](gui-control-plane.md) documents the existing contracts to carry
forward, and [validation gates](validation-gates.md) define required evidence.
