#!/bin/sh
# rashomon-sessionstart.sh -- an OPTIONAL SessionStart hook: one line about
# recorder state in the session's context, so the assistant can offer
# /rashomon when the session is not being recorded. Read-only: it runs
# `rashomon status`, creates no store, writes nothing, and always exits 0.
#
# This repository does not install it. To have the line in your own sessions,
# add the entry to your user settings (~/.claude/settings.json) with the
# absolute path of your checkout; nothing in this repository writes it there:
#
#   "hooks": {
#     "SessionStart": [
#       {
#         "matcher": "startup|resume",
#         "hooks": [
#           {
#             "type": "command",
#             "command": "sh /absolute/path/to/rashomon/scripts/rashomon-sessionstart.sh",
#             "timeout": 5
#           }
#         ]
#       }
#     ]
#   }
#
# stdout is injected into the model's context, so it is boring by design: one
# of eight lines, fixed apart from two counts, never a byte of the binary's
# own output. Keep it that way in review. Whatever status cannot establish is
# said as unknown and never rendered as "off": present, absent and
# unreadable-or-unknown entries are told apart, and a partial install is
# reported with its counts.

# Pinned locations win over PATH: this runs unattended, and PATH in a hook
# can carry a project-local impostor. Only absolute resolutions are used.
RASHOMON=""
if [ -n "${HOME:-}" ] && [ -x "${HOME:-}/.local/bin/rashomon" ]; then
	RASHOMON="$HOME/.local/bin/rashomon"
elif [ -n "${HOME:-}" ] && [ -x "${HOME:-}/go/bin/rashomon" ]; then
	RASHOMON="$HOME/go/bin/rashomon"
elif [ -x /opt/homebrew/bin/rashomon ]; then
	RASHOMON=/opt/homebrew/bin/rashomon
elif [ -x /usr/local/bin/rashomon ]; then
	RASHOMON=/usr/local/bin/rashomon
elif command -v rashomon >/dev/null 2>&1; then
	RASHOMON="$(command -v rashomon)"
	case "$RASHOMON" in /*) ;; *) RASHOMON="" ;; esac
fi

if [ -z "$RASHOMON" ]; then
	# No binary where this hook looks is not "not recorded": watch installs
	# entries by absolute path, and a binary kept anywhere else still fires.
	echo "rashomon: no binary found in ~/.local/bin, ~/go/bin, the Homebrew bins, or on PATH, so recorder state is unknown from here -- an install kept elsewhere could still be watching. If the user wants recording, /rashomon builds and installs it."
	exit 0
fi

# stdin closed so a binary that prompts cannot stall the hook until its
# timeout; stderr dropped so the injected line stays the only output; the
# output of a failed run is discarded rather than half-read.
out=$("$RASHOMON" status </dev/null 2>/dev/null) || out=""
if [ -z "$out" ]; then
	# "Could not read" is not "off": never render unknown as a definite no.
	echo "rashomon: binary present but its status could not be read, so recorder state is unknown -- not known to be recording, and not known to be off."
	exit 0
fi

# status prints one line per event, "  <Event>: <state>", with the state one
# of present, absent, unreadable, unknown, and a "hooks:" line saying whether
# an entry would run at all in this directory. The event names anchor the
# match, so the store's own "  present: yes" line is never counted as one.
#
# Deliberately five, not seven: Stop and StopFailure drive the exception-only
# recap, not recording, and status now prints a "recap:" line of its own for
# them. This script answers one question -- is this session's activity being
# recorded -- and widening the match here would fold a second, unrelated
# question ("has a Stop actually run") into the same present/absent count,
# which is the wrong axis for it.
events='PreToolUse|PostToolUse|PostToolUseFailure|SessionStart|SessionEnd'
count() { printf '%s\n' "$out" | grep -c -E "^  ($events): $1\$"; }
lines=$(count '[a-z]+')
present=$(count present)
absent=$(count absent)
unread=$(count '(unreadable|unknown)')

if [ "$lines" -ne 5 ]; then
	echo "rashomon: status output is not in the form this hook reads (the five entry lines are missing; the binary and this script may be from different versions), so recorder state is unknown. /rashomon-status shows the output itself."
elif ! printf '%s\n' "$out" | grep -q '^hooks: enabled'; then
	echo "rashomon: status cannot say whether hooks run in this directory (its hooks line reads disabled or unknown), so recorder state is unknown -- not known to be recording, and not known to be off. /rashomon-status shows the output itself."
elif [ "$unread" -gt 0 ]; then
	echo "rashomon: $unread of the 5 recorder entries read unreadable or unknown (no store yet, or a settings file that could not be read), so recorder state is unknown -- not known to be recording, and not known to be off. /rashomon-status shows which; /rashomon installs recording if the user wants it."
elif [ "$present" -eq 5 ]; then
	# The claim stops at "entries installed": whether records actually land
	# is proven per run by the report, never asserted here.
	echo "rashomon: recorder entries installed and watching. Whether records are landing is proven per run -- /rashomon-report shows it."
elif [ "$absent" -eq 5 ]; then
	echo "rashomon: binary present but not watching (all 5 recorder entries absent), so this session is not recorded. Offer /rashomon if the user wants recording."
else
	echo "rashomon: $present of the 5 recorder entries present and $absent absent, so this session's recording is incomplete. /rashomon re-runs watch, which installs the missing entries."
fi
exit 0
