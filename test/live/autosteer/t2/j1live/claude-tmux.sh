#!/bin/sh
# claude-tmux.sh: J1-LIVE's RK_CLAUDE_BIN (j1live.sh exports it for run.sh). --version is answered by the pinned CLI
# (J1_REAL_CLAUDE), so run.sh's version check reads the real one; every other call goes to drive.py, which starts the
# real CLI interactively in tmux and drives it (drive.py's header).
case "${1:-}" in --version) exec "${J1_REAL_CLAUDE:?J1_REAL_CLAUDE unset}" --version;; esac
exec python3 -I -B "$(cd "$(dirname "$0")" && pwd)/drive.py" "$@"
