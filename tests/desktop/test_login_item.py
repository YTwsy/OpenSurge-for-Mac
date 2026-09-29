#!/usr/bin/env python3
"""macOS integration gate: register only a temporary, uniquely identified test App.

Links the production ServiceManagement bridge, replaces an ad-hoc-signed binary,
and verifies explicit registration after replacement. Both test signatures are
unregistered before removing the temporary bundle. No installed OpenSurge state,
gateway, launchd service or system login-item database reset is involved.
"""
import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[2]
HARNESS = r'''
#import <Foundation/Foundation.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
const char *openSurgeLoginStatus(void);
bool setOpenSurgeLogin(bool enabled, char **message);
bool openSurgeIsInstalledApp(void);
int main(int argc, char **argv) {
    @autoreleasepool {
        NSString *before = @(openSurgeLoginStatus());
        char *message = NULL;
        bool ok = true;
        if (argc > 1 && strcmp(argv[1], "enable") == 0) ok = setOpenSurgeLogin(true, &message);
        if (argc > 1 && strcmp(argv[1], "disable") == 0) ok = setOpenSurgeLogin(false, &message);
        NSDictionary *result = @{@"build": BUILD_STAMP, @"before": before,
            @"after": @(openSurgeLoginStatus()), @"ok": @(ok),
            @"installed": @(openSurgeIsInstalledApp()), @"error": message ? @(message) : @""};
        free(message);
        NSData *data = [NSJSONSerialization dataWithJSONObject:result options:0 error:nil];
        puts([[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding].UTF8String);
    }
    return 0;
}
'''


def main():
    if sys.platform != "darwin" or os.getuid() == 0:
        raise SystemExit("Run as a logged-in macOS user, without sudo.")
    root = Path(tempfile.mkdtemp(prefix="opensurge-login-test-", dir="/private/tmp"))
    app = root / "OpenSurge Login Test.app"
    contents = app / "Contents"
    executable = contents / "MacOS" / "LoginTest"
    executable.parent.mkdir(parents=True)
    identifier = "com.opensurge.test.login." + root.name.removeprefix("opensurge-login-test-").replace("_", "-")
    (contents / "Info.plist").write_bytes(plistlib.dumps({
        "CFBundleIdentifier": identifier, "CFBundleExecutable": executable.name,
        "CFBundleName": "OpenSurge Login Test", "CFBundlePackageType": "APPL", "LSUIElement": True,
    }))
    source = root / "main.m"
    source.write_text(HARNESS)
    versions = []

    def probe(action):
        result = subprocess.run([str(executable), action], check=True, capture_output=True, text=True, timeout=20)
        value = json.loads(result.stdout)
        print(action, json.dumps(value), flush=True)
        return value

    def activate(binary):
        candidate = root / "candidate"
        shutil.copy2(binary, candidate)
        os.replace(candidate, executable)

    def build(stamp):
        candidate = root / "candidate"
        subprocess.run([
            "clang", "-fobjc-arc", "-mmacosx-version-min=13.0", '-DBUILD_STAMP=@"' + stamp + '"',
            str(source), str(ROOT / "apps/desktop/internal/native/login_darwin.m"),
            "-framework", "Foundation", "-framework", "ServiceManagement", "-o", str(candidate),
        ], check=True)
        os.replace(candidate, executable)
        subprocess.run(["codesign", "--force", "--sign", "-", "--timestamp=none", str(app)], check=True, capture_output=True)
        saved = root / stamp
        shutil.copy2(executable, saved)
        versions.append(saved)

    print("Isolated test bundle:", identifier, flush=True)
    try:
        build("before-upgrade")
        initial = probe("status")
        assert not initial["installed"], initial
        assert initial["after"] == "not_found", initial
        first = probe("enable")
        assert first["ok"] and first["after"] == "enabled", first

        build("after-upgrade")
        replaced = probe("status")
        assert replaced["after"] == "not_found", replaced
        recovered = probe("enable")
        assert recovered["ok"] and recovered["after"] == "enabled", recovered
        assert probe("status")["after"] == "enabled"
        disabled = probe("disable")
        assert disabled["ok"] and disabled["after"] == "disabled", disabled
        assert probe("status")["after"] == "disabled"
    finally:
        failures = []
        for binary in reversed(versions):
            try:
                activate(binary)
                state = probe("status")
                if state["after"] in ("enabled", "approval"):
                    state = probe("disable")
                if state["after"] not in ("disabled", "not_found"):
                    failures.append(str(binary))
            except Exception as error:
                failures.append(f"{binary}: {error}")
        if failures:
            raise RuntimeError(f"Test login cleanup failed; retained {root}: {failures}")
        shutil.rmtree(root)
        print("Both test signatures are inactive; temporary bundle removed.", flush=True)
    print("LOGIN_ITEM_RECOVERY_OK", flush=True)


if __name__ == "__main__":
    main()
