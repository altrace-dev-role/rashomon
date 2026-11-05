#!/bin/sh
# wrap.sh install|restore: put the arm's hooks into RK_CONFIG_DIR/settings.json for one run, and put it back.
#   install  backs settings.json up to RK_RUN_DIR; runs `RK_BIN watch --yes` (the real install path: entries
#            naming --install <id>, the store at RASHOMON_HOME) and `RK_BIN consent --yes` (record scope);
#            then prefixes every entry of ours with hook.sh so each event's verb, exit code, stdout and
#            milliseconds land in RK_RUN_DIR/hooks.log and hook-ms.log
#   restore  copies the backup back (run.sh traps this on exit)
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
S=$RK_CONFIG_DIR/settings.json
case "${1:?install|restore}" in
install)
  if [ -f "$S" ]; then cp "$S" "$RK_RUN_DIR/settings.bak.json"; else echo '{}' > "$RK_RUN_DIR/settings.bak.json"; fi
  env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR=$RK_CONFIG_DIR "$RK_BIN" watch --yes > "$RK_RUN_DIR/watch.txt" 2>&1 || { echo "wrap.sh: watch --yes failed:"; cat "$RK_RUN_DIR/watch.txt"; exit 1; }
  env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u AI_AGENT CLAUDE_CONFIG_DIR=$RK_CONFIG_DIR "$RK_BIN" consent --yes > "$RK_RUN_DIR/consent.txt" 2>&1 || { echo "wrap.sh: consent --yes failed:"; cat "$RK_RUN_DIR/consent.txt"; exit 1; }
  cp "$S" "$RK_RUN_DIR/settings.installed.json"
  python3 - "$S" "$RK_BIN" "$HERE/hook.sh" <<'PY'
import json, sys
import os
path, binary, hook = sys.argv[1:4]
doc = json.load(open(path))
n = 0
# watch writes the executable's real path (/tmp is /private/tmp on macOS), bare or quoted
forms = []
for b in {binary, os.path.realpath(binary)}:
    forms += [b + " ", "'" + b + "' ", '"' + b + '" ']
for event, entries in (doc.get("hooks") or {}).items():
    for entry in entries:
        for h in entry.get("hooks", []):
            cmd = h.get("command", "")
            for form in forms:
                if cmd.startswith(form):
                    h["command"] = hook + " " + cmd[len(form):]
                    n += 1
                    break
json.dump(doc, open(path, "w"), indent=2)
print("wrap.sh: wrapped", n, "hook entries")
PY
  ;;
restore)
  cp "$RK_RUN_DIR/settings.bak.json" "$S" && echo "wrap.sh: settings.json restored"
  ;;
esac
