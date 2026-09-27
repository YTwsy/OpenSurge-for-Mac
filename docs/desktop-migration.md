# Wails desktop migration

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

## Validation

- Root `make test` continues to validate the Go service code on Linux.
- `make web-test` and `make web-build` validate the shared React/TypeScript UI.
- `make desktop-build` produces and checks a native macOS preview bundle; CI checks
  both Apple Silicon and Intel builds. This is not a UI or network acceptance test.
- Native smoke tests cover actual window/tray focus, reopen, file handling, language,
  service reconnect, close/quit, and the lowest supported macOS version.
- Changes to gateway lifecycle, networking, or cleanup require the relevant
  [validation gates](agent-wiki/wiki/concepts/validation-gates.md). A desktop build
  or unit-test result must not be reported as host-network evidence.

When the migration is ready for release, create a versioned release branch from a
verified `Next` commit. Validate and publish from that release branch, verify the
published artifacts, then PR that same release branch into `master`. Release fixes
flow back to `Next` through PRs.
