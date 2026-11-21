#!/bin/sh
# Reads the members of what a release ships, before it is published. Each of
# the six archives GoReleaser writes into DIST must carry rashomon and pgraph
# (rashomon.exe and pgraph.exe on windows), LICENSE and THIRD_PARTY_NOTICES,
# and the plugin archive must carry bin/rashomon-<os>-<arch> and
# bin/pgraph-<os>-<arch> for each of its four platforms beside the same two
# files.
#
#   scripts/check-release-archives.sh DIST PLUGIN_ZIP
#
# Each missing member is named on a line of its own, and the exit is 1.
# release.yaml runs this between GoReleaser's draft and publishing, so a
# release that fails it stays a draft; ci.yaml's release-dry-run job runs it
# on a snapshot.
set -eu
if [ $# -ne 2 ]; then
	echo "usage: scripts/check-release-archives.sh DIST PLUGIN_ZIP" >&2
	exit 2
fi
dist=$1
plugin=$2
failed=0

# members ARCHIVE prints the names of the archive's members.
members() {
	case $1 in
	*.zip) unzip -Z1 "$1" ;;
	*) tar -tzf "$1" ;;
	esac
}

# need LISTING MEMBER ARCHIVE names MEMBER when ARCHIVE's LISTING lacks it.
need() {
	if ! printf '%s\n' "$1" | grep -qxF -e "$2" -e "./$2"; then
		echo "check-release-archives.sh: $3 has no $2" >&2
		failed=1
	fi
}

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	os=${target%/*}
	arch=${target#*/}
	ext=tar.gz
	exe=
	if [ "$os" = windows ]; then
		ext=zip
		exe=.exe
	fi
	# One archive per target: none, or two left from another version, fails.
	set -- "$dist"/rashomon_*_"$os"_"$arch"."$ext"
	if [ $# -ne 1 ] || [ ! -f "$1" ]; then
		echo "check-release-archives.sh: $dist has no single rashomon_*_${os}_${arch}.$ext archive" >&2
		failed=1
		continue
	fi
	archive=$1
	name=${archive##*/}
	if ! list=$(members "$archive"); then
		echo "check-release-archives.sh: $name cannot be read" >&2
		failed=1
		continue
	fi
	for member in "rashomon$exe" "pgraph$exe" LICENSE THIRD_PARTY_NOTICES; do
		need "$list" "$member" "$name"
	done
	echo "read $name"
done

name=${plugin##*/}
if [ ! -f "$plugin" ]; then
	echo "check-release-archives.sh: there is no plugin archive at $plugin" >&2
	failed=1
elif ! list=$(unzip -Z1 "$plugin"); then
	echo "check-release-archives.sh: $name cannot be read" >&2
	failed=1
else
	for pair in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64; do
		need "$list" "bin/rashomon-$pair" "$name"
		need "$list" "bin/pgraph-$pair" "$name"
	done
	need "$list" LICENSE "$name"
	need "$list" THIRD_PARTY_NOTICES "$name"
	echo "read $name"
fi

if [ "$failed" -ne 0 ]; then
	echo "check-release-archives.sh: a member named above is missing, so the release is not ready to publish" >&2
	exit 1
fi
echo "every archive carries what it ships"
