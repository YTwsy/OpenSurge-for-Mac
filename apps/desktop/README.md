# OpenSurge desktop host

This separate Go module hosts the Wails desktop app shipped in OpenSurge v0.3. It pins
Wails v3 to an exact prerelease and keeps its native dependencies outside the root
Go module, so the existing Linux `make test` gate remains independent of WebKit/GTK.

Requirements: macOS, Go 1.25+, and Xcode Command Line Tools. Wails uses the system
WebView; no Chromium or Node runtime is bundled. Node and pnpm remain frontend
build tools.

From the repository root:

```sh
make desktop-build
open "bin/OpenSurge Desktop Preview.app"
```

The build defaults to the Go toolchain's architecture. Set
`OPENSURGE_DESKTOP_ARCH=arm64` or `x86_64` to select the other target. The minimum
deployment target is macOS 13.0; compiling on a newer macOS does not establish
runtime compatibility with macOS 13.

The preview embeds the existing React control panel at `wails://localhost` and
connects to the independent Control Service through native discovery and a bounded
HTTP relay. The bearer credential and authenticated cookie remain in native memory;
neither enters JavaScript. No gateway business implementation is linked into the host.

Start the installed OpenSurge app before opening the preview, or use the isolated
[smoke fixture](../../docs/agent-wiki/sources/validation/desktop-smoke.md). Session and endpoint changes reconnect
automatically while preserving the current page and drafts. Mutations are never
replayed automatically. `⌘R` refreshes state without reloading, and `⌘1`–`⌘8`
navigate the existing pages. Closing the window hides it; Dock reopen, a second
launch or **View → Show Main Window** preserves its state. `⌘Q` confirms UI-only exit;
the gateway and background service keep running.

First launch uses a 1440 × 900 point window, constrained to the screen's available
area. Users can resize it (minimum 1080 × 640); subsequent launches restore their
chosen frame. A transparent inset title bar keeps the native traffic lights over
the sidebar and hides the duplicate window title. The empty top strip supports
native dragging and double-click behaviour; it does not cover page controls.

Native menus follow the UI language. HTTP(S) links open in the system browser;
confirmation, clipboard and recovery-card saving use macOS facilities. Recurring
reads and SSE pause while the window is hidden, minimised or occluded. A native
menu-bar icon opens a separate compact React popup (`⌘⇧M` also opens it). It follows
the main window's language/theme and shares the authoritative sleep-prevention
control, with recovery reminders and a diagnostic summary. Reconnect can wake the
installed user Control Service. Full exit requires a fresh stopped/recovered status,
then stops only that user service and leaves the root Helper available. Both exit
choices require native confirmation. The popup's App settings report the actual
macOS login-item state, including approval and failures. Login registration is
explicit and scoped to this bundle; launch does not register anything. Stable-release
discovery checks the official GitHub repository daily and supports a manual check.
It opens a verified release page without downloading or installing an update.
The uninstall entry offers keep-data and remove-all choices through native
confirmation, then delegates to the existing installed script. The preview identity
cannot uninstall a production installation; smoke mode records fixture actions only.
Run the native client and host boundary tests with `make desktop-test`.

`make desktop-production-build` builds `bin/OpenSurge.app` with the existing
installed identity `com.opensurge.menubar` and the new `OpenSurgeDesktop` executable.
It accepts `OPENSURGE_APP_ARCH`, `OPENSURGE_VERSION`, `OPENSURGE_BUILD_NUMBER` and
`OPENSURGE_RELEASE_TAG`; version and tag must agree. Both variants are built in a
clean staging bundle and verified for metadata, architecture, macOS 13 deployment
target and ad-hoc signature. Rebuilding a running output is refused.

Only the production identity at `/Applications/OpenSurge.app`, using the default
Control Service directory, wakes the installed user service once on launch. It does
not restart an already running service or start the gateway. A failed wake leaves
the UI available for explicit reconnect. A custom directory never operates the real
job; production `--smoke-actions` exercises launch wake with a fixture runner.
Preview and production single-instance identities are separate.

The preview retains its separate identifier and output. Since v0.3, `make gui-build`
and `make gui-installer` use this Wails host. The Swift host remains only as
maintenance source in `apps/menubar`; its explicit legacy build writes under
`bin/legacy/`. Local bundles are ad-hoc signed, not Developer ID signed or notarized.
See [installer acceptance](../../docs/agent-wiki/sources/validation/evidence-map.md#桌面与安装) for validation limits.

See [the migration history](../../docs/agent-wiki/tasks/finished_archived/desktop-migration-v0.3.md) and
[the desktop host contract](../../docs/agent-wiki/sources/decisions/desktop-host.md).
