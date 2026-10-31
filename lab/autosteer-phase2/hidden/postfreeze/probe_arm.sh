#!/bin/sh
# Post-freeze probes for one arm at one commit, on both interpreter sides, in the arm's own steerci image.
# Usage: sh probe_arm.sh <arm> <ref> [battery]
#   coding probes (tree: hex, zlib, base64, rot13, utf-16, punycode), nesting and rot13 (tree2: nest60, nest95, rot);
#   with "battery", also the second opinion's 43-case battery into extra/arm-<arm>-<side>.json (+ a copy stamped
#   with the commit), scored by extra/score_extra.py. Oracles: ORACLE.txt and extra/oracle-<version>.json.
# Same commands as the 00:18, 00:24 and 01:10 runs, moved from the transcript into a file (checklist 11a).
set -eu
arm=$1; ref=$2; battery=${3:-}
P=$HOME/.k-hidden-accept/phase2/postfreeze
repo=$HOME/rashomon-eval/py27/$arm/py2scan
head=$(git -C "$repo" rev-parse --short "$ref")
for side in 2.7 3.14; do
  img=$(docker images --format '{{.Repository}}:{{.Tag}}' | grep "^steerci/$arm-$side:" | head -1)
  tmp=$(mktemp -d)
  git -C "$repo" archive --prefix=src/ "$ref" | tar -xf - -C "$tmp"
  cp -R "$P/tree" "$P/tree2" "$P/run_arm.sh" "$P/run_arm2.sh" "$P/extra/hostile_extra.sh" "$tmp/"
  cp "$P/extra/run_arm.sh" "$tmp/run_extra.sh"
  for s in run_arm.sh run_arm2.sh; do
    echo "== $arm $head $side $s"
    tar -cf - -C "$tmp" . | docker run -i --rm --label steerci-role=probe --network none --user 1000:1000 \
      -e HOME=/tmp "$img" sh -c "mkdir -p /tmp/in && tar -xf - -C /tmp/in && sh /tmp/in/$s" 2>&1 | tail -8
  done
  # item 2b on the probes (added 02:10, after S's utf-16 message differed by host unseen since 00:18): hash each JSON
  tar -cf - -C "$tmp" . | docker run -i --rm --label steerci-role=probe --network none --user 1000:1000 \
    -e HOME=/tmp "$img" sh -c 'mkdir -p /tmp/in && tar -xf - -C /tmp/in && cd /tmp/in/src && for c in /tmp/in/tree/* /tmp/in/tree2/*; do
      timeout 60 python -m py2scan scan --format json "$c" > /tmp/o.json 2>/dev/null; echo "${c#/tmp/in/} rc=$? $(sha256sum < /tmp/o.json | cut -c1-16)"; done' \
    > "$P/extra/hash-$arm-$head-$side.txt"
  if [ "$battery" = battery ]; then
    out=$P/extra/arm-$arm-$side
    tar -cf - -C "$tmp" . | docker run -i --rm --label steerci-role=probe --network none --user 1000:1000 \
      -e HOME=/tmp "$img" sh -c 'mkdir -p /tmp/in && tar -xf - -C /tmp/in && sh /tmp/in/run_extra.sh' > "$out.json" 2> "$out.err"
    cp "$out.json" "$P/extra/arm-$arm-$head-$side.json"
    echo "$head" > "$P/extra/arm-$arm.head"
    echo "== $arm $head $side battery: $(wc -c < "$out.json") bytes"
  fi
  rm -rf "$tmp"
done
a=$P/extra/hash-$arm-$head-2.7.txt; b=$P/extra/hash-$arm-$head-3.14.txt
n=$(wc -l < "$a" | tr -d ' '); d=$(diff "$a" "$b" | grep '^<' | awk '{print $2}' | tr '\n' ' ')
echo "== $arm $head item 2b on the $n probes: $(diff "$a" "$b" | grep -c '^<') differ between 2.7 and 3.14 (JSON bytes or exit code): ${d:--}"
