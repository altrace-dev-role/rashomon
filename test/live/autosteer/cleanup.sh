#!/bin/sh
# cleanup.sh NAME: remove a judged run's scratch worktree by exact path. The run directory's other files stay.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
RK_REPO=${RK_REPO:-$(cd "$HERE/../../.." && pwd)}
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
D=$RK_RUNS/${1:?NAME}
[ -d "$D/tree" ] || { echo "cleanup.sh: no tree under $D" >&2; exit 2; }
git -C "$RK_REPO" worktree remove --force "$D/tree" && echo "removed $D/tree"
