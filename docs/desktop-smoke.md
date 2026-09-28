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
6. Use `⌘1`–`⌘8` for pages and `⌘R` to refresh state without reloading a draft.
   `⌘W` hides the main window; opening the App again must reveal the same window
   and draft. Hidden/minimised/occluded windows suspend recurring requests and SSE.
7. Follow an HTTP(S) link to the system browser. Gateway confirmation uses a native
   sheet; cancel must leave the operation unsubmitted. File/custom-scheme navigation
   and unsolicited external redirects must not replace the bundled application.
8. Set the fixture's `recovery` to `true` and `mode` to `same_wifi_dhcp`. View its
   recovery card as plain text, close the modal, and download it with the native save
   panel. Cancel is not success. Compare the saved text and permissions with the
   fixture response. Check both languages, keyboard copy/paste, and window sizing.
   On first launch the window requests 1440 × 900 points (fitted to the available
   screen). Verify the transparent title area in both themes, the native traffic
   lights, top-strip dragging, resize, and quit/reopen restoring the chosen frame.
   Use only an isolated test bundle's saved frame when exercising first launch;
   do not remove a user's existing preview/window preference.

Run `make test web-test desktop-test` before opening a PR. Build both native
architectures with `OPENSURGE_DESKTOP_ARCH=arm64 make desktop-build` and
`OPENSURGE_DESKTOP_ARCH=x86_64 make desktop-build`. Restore the host architecture
before launching; a successful cross-build does not establish Intel UI behavior.

## Main window appearance and lifecycle

In both themes, open a connection filter and the language picker. Confirm themed
options and checkmarks, a thin green-grey scrollbar, and readable green text selection
in a search field. Exercise Arrow keys, Home/End, typeahead, Enter and Escape; Escape
must retain the previous selection, and clicking outside must close the menu.
Switch languages, allow background refreshes, and reopen the main window to confirm
the saved language and native menus agree. A failed save must restore the prior choice.

Close the main window and confirm the Dock entry disappears while the tray remains.
Reopen from the tray and a second launch; both restore Dock presence and the current
page. Cmd-H also hides Dock presence. Covering the main window with another app must
not hide the Dock entry; minimisation retains the usual Dock restore path.

Launch with `--smoke-actions --smoke-startup-delay 3s` in the isolated fixture to
inspect cold loading. Before React appears, the native window shows a light mint
gradient and the OpenSurge icon, with no black frame. The normal host has no artificial
delay. Logs distinguish native-placeholder installation from the first React frame.

## Menu-bar popup

Open the native tray popup (or use `⌘⇧M`). Verify its compact layout without the
main sidebar, focus-loss/Escape dismissal, and panel/network navigation reopening
the existing main window. Change language and theme in the main window and verify
the popup follows. Copy the fixture diagnostic summary and check native clipboard
paste. Toggle sleep prevention once from each window: each click records exactly
one PUT and the other window must show the acknowledged result.

Set `recovery: true` and `recovery_stage: gateway_stopped_waiting_router_dhcp` in
the fixture scenario to exercise recovery priority. Use `offline: true` to verify
unreachable status clears stale values and disables the sleep control; restore it
and increment `generation` to test reauthentication. With both windows hidden,
the native 15-second menu-bar status monitor keeps polling; a running/degraded
gateway also keeps its native two-second traffic observation loop. The main-window
reads and SSE remain stopped. This fixture never wakes or stops an installed service.

The overview additionally needs a running fixture with `traffic` and `routing`
objects in `scenario.json` (merged into their existing API responses). Use RFC 3339
sample timestamps supplied by the fixture, three downstream devices including one
inactive inventory row, and a separate `gateway_local`. Verify Mac-first rows, active
counts and rates after two samples. Routing and device rows must expand in place;
only their explicit desktop links navigate, retaining the selected connection owner. Set
`traffic_unavailable: true` to confirm old rates disappear; hide both windows for
more than twenty seconds, confirm native traffic reads continue while main UI/SSE
reads stop, then reopen only the popup. Its retained curve/rates should display
immediately, without waiting for a second renderer sample. A slow local-routing
response must not delay traffic. Stopping the gateway must stop native traffic reads.
Compare native light/dark backgrounds over another window, keyboard focus, and
reduced transparency/contrast. The menu icon must use the original image at 18 points.
Expand and collapse Network status repeatedly, including by keyboard: the disclosure
and native window must animate together, keep the top edge anchored and show no right
scrollbar. Values align at the start of the second column as in the Swift Grid.
Check that the footer remains visible; on a short display, content may scroll without
a visible scrollbar. With `--smoke-actions`, the host logs measured native window
heights and the actual template image size for this acceptance check.
These native checks require a build with `private_mac_apis`, as provided by the App
builder; a browser screenshot alone does not verify the AppKit material.

## Service lifecycle

For lifecycle acceptance, add `--smoke-actions` to the preview invocation above.
This flag requires a custom discovery directory. The fixture records lifecycle
commands as mutation bodies and never executes launchctl. Without this flag, custom
discovery directories disable service management.

Verify reconnect records only `print`/`bootstrap` as needed and `kickstart` without
`-k`. UI-only exit must confirm that the gateway continues and issue no stop command.
Full exit must be disabled for running, recovering, unavailable or unknown status.
With a stopped fixture, open its confirmation, change the scenario to running, and
confirm: the App must stay open and record no `bootout`. Reset to stopped, confirm
again, and verify exactly one `bootout gui/<uid>/com.opensurge.control` followed by
App exit. Cancellation must restore the popup. Test Cmd-Q as the UI-only route.
Launch/reopen from behind another application and repeat after hiding/minimising
the main window: the main window must come forward without becoming always-on-top.
Start exit from the tray with the main window closed and another App in front. Its
confirmation must be a visible sheet on the foreground main window; cancellation
restores the hidden main window and popup. No lifecycle command may run on cancel.

## Login items and updates

With `--smoke-actions`, open **More actions**; settings and updates are already expanded.
Verify that Show at login uses the same switch style as the sleep control. Login changes affect
only fixture state. `login_approval: true` makes enablement require approval;
`login_failure: true` rejects a change and must preserve the actual checkbox state.
Without smoke actions, do not register a preview login item just to test the UI.

The fixture defaults to a hypothetical `v0.2.5` release. `release_tag` changes it;
`update_failure: true` fails checks. Verify the stable release link, no-newer-release
state and retryable failure. A failed check must clear the prior download link.
The native App menu's update action opens this same panel. Real checks use only the
official GitHub repository; fixture release versions are not claims about publication.

## Uninstall entry

Without smoke actions, the preview must show a disabled uninstall entry explaining
that it cannot remove the installed App. With `--smoke-actions`, a stopped fixture
enables it. Confirm the native dialog has **Uninstall and Keep Data**, **Remove All
Data** and a default **Cancel**. Change the fixture to running while that dialog is
open; confirming must fail without recording uninstall or login changes.

`uninstall_outcome` defaults to `cancel`; set it to `failure` or `success` to exercise
the other results. With fixture login enabled, cancellation/failure must record
disable followed by restore and leave the App running; success must record the chosen
mode once, leave fixture login disabled, and exit the App. This does not execute the
installed script, ask for administrator privileges or delete any installed data.

The minimum target is macOS 13. Native acceptance on macOS 14 alone must not be
reported as a macOS 13 runtime test. Installer upgrade and real gateway/network
acceptance remain separate gates.
