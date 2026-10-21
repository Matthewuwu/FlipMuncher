#!/usr/bin/env bash
# Bundles a portable Tesseract OCR install next to the built Linux binary
# (see app.go's ocrPaths()), then packages both a plain tar.gz and an
# AppImage. Requires: apt (tesseract-ocr), appimagetool, on PATH.
# Mirrors scripts/package-windows-release.sh's approach.
set -euo pipefail

BIN_DIR="build/bin"
BIN_NAME="FlipMuncher"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

# --- Tesseract OCR (see app.go's ocrPaths) ----------------------------
echo "Installing Tesseract via apt to harvest a portable copy..."
sudo apt-get update -qq
sudo apt-get install -y -qq tesseract-ocr
# Not pinned: the apt package/eng.traineddata come from the runner's Ubuntu
# repo (version follows the runner image); no stable checksum is practical.

OCR_DIR="$BIN_DIR/ocr"
mkdir -p "$OCR_DIR/tessdata"
TESS_BIN="$(command -v tesseract)"
cp "$TESS_BIN" "$OCR_DIR/tesseract"
cp /usr/share/tesseract-ocr/*/tessdata/eng.traineddata "$OCR_DIR/tessdata/"

# Bundle every shared library tesseract needs except the glibc core
# (libc, libm, ld-linux...), which must match the host's kernel/loader and
# is present everywhere. A name allowlist missed libgomp, libLerc and more,
# so OCR failed on machines that only have GTK3/WebKitGTK installed.
for lib in $(ldd "$TESS_BIN" | awk '/=>/ {print $3}' | grep -vE '/(libc|libm|libdl|librt|libpthread|libresolv|libutil)\.so'); do
  [ -f "$lib" ] && cp -fL "$lib" "$OCR_DIR/"
done

# Fail the build if the bundle is not self-contained: with only the bundled
# folder on the library path, every dependency must resolve.
if LD_LIBRARY_PATH="$OCR_DIR" ldd "$OCR_DIR/tesseract" | grep -q "not found"; then
  echo "ERROR: bundled tesseract has unresolved libraries:" >&2
  LD_LIBRARY_PATH="$OCR_DIR" ldd "$OCR_DIR/tesseract" | grep "not found" >&2
  exit 1
fi
echo "Bundled Tesseract OCR into $OCR_DIR ($(du -sh "$OCR_DIR" | cut -f1))"

# --- tar.gz --------------------------------------------------------------
cd "$BIN_DIR"
chmod +x "$BIN_NAME"
tar -czf "../../FlipMuncher-linux-portable.tar.gz" "$BIN_NAME" ocr
cd - >/dev/null
echo "Wrote FlipMuncher-linux-portable.tar.gz ($(du -sh FlipMuncher-linux-portable.tar.gz | cut -f1))"

# --- AppImage --------------------------------------------------------------
APPDIR="$WORK_DIR/FlipMuncher.AppDir"
mkdir -p "$APPDIR/usr/bin/ocr"
cp "$BIN_DIR/$BIN_NAME" "$APPDIR/usr/bin/"
cp -r "$BIN_DIR/ocr/." "$APPDIR/usr/bin/ocr/"
cp build/appicon.png "$APPDIR/flipmuncher.png"

cat > "$APPDIR/flipmuncher.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=FlipMuncher
Exec=FlipMuncher
Icon=flipmuncher
Categories=Utility;
EOF

cat > "$APPDIR/AppRun" <<'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/bin/FlipMuncher" "$@"
EOF
chmod +x "$APPDIR/AppRun"

appimagetool "$APPDIR" "FlipMuncher-x86_64.AppImage"
echo "Wrote FlipMuncher-x86_64.AppImage ($(du -sh FlipMuncher-x86_64.AppImage | cut -f1))"
