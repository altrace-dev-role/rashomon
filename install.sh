#!/bin/sh
# rashomon installer.
#
#   curl -fsSL https://raw.githubusercontent.com/altrace-dev-role/rashomon/main/install.sh | sh
#
# What this script will and will not do, stated up front because a script you
# pipe into a shell deserves to say so:
#
#   IT WILL   download a released archive, verify its SHA-256 against the
#             release's own checksums file, and place one binary in a directory
#             you own.
#   IT WILL   refuse to install anything it could not verify. There is no
#             --skip-checksum and no fallback path that installs unverified
#             bytes. A tool whose product is "say what you actually observed"
#             does not get to guess about its own payload.
#   IT WILL NOT use sudo, write outside the install directory, or touch your
#             Claude Code configuration. Installing rashomon records nothing;
#             only running `rashomon watch` yourself installs the recorders.
#   IT WILL NOT contact anything except the release host.
#
# Knobs, all optional:
#   RASHOMON_VERSION      tag to install, e.g. v0.1.0   (default: latest release)
#   RASHOMON_INSTALL_DIR  where to put the binary       (default: ~/.local/bin)
#   RASHOMON_REPO         owner/name of the release repo
#   RASHOMON_BASE_URL     full base URL for release downloads

set -eu

REPO="${RASHOMON_REPO:-altrace-dev-role/rashomon}"
BASE_URL="${RASHOMON_BASE_URL:-https://github.com/${REPO}/releases/download}"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"
INSTALL_DIR="${RASHOMON_INSTALL_DIR:-${HOME}/.local/bin}"

say()  { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
die()  { printf 'install: %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------- preflight --

need() { command -v "$1" >/dev/null 2>&1; }

if need curl; then
  fetch()      { curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"; }
  fetch_out()  { curl -fsSL --proto '=https' --tlsv1.2 "$1"; }
elif need wget; then
  fetch()      { wget -qO "$2" "$1"; }
  fetch_out()  { wget -qO- "$1"; }
else
  die "needs curl or wget on PATH"
fi

# Checksum verification is not optional, so the absence of a checksum tool is a
# hard stop rather than a warning that gets scrolled past.
if need shasum;      then sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
elif need sha256sum; then sha256() { sha256sum "$1" | cut -d' ' -f1; }
else die "needs shasum or sha256sum to verify the download; refusing to install unverified bytes"
fi

need tar || die "needs tar on PATH"

# ------------------------------------------------------------ os and arch ----

os=$(uname -s)
case "$os" in
  Darwin) OS=darwin ;;
  Linux)  OS=linux  ;;
  *) die "unsupported operating system: $os (this release ships macOS and Linux)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)  ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) die "unsupported architecture: $arch (this release ships amd64 and arm64)" ;;
esac

# -------------------------------------------------------------- version ------

if [ -n "${RASHOMON_VERSION:-}" ]; then
  TAG="$RASHOMON_VERSION"
else
  # Parse the tag out of the latest-release API without depending on jq.
  TAG=$(fetch_out "$API_URL" 2>/dev/null \
        | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
        | head -n 1) || true
  [ -n "${TAG:-}" ] || die "could not resolve the latest release of ${REPO}.
  If no release exists yet, build from source instead:
      go install github.com/${REPO}/cmd/rashomon@main"
fi

# goreleaser's archive names carry the version WITHOUT the leading v.
VERSION=${TAG#v}

ARCHIVE="rashomon_${VERSION}_${OS}_${ARCH}.tar.gz"
ARCHIVE_URL="${BASE_URL}/${TAG}/${ARCHIVE}"
SUMS_URL="${BASE_URL}/${TAG}/checksums.txt"

say "rashomon ${TAG}  ${OS}/${ARCH}"
say "  from ${ARCHIVE_URL}"

# ----------------------------------------------------------------- fetch -----

TMP=$(mktemp -d 2>/dev/null || mktemp -d -t rashomon)
# Clean up on every exit path, including an interrupt part-way through a
# download, so a failed install never leaves a half-written archive behind.
trap 'rm -rf "$TMP"' EXIT INT TERM HUP

fetch "$ARCHIVE_URL" "$TMP/$ARCHIVE" || die "download failed: $ARCHIVE_URL"
fetch "$SUMS_URL"    "$TMP/checksums.txt" || die "could not fetch checksums.txt; refusing to install unverified bytes"

# --------------------------------------------------------------- verify -----

want=$(grep -E "[[:space:]]\*?${ARCHIVE}\$" "$TMP/checksums.txt" | cut -d' ' -f1 | head -n 1) || true
[ -n "${want:-}" ] || die "checksums.txt names no entry for ${ARCHIVE}; refusing to install"

got=$(sha256 "$TMP/$ARCHIVE")

if [ "$want" != "$got" ]; then
  die "CHECKSUM MISMATCH for ${ARCHIVE}
  expected ${want}
  got      ${got}
  Nothing was installed. Treat this as a compromised or corrupted download."
fi

say "  sha256 verified (${got})"

# -------------------------------------------------------------- install -----

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" || die "could not extract $ARCHIVE"
[ -f "$TMP/rashomon" ] || die "archive did not contain a rashomon binary"

mkdir -p "$INSTALL_DIR" || die "could not create $INSTALL_DIR"

# Install via a temporary name and rename, so an interrupted copy cannot leave a
# truncated binary at a path Claude Code may already be configured to execute.
cp "$TMP/rashomon" "$INSTALL_DIR/.rashomon.incoming" || die "could not write to $INSTALL_DIR"
chmod 0755 "$INSTALL_DIR/.rashomon.incoming"
mv "$INSTALL_DIR/.rashomon.incoming" "$INSTALL_DIR/rashomon"

say "  installed ${INSTALL_DIR}/rashomon"

# One binary, whatever else the release carries. Nothing here fetches a second
# program: a release asset this script does not know about is not one it
# installs, so what gets placed on this machine is decided by this file and
# not by what happens to be attached to a tag.

# ----------------------------------------------------------------- next ------

say ""
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) warn "NOTE: ${INSTALL_DIR} is not on your PATH. Add it:"
     warn "    export PATH=\"${INSTALL_DIR}:\$PATH\""
     warn "" ;;
esac

say "Next:"
say "    rashomon watch          # install the recorders into your Claude Code settings"
say "    claude                  # work normally"
say "    rashomon report         # read the session back"
say "    rashomon detach         # remove the recorders"
say ""
say "Network destinations are not observed in this alpha. Declarations and"
say "executions are recorded normally, and every report says destinations were"
say "not observed rather than implying none occurred."
