# Wails desktop migration

> 归档于 2026-10-05。Next 桌面迁移开发已完成，且已通过 PR #81 合入 master（405daac）。原文中的分支计划和发布顺序仅属于当时任务。
> 来源：`docs/desktop-migration.md`，文档快照 `405daacba9720731608dba46dd2c7592e62b40d5`；快照提交不冒充被测提交。
> 当前方法见 [验证门槛](../../sources/validation/test-gates.md)，当前适用性见 [证据入口](../../sources/validation/evidence-map.md)。以下保留原文，历史指令不作为新任务的默认指令。

This page records the v0.3 migration on `Next`. The Wails host ships in v0.3.0;
remaining installation and platform checks are listed below.

The `Next` integration branch starts at `origin/master` commit
`72eaeb26485f3f9405b05de9fa74d006b637d103`, which contains release v0.2.4 through
PR #55. Each migration increment starts from the current `origin/Next` on a
`codex/feature/*` branch and reaches `Next` through a reviewed PR. Use an explicit
PR base of `Next`; the repository default branch remains `master`.

Each PR should deliver a buildable, independently verifiable increment. A stage can
span multiple PRs when its implementation or validation is too large for one review.
Integration merges preserve the PR boundary with merge commits. Maintenance fixes
enter `Next` through a separate synchronization PR.

## Stages

1. **Foundation:** isolated Wails module, pinned dependency, local preview bundle,
   frontend CI, macOS desktop builds, and the host/service architecture contract.
2. **Main window:** service discovery, native bootstrap authentication, existing
   React UI, bounded reconnect, external navigation handling, and main-window reopen.
3. **Menu bar:** native tray icon with a React popup, status/recovery display,
   shared language and theme, and visibility-aware refresh.
4. **Desktop lifecycle:** single instance, login item, close/quit semantics,
   Control Service wake/stop serialization, update discovery, and uninstall entry.
5. **Installer cutover:** production App identity, PKG upgrade/cleanup integration,
   architecture checks, macOS acceptance, user documentation, and old-host retirement.

Stages 1–4 and the stage 5 implementation now reach the `Next` PKG: the Wails host
uses the installed identity, and upgrade process handling recognises both Swift and
Wails executables. Default GUI build/test/package targets no longer use Swift.
The local preview remains isolated. The main window also uses a spacious default
and integrated native title bar.

Publishing v0.3.0 does not establish every platform acceptance result. Real
PackageKit upgrade/uninstall, login-item continuity and administrator authorization
must be verified on a disposable installation. macOS 13 and native Intel runtime
acceptance remain outstanding. See [installer acceptance](desktop-installation-v0.3.md).

## Validation

- Root `make test` continues to validate the Go service code on Linux.
- `make web-test` and `make web-build` validate the shared React/TypeScript UI.
- `make desktop-build` produces and checks a native macOS preview bundle; CI checks
  both Apple Silicon and Intel builds. This is not a UI or network acceptance test.
- Native smoke tests cover actual window/tray focus, reopen, file handling, language,
  service reconnect, close/quit, and the lowest supported macOS version.
- Changes to gateway lifecycle, networking, or cleanup require the relevant
  [validation gates](../../sources/validation/test-gates.md). A desktop build
  or unit-test result must not be reported as host-network evidence.

For v0.3.0 stable, use the existing `codex/release-v0.3.0` branch. Validate and
publish the exact release commit, then verify the published artifacts. Synchronize
`Next` to that commit and merge `Next` into `master` through a merge-commit PR,
following the maintainer's requested branch flow. The release tag and both branch
heads must preserve the same source tree.
