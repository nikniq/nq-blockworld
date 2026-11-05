#!/usr/bin/env bash
# cx.sh - build and package Blockworld.
#
#   ./cx.sh                   test, build and package for this machine into dist/
#   ./cx.sh --build-only      just compile ./blockworld (or blockworld.exe): no tests, no package
#   ./cx.sh --version 1.2     version stamped into the game, package names and metadata
#                             (default: the latest git tag, or 1.0 when there is none)
#   ./cx.sh --skip-tests      package without running the test suite
#   ./cx.sh --universal       macOS: one app that runs on both Apple silicon and Intel
#   ./cx.sh --dmg             macOS: also write a drag-to-Applications disk image
#   ./cx.sh --target windows  macOS/Linux: cross-compile the Windows package (needs mingw-w64)
#   ./cx.sh --deps            install the build dependencies first (Linux: apt, dnf, pacman,
#                             zypper or apk via sudo; macOS: command line tools, Go via brew)
#   ./cx.sh --install         install the packaged game for the current user (Linux app menu,
#                             macOS /Applications); combine with --deps for a one-shot setup
#   ./cx.sh --run             launch the packaged game afterwards
#   ./cx.sh --clean           remove dist/ and build outputs
#
# Output (always under dist/):
#   macOS    Blockworld.app, Blockworld-<ver>-macos-<arch>.zip (and .dmg with --dmg)
#   Linux    blockworld-<ver>-linux-<arch>.tar.gz (binary, icon, .desktop, install.sh, uninstall.sh)
#   Windows  blockworld-<ver>-windows-<arch>.zip (blockworld.exe with embedded icon)
# plus SHA256SUMS. raylib is compiled through cgo, so a package is built on its own
# platform, except Windows, which can also be cross-compiled with --target windows.
# On Windows run this from Git Bash or MSYS2, or use build.ps1 from PowerShell.
set -euo pipefail

cd "$(dirname "$0")"

say()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required: $2"; }

