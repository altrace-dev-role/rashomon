#!/bin/sh
# SessionStart hook for this repository: put one line about recorder state
# into the session's context, so the assistant can offer /rashomon when the
# session is not being recorded. Read-only, no store created, always exit 0.

RASHOMON=""
if command -v rashomon >/dev/null 2>&1; then
	RASHOMON=$(command -v rashomon)
elif [ -x "$HOME/.local/bin/rashomon" ]; then
	RASHOMON="$HOME/.local/bin/rashomon"
fi

if [ -z "$RASHOMON" ]; then
	echo "rashomon: no binary on this machine, so this session is not recorded. If the user wants recording, the rashomon skill builds and installs it (/rashomon)."
	exit 0
fi

if "$RASHOMON" status 2>/dev/null | grep -q '^  PreToolUse: present'; then
	echo "rashomon: recorder installed and watching -- this session's tool calls are being recorded. /rashomon-report renders them."
else
	echo "rashomon: binary present but not watching, so this session is not recorded. Offer /rashomon if the user wants recording."
fi
exit 0
