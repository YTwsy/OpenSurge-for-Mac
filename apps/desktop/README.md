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

The foundation currently opens a minimal host window. The next PR connects the
existing React control panel through the Control Service's authenticated loopback
endpoint. This preview has a separate bundle identifier and output path; production
PKG builds continue to use the existing menu-bar application until the installation
cutover is independently reviewed. The local bundle is ad-hoc signed, not Developer
ID signed or notarized.

See [the migration plan](../../docs/desktop-migration.md) and
[the desktop host contract](../../docs/agent-wiki/wiki/concepts/desktop-host.md).
