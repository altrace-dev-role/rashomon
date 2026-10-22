# The live hook-contract checks: what they keep, what they print, and what
# they leave. The recorder and the summaries are the one reader of what a real
# session sends (test/live/hc_shape.py); the script is the one writer of
# scratch and the one place the user's settings file is checked for.
m("H-115 the recorder keeps a raw identifier", "test/live/hc_shape.py",
  "    return hashlib.sha256(v.encode(\"utf-8\")).hexdigest()[:16]",
  "    return v",
  "TestH115_TheRecorderKeepsNoValue")
m("H-115 the recorder keeps a failure's error text", "test/live/hc_shape.py",
  "\"error\": error_shape(p.get(\"error\"), os.environ.get(\"HC_MARKER\", \"\")),",
  "\"error\": p.get(\"error\"),",
  "TestH115_TheRecorderKeepsNoValue")
m("H-115 the recorder keeps a key name it does not recognise", "test/live/hc_shape.py",
  "\"keys\": sorted(k if KEY.match(k) else \"<other>\" for k in p),",
  "\"keys\": sorted(p),",
  "TestH115_TheRecorderKeepsNoValue")
m("H-115 the hook summary prints a hook's stdout", "test/live/hc_shape.py",
  "                size(e.get(\"stdout\")), size(e.get(\"stderr\"))))",
  "                e.get(\"stdout\"), size(e.get(\"stderr\"))))",
  "TestH115_TheStreamSummariesPrintNoValue")
m("H-115 the script runs over an existing user settings file", "test/live/hc-items.sh",
  "if [ -e \"$CFG/settings.json\" ]; then",
  "if false; then",
  "TestH115_RefusesAMachineWithAUserSettingsFile")
m("H-115 the script does not record the claude version", "test/live/hc-items.sh",
  "echo \"claude --version: $(claude --version 2> /dev/null | head -n 1)\"\n",
  "",
  "TestH115_RecordsTheVersionAndLeavesNothingBehind")
m("H-115 the script leaves its scratch behind", "test/live/hc-items.sh",
  "  rm -rf \"$T\"\n",
  "  :\n",
  "TestH115_RecordsTheVersionAndLeavesNothingBehind")
# What a real session leaves outside the scratch, each written by the
# stand-in claude the way 2.1.305 wrote it.
m("H-115 the script leaves Claude Code's temporary files in the caller's TMPDIR", "test/live/hc-items.sh",
  "export TMPDIR=\"$T/tmp\"\n",
  "",
  "TestH115_RecordsTheVersionAndLeavesNothingBehind")
m("H-115 the script leaves a session's session-env directory", "test/live/hc-items.sh",
  "    rmdir \"$CFG/session-env/$s\" 2> /dev/null\n",
  "    :\n",
  "TestH115_RecordsTheVersionAndLeavesNothingBehind")
m("H-115 the script leaves a session's transcript", "test/live/hc-items.sh",
  "      rm -rf \"$f\" \"${f%.jsonl}\"; rmdir \"$(dirname \"$f\")\" 2> /dev/null\n",
  "      :\n",
  "TestH115_RecordsTheVersionAndLeavesNothingBehind")
m("H-115 a hangup skips the cleanup", "test/live/hc-items.sh",
  "trap 'exit 129' HUP\n",
  "",
  "TestH115_AHangupStillRemovesWhatTheRunLeft")
