# Legacy Swift menu-bar host

This source is retained for v0.2.4 maintenance and migration reference. v0.3
GUI build/test/package targets use `apps/desktop` (Wails + system WebView).

An explicit `make menubar-build` writes `bin/legacy/OpenSurge.app`; it cannot
replace the production Wails build at `bin/OpenSurge.app`. The old host keeps
its historical bundle identity and should not be installed alongside the new
host. Its tests remain available through `make menubar-test` for legacy work.

See [desktop migration](../../docs/agent-wiki/tasks/finished_archived/desktop-migration-v0.3.md) and
[installer acceptance](../../docs/agent-wiki/sources/validation/evidence-map.md#桌面与安装).

Historical panel failures and maintenance constraints: [Swift host notes](../../docs/agent-wiki/tasks/finished_archived/swift-menubar-focus-regression.md).
