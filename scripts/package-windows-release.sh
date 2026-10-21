#!/usr/bin/env bash
# Bundles portable, no-install, no-admin dependencies next to the built
# exe: the Fixed Version WebView2 Runtime (see main.go's
# webview2RuntimeDir()) and a minimal Tesseract OCR install (see
# app.go's ocrPaths()). Requires: curl, unzip, 7z (p7zip-full), objdump
# (binutils) - all preinstalled on GitHub's ubuntu-latest runners.
set -euo pipefail

WEBVIEW2_VERSION="154.0.4258.37"
TESSERACT_VERSION="5.4.0.20240606"
# sha256 of the pinned downloads (verified when pinned; bump together with versions).
SHA_WV2_RUNTIME="9859e988d3a949ae0dde73981e08cc4a5b7ff1be1482c49aa1ce35841980ffe2"
SHA_WV2_CORE="047f0283e7d93d1f3118d958275e216706ba7848b1582536fc405b88b3339495"
SHA_TESSERACT="c885fff6998e0608ba4bb8ab51436e1c6775c2bafc2559a19b423e18678b60c9"
BIN_DIR="build/bin"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

# --- WebView2 (see main.go's webview2RuntimeDir) ----------------------
# The runtime binaries are Microsoft's, redistributed via the
# WebView2.Runtime.X64(.Core) NuGet packages (github.com/ProKn1fe/WebView2.Runtime)
# since Microsoft's own download page has no stable automatable URL.
RUNTIME_DIR="$BIN_DIR/webview2runtime"
for pkg in "webview2.runtime.x64" "webview2.runtime.x64.core"; do
  case "$pkg" in webview2.runtime.x64) want="$SHA_WV2_RUNTIME" ;; *) want="$SHA_WV2_CORE" ;; esac
  echo "Downloading $pkg $WEBVIEW2_VERSION..."
  curl -fsSL -o "$WORK_DIR/$pkg.nupkg" \
    "https://api.nuget.org/v3-flatcontainer/$pkg/$WEBVIEW2_VERSION/$pkg.$WEBVIEW2_VERSION.nupkg"
  echo "$want  $WORK_DIR/$pkg.nupkg" | sha256sum -c -
  unzip -q "$WORK_DIR/$pkg.nupkg" -d "$WORK_DIR/$pkg"
done
mkdir -p "$RUNTIME_DIR"
cp -r "$WORK_DIR/webview2.runtime.x64/contentFiles/any/any/WebView2/." "$RUNTIME_DIR/"
cp -r "$WORK_DIR/webview2.runtime.x64.core/contentFiles/any/any/WebView2/." "$RUNTIME_DIR/"
echo "Bundled WebView2 runtime into $RUNTIME_DIR ($(du -sh "$RUNTIME_DIR" | cut -f1))"

# --- Tesseract OCR (see app.go's ocrPaths) ----------------------------
# Uses the UB-Mannheim Windows build (Apache-2.0, github.com/UB-Mannheim/tesseract),
# extracting the NSIS installer with 7z instead of running it, so no
# Windows machine or admin rights are needed to produce the bundle.
# Only tesseract.exe, its actual runtime DLLs (resolved by walking PE
# imports - the installer also ships training tools, docs and debug
# viewers we don't need), and the English trained-data file are kept.
OCR_DIR="$BIN_DIR/ocr"
TESS_SETUP_URL="https://digi.bib.uni-mannheim.de/tesseract/tesseract-ocr-w64-setup-$TESSERACT_VERSION.exe"
echo "Downloading Tesseract $TESSERACT_VERSION..."
curl -fsSL -o "$WORK_DIR/tesseract-setup.exe" "$TESS_SETUP_URL"
echo "$SHA_TESSERACT  $WORK_DIR/tesseract-setup.exe" | sha256sum -c -
mkdir -p "$WORK_DIR/tesseract"
7z x -o"$WORK_DIR/tesseract" -y "$WORK_DIR/tesseract-setup.exe" >/dev/null

mkdir -p "$OCR_DIR/tessdata"
cp "$WORK_DIR/tesseract/tesseract.exe" "$OCR_DIR/"
cp "$WORK_DIR/tesseract/tessdata/eng.traineddata" "$OCR_DIR/tessdata/"

# Walk the PE import table to find exactly the DLLs tesseract.exe needs
# at runtime (the installer also ships DLLs only used by its training
# tools, which we don't bundle).
declare -A needed=()
queue=("tesseract.exe")
while [ ${#queue[@]} -gt 0 ]; do
  f="${queue[-1]}"
  unset 'queue[-1]'
  [ -n "${needed[$f]:-}" ] && continue
  needed[$f]=1
  src="$WORK_DIR/tesseract/$f"
  [ -f "$src" ] || continue
  for dep in $(objdump -p "$src" 2>/dev/null | awk '/DLL Name:/{print $3}'); do
    match=$(ls "$WORK_DIR/tesseract" | grep -i "^${dep}$" || true)
    [ -n "$match" ] && [ -z "${needed[$match]:-}" ] && queue+=("$match")
  done
done
for f in "${!needed[@]}"; do
  [[ "$f" == *.dll ]] && cp "$WORK_DIR/tesseract/$f" "$OCR_DIR/"
done
echo "Bundled Tesseract OCR into $OCR_DIR ($(du -sh "$OCR_DIR" | cut -f1))"

# --- Package -----------------------------------------------------------
cd "$BIN_DIR"
zip -qr "../../FlipMuncher-portable.zip" FlipMuncher.exe webview2runtime ocr
# Lite: no bundled WebView2 (main.go falls back to the system one, which
# Windows 11 and up-to-date Windows 10 already ship).
zip -qr "../../FlipMuncher-windows-lite.zip" FlipMuncher.exe ocr
cd - >/dev/null

echo "Wrote FlipMuncher-portable.zip ($(du -sh FlipMuncher-portable.zip | cut -f1)) and FlipMuncher-windows-lite.zip ($(du -sh FlipMuncher-windows-lite.zip | cut -f1))"
