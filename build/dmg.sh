#!/bin/bash
# Create a macOS DMG with drag-to-Applications layout.
# Usage: ./build/dmg.sh <version>
# Output: build/bin/ForkSync-<version>.dmg
set -euo pipefail

VERSION="${1:?usage: dmg.sh <version>}"
APP_NAME="forksync"
PRODUCT_NAME="ForkSync"
SRC_APP="build/bin/${APP_NAME}.app"
OUTPUT_DMG="build/bin/${PRODUCT_NAME}-${VERSION}.dmg"
STAGING="build/dmg-staging"
VOL_NAME="${PRODUCT_NAME}"

if [ ! -d "$SRC_APP" ]; then
  echo "Error: $SRC_APP not found. Run 'make wails' first."
  exit 1
fi

echo "=== Creating DMG for ${PRODUCT_NAME} v${VERSION} ==="

# Clean up any previous artifacts
rm -rf "$STAGING"
rm -f "$OUTPUT_DMG" "${OUTPUT_DMG}.master.dmg"

# Prepare staging directory with app + Applications symlink
mkdir -p "$STAGING"
cp -R "$SRC_APP" "$STAGING/"
ln -s /Applications "$STAGING/Applications"

# Create a read-write DMG
RW_DMG="${OUTPUT_DMG}.rw.dmg"
hdiutil create -ov -volname "$VOL_NAME" \
  -srcfolder "$STAGING" \
  -fs HFS+ -format UDRW \
  "$RW_DMG" >/dev/null

# Mount the DMG and position icons via AppleScript
MOUNT_DIR="/Volumes/$VOL_NAME"
hdiutil attach -readwrite -noverify -noautoopen "$RW_DMG" >/dev/null

osascript <<APPLESCRIPT
tell application "Finder"
  set theDisk to disk "$VOL_NAME"
  open theDisk
  set theWin to container window of theDisk
  set current view of theWin to icon view
  set toolbar visible of theWin to false
  set statusbar visible of theWin to false
  set the bounds of theWin to {200, 120, 760, 440}
  set viewOptions to icon view options of theWin
  set arrangement of viewOptions to not arranged
  set icon size of viewOptions to 96
  set position of every item of theWin to {150, 150}
  set position of item "Applications" of theWin to {410, 150}
  close theWin
  open theDisk
end tell
APPLESCRIPT

# Give Finder a moment to save positions
sleep 2

# Unmount
hdiutil detach "$MOUNT_DIR" >/dev/null

# Convert to compressed read-only DMG
hdiutil convert "$RW_DMG" -format UDZO -imagekey zlib-level=9 -o "$OUTPUT_DMG" >/dev/null
rm -f "$RW_DMG"
rm -rf "$STAGING"

echo "Built: $OUTPUT_DMG"
echo "Size:  $(du -h "$OUTPUT_DMG" | cut -f1)"
