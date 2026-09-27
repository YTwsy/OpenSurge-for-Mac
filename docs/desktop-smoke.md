# Native desktop acceptance

Build with `make web-build desktop-test desktop-build`. The preview bundle is
`bin/OpenSurge Desktop Preview.app`; it has a separate identity from the installed
Swift host. The shared frontend identifies an unversioned Next build as
`v0.2.4-next`; release builds override `OPENSURGE_RELEASE_TAG`.

Use the isolated fixture for actions, restart and failure tests:

```sh
python3 scripts/desktop-smoke-service.py /private/tmp/opensurge-desktop-smoke
# In another terminal:
"bin/OpenSurge Desktop Preview.app/Contents/MacOS/OpenSurgeDesktop" \
  --control-dir /private/tmp/opensurge-desktop-smoke
```

The fixture has no Helper, gateway processes, launchd actions or real network
configuration. It serves illustrative API responses and records request counts,
mutation bodies and event-stream activity in `observations.json`. It is a UI
transport fixture, not a substitute for Control API or network validation.

1. Confirm the main window loads from `wails://localhost` and shows fixture LAN IP
   `192.0.2.10`. Toggle sleep prevention and verify one PUT per explicit click,
   including the correct JSON body. This changes only the fixture's in-memory state.
2. Import a newly created, non-sensitive YAML using the system file picker. Confirm
   the fixture records the filename and byte count, and the UI receives its response.
3. Type an unsaved source name or network field. Set `offline: true` in the fixture's
   `scenario.json`; verify a reconnect message appears without losing the page or
   draft. Set `offline: false` and increment `generation` to reject the old cookie;
   confirm automatic reauthentication and the same input element/value.
4. Restart the fixture using the same directory. It chooses a new loopback port;
   verify discovery follows the new endpoint without reloading the WebView.
5. Confirm SSE opens and reconnects, closes when the window host exits, and carries
   state changes before the stream ends. Unit tests additionally assert cancellation
   and prohibit automatic mutation replay after authentication failure.

Run `make test web-test desktop-test` before opening a PR. Build both native
architectures with `OPENSURGE_DESKTOP_ARCH=arm64 make desktop-build` and
`OPENSURGE_DESKTOP_ARCH=x86_64 make desktop-build`. Restore the host architecture
before launching; a successful cross-build does not establish Intel UI behavior.

The minimum target is macOS 13. Native acceptance on macOS 14 alone must not be
reported as a macOS 13 runtime test. Installer upgrade and real gateway/network
acceptance remain separate gates.
