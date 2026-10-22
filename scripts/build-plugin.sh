#!/bin/sh
# Builds the plugin's recorders -- one per platform, named the way
# plugin/bin/rashomon looks for them -- and packs plugin/ into
# dist/rashomon-plugin.zip, the archive .claude-plugin/marketplace.json installs.
#
#   scripts/build-plugin.sh [version]
#
# macOS and Linux only. Every hook runs plugin/bin/rashomon, a POSIX shell
# launcher, and Windows has no shell to run it; the settings install (`watch`)
# is the Windows path.
set -eu
version=${1:-dev}
cd "$(dirname "$0")/.."
rm -f plugin/bin/rashomon-*
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
	os=${target%/*}
	arch=${target#*/}
	echo "building $os/$arch"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" \
		-o "plugin/bin/rashomon-$os-$arch" ./cmd/rashomon
done
mkdir -p dist
rm -f dist/rashomon-plugin.zip
# -X leaves out extra attributes, and zip still stores the Unix modes, so an
# extractor that honours them keeps the launcher and recorders executable.
# The launcher also restores the recorder's bit itself, for one that does not.
(cd plugin && zip -qrX ../dist/rashomon-plugin.zip . -x 'bin/.gitkeep')
echo "wrote dist/rashomon-plugin.zip"
