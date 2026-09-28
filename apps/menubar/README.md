# Legacy Swift menu-bar host

This source is retained for v0.2.4 maintenance and migration reference. `Next`
GUI build/test/package targets use `apps/desktop` (Wails + system WebView).

An explicit `make menubar-build` writes `bin/legacy/OpenSurge.app`; it cannot
replace the production Wails build at `bin/OpenSurge.app`. The old host keeps
its historical bundle identity and should not be installed alongside the new
host. Its tests remain available through `make menubar-test` for legacy work.

See [desktop migration](../../docs/desktop-migration.md) and
[installer acceptance](../../docs/desktop-installation.md).
