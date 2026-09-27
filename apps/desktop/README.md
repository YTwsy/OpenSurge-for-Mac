# OpenSurge desktop host

This separate Go module hosts the Next branch's Wails desktop migration. It pins
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

The preview connects to the already-running Control Service using its per-user
discovery file and native credential. It exchanges that credential for a one-time
bootstrap URL and loads the existing React control panel in WKWebView. The WebView
uses the service's HttpOnly session; the native credential is never passed to JS.
No gateway business implementation is linked into the host.

Start the installed OpenSurge app before opening the preview. Startup retries are
bounded; use **Control Panel → Reconnect** (Cmd-R) after a service restart, session
expiry, or a failed initial connection. This returns to the dashboard and discards
the current page's unsaved UI state. Reconnect never repeats a gateway operation.
Closing the window hides it; reopening from the Dock or **Show Window** (Cmd-1)
preserves the page. Cmd-Q exits the preview UI only.

This is the first main-window increment. Automatic session recovery, external links,
downloads, tray UI, login registration, and service lifecycle management are separate
follow-ups. Test the native authentication client with `make desktop-test`.

This preview has a separate bundle identifier and output path; production
PKG builds continue to use the existing menu-bar application until the installation
cutover is independently reviewed. The local bundle is ad-hoc signed, not Developer
ID signed or notarized.

See [the migration plan](../../docs/desktop-migration.md) and
[the desktop host contract](../../docs/agent-wiki/wiki/concepts/desktop-host.md).
