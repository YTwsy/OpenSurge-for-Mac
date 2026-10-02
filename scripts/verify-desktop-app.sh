#!/usr/bin/env bash
set -euo pipefail

APP="${1:?App path required}"
ARCH="${2:?architecture required}"
VARIANT="${3:?preview or production required}"
VERSION="${4:?version required}"
BUILD_NUMBER="${5:?build number required}"
RELEASE_TAG="${6:?release tag required}"
case "$VARIANT" in
  preview) NAME="OpenSurge Desktop Preview"; ID=com.opensurge.desktop.preview ;;
  production) NAME=OpenSurge; ID=com.opensurge.menubar ;;
  *) exit 2 ;;
esac
PLIST="$APP/Contents/Info.plist"
check_value() {
  [[ "$(/usr/libexec/PlistBuddy -c "Print :$1" "$PLIST")" == "$2" ]] || {
    echo "desktop bundle has incorrect $1 (expected $2)" >&2; exit 1;
  }
}
/usr/bin/plutil -lint "$PLIST"
check_value CFBundleIdentifier "$ID"
check_value CFBundleName "$NAME"
check_value CFBundleDisplayName "$NAME"
check_value CFBundleExecutable OpenSurgeDesktop
check_value CFBundleShortVersionString "$VERSION"
check_value CFBundleVersion "$BUILD_NUMBER"
check_value OpenSurgeReleaseTag "$RELEASE_TAG"
check_value OpenSurgeReleaseCodename "$(python3 "$(dirname "$0")/release-codename.py" "$RELEASE_TAG")"
check_value LSMinimumSystemVersion 13.0
check_value CFBundleIconFile OpenSurgeAppIcon
check_value CFBundleLocalizations:0 en
check_value CFBundleLocalizations:1 zh-Hans
[[ -s "$APP/Contents/Resources/OpenSurgeAppIcon.icns" && ! -e "$APP/Contents/MacOS/OpenSurgeMenuBar" ]]
EXECUTABLE="$APP/Contents/MacOS/OpenSurgeDesktop"
[[ -x "$EXECUTABLE" ]]
[[ "$(/usr/bin/lipo -archs "$EXECUTABLE")" == "$ARCH" ]]
[[ "$(xcrun vtool -show-build "$EXECUTABLE" | awk '/minos/ { print $2; exit }')" == 13.0 ]]
/usr/bin/codesign --verify --strict "$APP"
printf 'Verified %s %s (%s) for %s, macOS 13.0+\n' "$NAME" "$RELEASE_TAG" "$BUILD_NUMBER" "$ARCH"
