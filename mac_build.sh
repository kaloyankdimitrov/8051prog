#!/bin/bash
set -euo pipefail

APP_NAME="8051Prog"
APP_ID="com.github.kaloyankdimitrov.8051prog"
BUILD_DIR="build"
APP_BUNDLE="$BUILD_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_BUNDLE/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

echo "==> Cleaning old build..."
rm -rf "$BUILD_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

echo "==> Building Go binary for darwin/arm64..."
GOOS=darwin GOARCH=arm64 go build -o "$MACOS_DIR/$APP_NAME" ./main.go

echo "==> Copying avrdude folder (excluding windows/ and linux/)..."
rsync -av --exclude 'windows' --exclude 'linux' avrdude "$RESOURCES_DIR/"

echo "==> Creating Info.plist..."
cat > "$CONTENTS_DIR/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
 "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>$APP_ID</string>
    <key>CFBundleExecutable</key>
    <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
</dict>
</plist>
EOF

echo "==> Packaging into zip..."
cd "$BUILD_DIR"
zip -r "$APP_NAME-macos.zip" "$APP_NAME.app"
cd ..

echo "✅ Build complete: $BUILD_DIR/$APP_NAME-macos.zip"