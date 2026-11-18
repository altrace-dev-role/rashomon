#!/bin/sh
# Builds the plugin's recorders -- one per platform, named the way
# plugin/bin/rashomon looks for them -- and the platform graph's writer the same
# way for plugin/bin/pgraph (from the pgraph module, which rashomon never
# imports), and packs plugin/ into dist/rashomon-plugin.zip, the archive
# .claude-plugin/marketplace.json installs.
#
#   scripts/build-plugin.sh [version [zip path]]
#
# The zip path defaults to dist/rashomon-plugin.zip; a relative one is read
# from the repository root.
#
# macOS and Linux only. Every hook runs plugin/bin/rashomon, a POSIX shell
# launcher, and Windows has no shell to run it; the settings install (`watch`)
# is the Windows path.
set -eu
version=${1:-dev}
out=${2:-dist/rashomon-plugin.zip}
cd "$(dirname "$0")/.."
# The version given must be the one plugin.json names: Claude Code caches an
# installed plugin under a directory named by that version, and the recorders
# report the version given here. "dev", the default, is a local build.
want=$(sed -n 's/^  "version": "\([^"]*\)",$/\1/p' plugin/.claude-plugin/plugin.json)
if [ -z "$want" ]; then
	echo "build-plugin.sh: plugin/.claude-plugin/plugin.json names no version" >&2
	exit 1
fi
if [ "$version" != dev ] && [ "$version" != "$want" ]; then
	echo "build-plugin.sh: version $version is not $want, the version plugin.json names" >&2
	exit 1
fi
rm -f plugin/bin/rashomon-* plugin/bin/pgraph-*
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
	os=${target%/*}
	arch=${target#*/}
	echo "building $os/$arch"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" \
		-o "plugin/bin/rashomon-$os-$arch" ./cmd/rashomon
	(cd pgraph && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w" -o "../plugin/bin/pgraph-$os-$arch" ./cmd/pgraph)
done
mkdir -p "$(dirname "$out")"
rm -f "$out"
# zip runs inside plugin/, so a relative path is made absolute first.
case $out in /*) abs=$out ;; *) abs=$PWD/$out ;; esac
# -X leaves out extra attributes, and zip still stores the Unix modes, so an
# extractor that honours them keeps the launcher and recorders executable.
# The launcher also restores the recorder's bit itself, for one that does not.
(cd plugin && zip -qrX "$abs" . -x 'bin/.gitkeep')
echo "wrote $out"