VERSION=""
RUN_TESTS=1
LAUNCH=0
DEPS=0
INSTALL=0
UNIVERSAL=0
DMG=0
BUILD_ONLY=0
TARGET=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version)    [ $# -ge 2 ] || fail "--version needs a value"; VERSION="$2"; shift ;;
    --version=*)  VERSION="${1#*=}" ;;
    --target)     [ $# -ge 2 ] || fail "--target needs a value"; TARGET="$2"; shift ;;
    --target=*)   TARGET="${1#*=}" ;;
    --skip-tests) RUN_TESTS=0 ;;
    --build-only) BUILD_ONLY=1 ;;
    --universal)  UNIVERSAL=1 ;;
    --dmg)        DMG=1 ;;
    --run)        LAUNCH=1 ;;
    --deps)       DEPS=1 ;;
    --install)    INSTALL=1 ;;
    --clean)      rm -rf dist blockworld blockworld.exe blockworld-macos blockworld-linux ./*.syso; echo "cleaned"; exit 0 ;;
    -h|--help)    sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)            fail "unknown option: $1 (see ./cx.sh --help)" ;;
  esac
  shift
done

if [ -z "$VERSION" ]; then
  VERSION="$(git describe --tags --abbrev=0 2>/dev/null || true)"
  VERSION="${VERSION#v}"
  [ -n "$VERSION" ] || VERSION="1.0"
fi

HOST="$(uname -s)"
case "$HOST" in
  Darwin) HOST=macOS ;;
  Linux) HOST=Linux ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT) HOST=Windows ;;
  *) fail "unsupported platform: $HOST" ;;
esac
ARCH="$(uname -m)"
case "$ARCH" in x86_64|amd64) ARCH=x64 ;; arm64|aarch64) ARCH=arm64 ;; esac

# What is being built: this machine's platform unless --target says otherwise.
OS="$HOST"
CROSS=0
case "$TARGET" in
  "") ;;
  windows|Windows) [ "$HOST" = Windows ] || { OS=Windows; CROSS=1; ARCH=x64; } ;;
  macos|macOS|darwin) [ "$HOST" = macOS ] || fail "the macOS package can only be built on a Mac" ;;
  linux|Linux) [ "$HOST" = Linux ] || fail "the Linux package can only be built on Linux" ;;
  *) fail "unknown target: $TARGET (windows, macos or linux)" ;;
esac
[ "$UNIVERSAL" = 0 ] || [ "$OS" = macOS ] || fail "--universal only applies to the macOS package"
[ "$DMG" = 0 ] || [ "$OS" = macOS ] || fail "--dmg only applies to the macOS package"
MINGW_CC="x86_64-w64-mingw32-gcc"

# ---------- optional dependency install ----------
if [ "$DEPS" = 1 ]; then
  case "$HOST" in
    Linux)
      if command -v apt-get >/dev/null 2>&1; then
        say "installing dependencies with apt"
        sudo apt-get update
        sudo apt-get install -y golang-go build-essential libgl1-mesa-dev libx11-dev libxi-dev \
          libxcursor-dev libxrandr-dev libxinerama-dev libwayland-dev libxkbcommon-dev
        [ "$CROSS" = 0 ] || sudo apt-get install -y gcc-mingw-w64-x86-64 zip
      elif command -v dnf >/dev/null 2>&1; then
        say "installing dependencies with dnf"
        sudo dnf install -y golang gcc mesa-libGL-devel libX11-devel libXi-devel libXcursor-devel \
          libXrandr-devel libXinerama-devel wayland-devel libxkbcommon-devel
        [ "$CROSS" = 0 ] || sudo dnf install -y mingw64-gcc zip
      elif command -v pacman >/dev/null 2>&1; then
        say "installing dependencies with pacman"
        sudo pacman -S --needed --noconfirm go gcc mesa libx11 libxi libxcursor libxrandr libxinerama wayland libxkbcommon
        [ "$CROSS" = 0 ] || sudo pacman -S --needed --noconfirm mingw-w64-gcc zip
      elif command -v zypper >/dev/null 2>&1; then
        say "installing dependencies with zypper"
        sudo zypper install -y go gcc Mesa-libGL-devel libX11-devel libXi-devel libXcursor-devel \
          libXrandr-devel libXinerama-devel wayland-devel libxkbcommon-devel
      elif command -v apk >/dev/null 2>&1; then
        say "installing dependencies with apk"
        sudo apk add go build-base mesa-dev libx11-dev libxi-dev libxcursor-dev libxrandr-dev \
          libxinerama-dev wayland-dev libxkbcommon-dev
      else
        fail "no supported package manager found (apt, dnf, pacman, zypper or apk); install Go, gcc and the OpenGL/X11 headers by hand"
      fi
      ;;
    macOS)
      say "installing the command line tools (clang)"
      xcode-select --install 2>/dev/null || true
      if command -v brew >/dev/null 2>&1; then
        command -v go >/dev/null 2>&1 || brew install go
        [ "$CROSS" = 0 ] || command -v "$MINGW_CC" >/dev/null 2>&1 || brew install mingw-w64
      fi
      ;;
    *) say "--deps is not automated on $HOST; see the README" ;;
  esac
fi

# ---------- toolchain checks ----------
need go "install Go 1.25 or newer from https://go.dev/dl/ (or run ./cx.sh --deps)"
case "$OS" in
  macOS)
    need clang "run: xcode-select --install"
    ;;
  Linux)
    need gcc "e.g. sudo apt install build-essential, or sudo dnf install gcc (or run ./cx.sh --deps)"
    for h in GL/gl.h X11/Xlib.h; do
      if ! echo "#include <$h>" | gcc -E - >/dev/null 2>&1; then
        fail "missing development header $h; run ./cx.sh --deps, or see the README for your distribution's package list"
      fi
    done
    ;;
  Windows)
    if [ "$CROSS" = 1 ]; then
      need "$MINGW_CC" "install mingw-w64 (brew install mingw-w64, sudo apt install gcc-mingw-w64-x86-64, or ./cx.sh --target windows --deps)"
    else
      need gcc "install a GCC toolchain such as w64devkit or MSYS2 mingw-w64 and put gcc on PATH"
    fi
    ;;
esac

LDFLAGS="-s -w -X main.gameVersion=$VERSION"
export CGO_ENABLED=1

# gobuild <output> [extra ldflags]: one release build with the shared flags.
gobuild() {
  go build -trimpath -ldflags="$LDFLAGS${2:+ $2}" -o "$1" .
}

# ---------- quick build ----------
if [ "$BUILD_ONLY" = 1 ]; then
  if [ "$OS" = Windows ]; then
    say "building blockworld.exe"
    if [ "$CROSS" = 1 ]; then
      GOOS=windows GOARCH=amd64 CC="$MINGW_CC" gobuild blockworld.exe "-H windowsgui"
    else
      gobuild blockworld.exe "-H windowsgui"
    fi
    OUT="./blockworld.exe"
  else
    say "building blockworld"
    gobuild blockworld
    OUT="./blockworld"
  fi
  say "done: $OUT"
  if [ "$LAUNCH" = 1 ] && [ "$CROSS" = 0 ]; then "$OUT"; fi
  exit 0
fi

mkdir -p dist
[ "$UNIVERSAL" = 0 ] || ARCH=universal
say "Blockworld $VERSION for $OS/$ARCH"

# ---------- icon and tests ----------
say "generating icon"
CGO_ENABLED=0 go run ./cmd/mkicon

if [ "$RUN_TESTS" = 1 ]; then
  say "running tests"
  if ! go test ./... > dist/test.log 2>&1; then
    tail -40 dist/test.log
    fail "tests failed (full log in dist/test.log)"
  fi
  rm -f dist/test.log
fi

# ---------- package ----------
case "$OS" in
  macOS)
    need iconutil "ships with macOS"
    need sips "ships with macOS"
    export MACOSX_DEPLOYMENT_TARGET=11.0
    APP="dist/Blockworld.app"
    rm -rf "$APP" dist/icon.iconset
    mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" dist/icon.iconset
    if [ "$UNIVERSAL" = 1 ]; then
      need lipo "ships with the command line tools"
      say "building for Apple silicon"
      GOOS=darwin GOARCH=arm64 gobuild dist/.blockworld-arm64
      say "building for Intel"
      GOOS=darwin GOARCH=amd64 gobuild dist/.blockworld-amd64
      lipo -create -output "$APP/Contents/MacOS/blockworld" dist/.blockworld-arm64 dist/.blockworld-amd64
      rm -f dist/.blockworld-arm64 dist/.blockworld-amd64
    else
      say "building"
      gobuild "$APP/Contents/MacOS/blockworld"
    fi
    say "assembling $APP"
    for s in 16 32 128 256 512; do
      sips -z "$s" "$s" icon.png --out "dist/icon.iconset/icon_${s}x${s}.png" >/dev/null
      d=$((s*2)); sips -z "$d" "$d" icon.png --out "dist/icon.iconset/icon_${s}x${s}@2x.png" >/dev/null
    done
    iconutil -c icns dist/icon.iconset -o "$APP/Contents/Resources/blockworld.icns"
    rm -rf dist/icon.iconset
    cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Blockworld</string>
  <key>CFBundleDisplayName</key><string>Blockworld</string>
  <key>CFBundleIdentifier</key><string>com.blockworld.game</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleExecutable</key><string>blockworld</string>
  <key>CFBundleIconFile</key><string>blockworld</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.games</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
</dict></plist>
PLIST
    say "signing (ad hoc)"
    codesign --force --deep --sign - "$APP"
    ZIP="Blockworld-$VERSION-macos-$ARCH.zip"
    rm -f "dist/$ZIP"
    (cd dist && ditto -c -k --keepParent Blockworld.app "$ZIP")
    if [ "$DMG" = 1 ]; then
      say "writing disk image"
      rm -rf dist/dmg "dist/Blockworld-$VERSION-macos-$ARCH.dmg"
      mkdir -p dist/dmg
      cp -R "$APP" dist/dmg/
      ln -s /Applications dist/dmg/Applications
      hdiutil create -quiet -volname Blockworld -srcfolder dist/dmg -ov -format UDZO \
        "dist/Blockworld-$VERSION-macos-$ARCH.dmg"
      rm -rf dist/dmg
    fi
    OUT="$APP"
    ;;
  Linux)
    PKG="blockworld-$VERSION-linux-$ARCH"
    STAGE="dist/$PKG"
    rm -rf "$STAGE"
    mkdir -p "$STAGE"
    say "building"
    gobuild "$STAGE/blockworld"
    say "assembling $STAGE"
    cp icon.png "$STAGE/"
    cat > "$STAGE/blockworld.desktop" <<DESK
[Desktop Entry]
Type=Application
Name=Blockworld
Comment=Mine by day, survive the night
Exec=blockworld
Icon=blockworld
Terminal=false
Categories=Game;
DESK
    cat > "$STAGE/install.sh" <<'INST'
#!/usr/bin/env bash
# Installs Blockworld for the current user with a menu entry and icon.
set -e
cd "$(dirname "$0")"
mkdir -p ~/.local/bin ~/.local/share/applications ~/.local/share/icons/hicolor/512x512/apps
cp blockworld ~/.local/bin/blockworld
cp icon.png ~/.local/share/icons/hicolor/512x512/apps/blockworld.png
sed "s|^Exec=.*|Exec=$HOME/.local/bin/blockworld|" blockworld.desktop > ~/.local/share/applications/blockworld.desktop
update-desktop-database ~/.local/share/applications 2>/dev/null || true
gtk-update-icon-cache ~/.local/share/icons/hicolor 2>/dev/null || true
echo "Installed. Find Blockworld in your application menu, or run ~/.local/bin/blockworld"
INST
    cat > "$STAGE/uninstall.sh" <<'UNINST'
#!/usr/bin/env bash
# Removes what install.sh added. Saved worlds and settings are left alone.
rm -f ~/.local/bin/blockworld ~/.local/share/applications/blockworld.desktop \
  ~/.local/share/icons/hicolor/512x512/apps/blockworld.png
update-desktop-database ~/.local/share/applications 2>/dev/null || true
echo "Blockworld removed. Your saved worlds were not touched."
UNINST
    chmod +x "$STAGE/install.sh" "$STAGE/uninstall.sh" "$STAGE/blockworld"
    (cd dist && tar -czf "$PKG.tar.gz" "$PKG")
    OUT="$STAGE/blockworld"
    ;;
  Windows)
    if ! command -v go-winres >/dev/null 2>&1; then
      say "installing go-winres (embeds the icon into the .exe)"
      CGO_ENABLED=0 GOOS= GOARCH= go install github.com/tc-hib/go-winres@latest
      PATH="$PATH:$(go env GOPATH)/bin"
      export PATH
      need go-winres "go install github.com/tc-hib/go-winres@latest and add \$(go env GOPATH)/bin to PATH"
    fi
    PKG="blockworld-$VERSION-windows-$ARCH"
    rm -rf "dist/$PKG"
    mkdir -p "dist/$PKG"
    say "embedding icon and building"
    # Windows wants a purely numeric file version; a tag such as 1.2-beta keeps its number.
    NUMVER="$(printf '%s' "$VERSION" | sed 's/[^0-9.].*$//')"
    [ -n "$NUMVER" ] || NUMVER="0.0"
    go-winres simply --icon icon.png --product-name Blockworld --file-description "Blockworld" \
      --product-version "$NUMVER" --file-version "$NUMVER"
    if [ "$CROSS" = 1 ]; then
      GOOS=windows GOARCH=amd64 CC="$MINGW_CC" gobuild "dist/$PKG/blockworld.exe" "-H windowsgui"
    else
      gobuild "dist/$PKG/blockworld.exe" "-H windowsgui"
    fi
    rm -f ./*.syso
    rm -f "dist/$PKG.zip"
    if command -v zip >/dev/null 2>&1; then
      (cd dist && zip -qr "$PKG.zip" "$PKG")
    elif command -v powershell >/dev/null 2>&1; then
      powershell -NoProfile -Command "Compress-Archive -Force -Path 'dist/$PKG' -DestinationPath 'dist/$PKG.zip'"
    else
      fail "zip is required to write the package (install zip, or use build.ps1 on Windows)"
    fi
    OUT="dist/$PKG/blockworld.exe"
    ;;
esac

# ---------- checksums ----------
say "checksums"
(
  cd dist
  rm -f SHA256SUMS
  for f in *.zip *.tar.gz *.dmg; do
    [ -f "$f" ] || continue
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$f" >> SHA256SUMS
    elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$f" >> SHA256SUMS
    fi
  done
  if [ -f SHA256SUMS ]; then cat SHA256SUMS; fi
)

say "done"
ls -1 dist | sed 's/^/    dist\//'

# ---------- optional install ----------
if [ "$INSTALL" = 1 ]; then
  case "$OS" in
    Linux)
      say "installing for the current user"
      "$STAGE/install.sh"
      OUT="$HOME/.local/bin/blockworld"
      ;;
    macOS)
      say "installing to /Applications (falls back to ~/Applications)"
      if rm -rf /Applications/Blockworld.app 2>/dev/null && cp -R "$APP" /Applications/ 2>/dev/null; then
        OUT="/Applications/Blockworld.app"
      else
        mkdir -p ~/Applications
        rm -rf ~/Applications/Blockworld.app
        cp -R "$APP" ~/Applications/
        OUT="$HOME/Applications/Blockworld.app"
      fi
      echo "Installed $OUT"
      ;;
    *) say "--install is not automated for $OS; the package is in dist/" ;;
  esac
fi

if [ "$LAUNCH" = 1 ]; then
  if [ "$CROSS" = 1 ]; then
    say "--run skipped: a cross-compiled Windows build cannot run here"
  else
    say "launching"
    case "$OS" in
      macOS) open "$OUT" ;;
      *) "$OUT" & ;;
    esac
  fi
fi
