# The live hook-contract checks: what they keep, what they print, and what
# they leave. The recorder and the summaries are the one reader of what a real
# session sends (test/live/hc_shape.py); the script is the one writer of
# scratch and the one place the user's settings file is checked for.
m("H-108 the recorder keeps a raw identifier", "test/live/hc_shape.py",
  "    return hashlib.sha256(v.encode(\"utf-8\")).hexdigest()[:16]",
  "    return v",
  "TestH108_TheRecorderKeepsNoValue")
m("H-108 the recorder keeps a failure's error text", "test/live/hc_shape.py",
  "\"error\": error_shape(p.get(\"error\"), os.environ.get(\"HC_MARKER\", \"\")),",
  "\"error\": p.get(\"error\"),",
  "TestH108_TheRecorderKeepsNoValue")
m("H-108 the recorder keeps a key name it does not recognise", "test/live/hc_shape.py",
  "\"keys\": sorted(k if KEY.match(k) else \"<other>\" for k in p),",
  "\"keys\": sorted(p),",
  "TestH108_TheRecorderKeepsNoValue")
m("H-108 the hook summary prints a hook's stdout", "test/live/hc_shape.py",
  "                size(e.get(\"stdout\")), size(e.get(\"stderr\"))))",
  "                e.get(\"stdout\"), size(e.get(\"stderr\"))))",
  "TestH108_TheStreamSummariesPrintNoValue")
m("H-108 the script runs over an existing user settings file", "test/live/hc-items.sh",
  "if [ -e \"$CFG/settings.json\" ]; then",
  "if false; then",
  "TestH108_RefusesAMachineWithAUserSettingsFile")
m("H-108 the script does not record the claude version", "test/live/hc-items.sh",
  "echo \"claude --version: $(claude --version 2> /dev/null | head -n 1)\"\n",
  "",
  "TestH108_RecordsTheVersionAndLeavesNothingBehind")
m("H-108 the script leaves its scratch behind", "test/live/hc-items.sh",
  "  rm -rf \"$T\"\n",
  "  :\n",
  "TestH108_RecordsTheVersionAndLeavesNothingBehind")
