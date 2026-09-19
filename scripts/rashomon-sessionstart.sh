#!/bin/sh
# SessionStart hook for this repository: put one line about recorder state
# into the session's context, so the assistant can offer /rashomon when the
# session is not being recorded. Read-only, no store created, always exit 0.
#
# This script's stdout is injected into the model's context. It is boring by
# design -- it emits one of four fixed lines and never forwards a byte of the
# binary's own output. Keep it that way in review: any change here changes
# what every contributor's session is told.

# Pinned locations win over PATH: this runs unattended, and PATH in a hook
# can carry a project-local impostor. Only absolute resolutions are used.
RASHOMON=""
if [ -n "${HOME:-}" ] && [ -x "${HOME:-}/.local/bin/rashomon" ]; then
	RASHOMON="$HOME/.local/bin/rashomon"
elif [ -n "${HOME:-}" ] && [ -x "${HOME:-}/go/bin/rashomon" ]; then
	RASHOMON="$HOME/go/bin/rashomon"
elif command -v rashomon >/dev/null 2>&1; then
	RASHOMON="$(command -v rashomon)"
	case "$RASHOMON" in /*) ;; *) RASHOMON="" ;; esac
fi

if [ -z "$RASHOMON" ]; then
	echo "rashomon: no binary on this machine, so this session is not recorded. If the user wants recording, the rashomon skill builds and installs it (/rashomon)."
	exit 0
fi

# stdin closed so a binary that prompts cannot stall the hook until its
# timeout; stderr dropped so the injected line stays the only output.
out=$("$RASHOMON" status </dev/null 2>/dev/null)
if [ $? -ne 0 ] || [ -z "$out" ]; then
	# "Could not read" is not "off": never render unknown as a definite no.
	echo "rashomon: binary present but its status could not be read, so recorder state is unknown -- not known to be recording, and not known to be off."
	exit 0
fi

# Watching means all five of this install's entries read present, not just
# one. The claim stops at "entries installed": whether records actually land
# is proven per run by the report, never asserted here.
if [ "$(printf '%s\n' "$out" | grep -c ': present$')" -eq 5 ]; then
	echo "rashomon: recorder entries installed and watching. Whether records are landing is proven per run -- /rashomon-report shows it."
else
	echo "rashomon: binary present but not watching, so this session is not recorded. Offer /rashomon if the user wants recording."
fi
exit 0
