#!/usr/bin/env bash
# Bundles a portable Tesseract OCR install inside the .app bundle (see
# app.go's ocrPaths()), then zips the .app. Requires: brew (tesseract),
# otool (Xcode CLT) - both preinstalled on GitHub's macos-latest runners.
# Mirrors scripts/package-windows-release.sh and package-linux-release.sh.
set -euo pipefail

APP_DIR="build/bin/FlipMuncher.app"
MACOS_DIR="$APP_DIR/Contents/MacOS"

# --- Tesseract OCR (see app.go's ocrPaths) ----------------------------
# NOTE: brew on macos-latest (arm64) yields an arm64-only tesseract, while the
# app itself is darwin/universal. OCR therefore works on Apple Silicon only;
# on Intel Macs the bundled binary cannot run and the app reports OCR as
# unavailable (PDF download is unaffected). A lipo'd universal build would
# need an x86_64 brew under Rosetta and was judged too risky for CI blind.
# Homebrew formula/data versions are not pinned (brew has no stable pin/checksum).
echo "Installing Tesseract via brew to harvest a portable copy..."
brew install tesseract

OCR_DIR="$MACOS_DIR/ocr"
mkdir -p "$OCR_DIR/tessdata"
TESS_BIN="$(command -v tesseract)"
cp "$TESS_BIN" "$OCR_DIR/tesseract"
TESSDATA_SRC="$(brew --prefix tesseract)/share/tessdata/eng.traineddata"
cp "$TESSDATA_SRC" "$OCR_DIR/tessdata/"

# Walk otool -L's dependency list, keep only Homebrew-provided dylibs
# (system /usr/lib and /System frameworks are present on every Mac).
# macOS ships bash 3.2, so this avoids bash4-only associative arrays /
# negative indices and tracks "seen" paths in a plain newline list.
seen="$TESS_BIN"$'\n'
queue=("$TESS_BIN")
while [ ${#queue[@]} -gt 0 ]; do
  f="${queue[0]}"
  queue=("${queue[@]:1}")
  for dep in $(otool -L "$f" 2>/dev/null | tail -n +2 | awk '{print $1}' | grep -E '^(/usr/local|/opt/homebrew)'); do
    if ! grep -qxF "$dep" <<<"$seen"; then
      seen+="$dep"$'\n'
      queue+=("$dep")
      cp -f "$dep" "$OCR_DIR/"
    fi
  done
done

# Absolute Homebrew paths (e.g. /opt/homebrew/...) won't exist on a
# machine without Homebrew; rewrite every dylib reference to point at
# the copies bundled alongside tesseract itself (tesseract is run as
# its own process, so @executable_path is $OCR_DIR at runtime).
for bin in "$OCR_DIR/tesseract" "$OCR_DIR"/*.dylib; do
  [ -f "$bin" ] || continue
  for dep in $(otool -L "$bin" 2>/dev/null | tail -n +2 | awk '{print $1}' | grep -E '^(/usr/local|/opt/homebrew)'); do
    install_name_tool -change "$dep" "@executable_path/$(basename "$dep")" "$bin"
  done
done

# install_name_tool invalidates signatures (mandatory on Apple Silicon):
# ad-hoc re-sign every bundled Mach-O, then the whole .app (inside-out).
for bin in "$OCR_DIR"/*.dylib "$OCR_DIR/tesseract"; do
  [ -f "$bin" ] && codesign --force --sign - "$bin"
done
codesign --force --deep --sign - "$APP_DIR"
codesign --verify --deep --strict "$APP_DIR"
echo "Bundled Tesseract OCR into $OCR_DIR ($(du -sh "$OCR_DIR" | cut -f1))"

# --- Package -----------------------------------------------------------
cd build/bin
zip -qr "../../FlipMuncher-macos.zip" FlipMuncher.app
cd - >/dev/null
echo "Wrote FlipMuncher-macos.zip ($(du -sh FlipMuncher-macos.zip | cut -f1))"
