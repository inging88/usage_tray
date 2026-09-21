#!/usr/bin/env bash
# macOS 빌드 — 맥에서 돌린다. 결과: dist/UsageTray.app (더블클릭 실행) + dist/usage-tray (바이너리)
#
# 왜 맥에서 빌드해야 하나: 메뉴바(NSStatusItem)는 Cocoa API 라 cgo 를 거친다. cgo 는
# 크로스 컴파일이 안 되므로 윈도우에서 맥 바이너리를 만들 수 없다. Xcode Command Line Tools
# 만 있으면 된다(`xcode-select --install`), 전체 Xcode 는 필요 없다.
#
# 서명을 안 하므로 처음 실행할 때 Gatekeeper 가 막는다 — README 의 '맥에서 쓰기' 참고.

set -euo pipefail
cd "$(dirname "$0")"

APP="dist/UsageTray.app"
BIN="dist/usage-tray"

mkdir -p dist
export CGO_ENABLED=1

# 애플 실리콘 + 인텔 둘 다 도는 유니버설 바이너리로 만든다.
if [ "${UNIVERSAL:-1}" = "1" ] && command -v lipo >/dev/null 2>&1; then
  GOARCH=arm64 go build -trimpath -ldflags '-s -w' -o dist/usage-tray-arm64 .
  GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o dist/usage-tray-amd64 .
  lipo -create -output "$BIN" dist/usage-tray-arm64 dist/usage-tray-amd64
  rm -f dist/usage-tray-arm64 dist/usage-tray-amd64
else
  go build -trimpath -ldflags '-s -w' -o "$BIN" .
fi

# .app 번들 — 메뉴바 앱은 Dock 아이콘이 없어야 하므로 LSUIElement=true 를 넣는다.
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN" "$APP/Contents/MacOS/usage-tray"

# 앱 아이콘 — assets/icon.png(1024) 한 장에서 굽는다. sips·iconutil 은 맥 기본 도구다.
# 메뉴바에 뜨는 그림이 아니라 Finder·로그인 항목·'정보 가져오기' 에 뜨는 번들 아이콘이다.
if [ -f assets/icon.png ] && command -v iconutil >/dev/null 2>&1; then
  ICONSET="$(mktemp -d)/AppIcon.iconset"
  mkdir -p "$ICONSET"
  for s in 16 32 128 256 512; do
    sips -z $s $s assets/icon.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
    sips -z $((s * 2)) $((s * 2)) assets/icon.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
  done
  iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
  rm -rf "$(dirname "$ICONSET")"
else
  echo "아이콘을 건너뛴다 (assets/icon.png 또는 iconutil 이 없다)"
fi
cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>UsageTray</string>
  <key>CFBundleDisplayName</key><string>Usage Tray</string>
  <key>CFBundleIdentifier</key><string>local.usage-tray</string>
  <key>CFBundleVersion</key><string>1.1.0</string>
  <key>CFBundleShortVersionString</key><string>1.1.0</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>usage-tray</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>LSUIElement</key><true/>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
</dict>
</plist>
PLIST

# 로컬 서명(ad-hoc) — 없으면 애플 실리콘에서 실행이 아예 막힌다. 배포용 공증과는 다르다.
codesign --force --deep --sign - "$APP" 2>/dev/null || echo "codesign 실패 — 그래도 직접 빌드한 맥에서는 돈다"

echo "빌드 완료: $APP"
echo "실행: open $APP"
echo "로그인 시 자동 실행: 시스템 설정 → 일반 → 로그인 항목 에 $APP 추가"
