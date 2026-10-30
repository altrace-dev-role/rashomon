# CANARY1: P-TRU-1's reader (test/live/p_tru_1.py) is the one place a hook's
# context is judged to have reached the model, and its script
# (test/live/p-tru-1.sh) the one place a live run's configuration, artifacts
# and scratch are guarded. The verified list in internal/steer/client.go
# grows only from what these two write.
m("canary1 the reader takes the hook's own record for delivery", "test/live/p_tru_1.py",
  "holds(e.get(\"rendered\"), context_token(",
  "holds(a.get(\"content\"), context_token(",
  "TestCANARY1_TheReaderReadsDeliveryFromWhatTheModelWasGiven")
m("canary1 the reader takes any attachment holding the token for delivery", "test/live/p_tru_1.py",
  "kind == \"hook_additional_context\" and holds(e.get(\"rendered\"), context_token(r[\"event\"], nonce))",
  "holds(e, context_token(r[\"event\"], nonce))",
  "TestCANARY1_TheReaderReadsDeliveryFromWhatTheModelWasGiven")
m("canary1 the reader matches a token of any nonce", "test/live/p_tru_1.py",
  "    return f\"PTRU-CONTEXT-{event}-{nonce}\"\n",
  "    return f\"PTRU-CONTEXT-{event}-\"\n",
  "TestCANARY1_TheReaderReadsDeliveryFromWhatTheModelWasGiven")
m("canary1 the reader passes a control the model did not repeat", "test/live/p_tru_1.py",
  "\"control_ok\": control[\"delivered\"] and control[\"echoed\"],",
  "\"control_ok\": control[\"delivered\"],",
  "TestCANARY1_TheReaderReadsDeliveryFromWhatTheModelWasGiven")
m("canary1 the reader exits 0 on a failed control", "test/live/p_tru_1.py",
  "    return 0 if art[\"control_ok\"] else 2\n",
  "    return 0\n",
  "TestCANARY1_TheReaderReadsDeliveryFromWhatTheModelWasGiven|TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the reader writes over an artifact", "test/live/p_tru_1.py",
  "    with open(args.out, \"x\", encoding=\"utf-8\") as f:\n",
  "    with open(args.out, \"w\", encoding=\"utf-8\") as f:\n",
  "TestCANARY1_TheReaderNeverWritesOverAnArtifact")
m("canary1 the reader guesses a version from a mixed transcript", "test/live/p_tru_1.py",
  "(versions[0], \"transcript\") if len(versions) == 1 else (None, None)",
  "(versions[0], \"transcript\") if versions else (None, None)",
  "TestCANARY1_TheReaderTakesAStreamlessVersionFromTheTranscript")
m("canary1 the settings check lets hooks through", "test/live/p_tru_1.py",
  "    if s.get(\"hooks\") or enabled:\n",
  "    if enabled:\n",
  "TestCANARY1_TheScriptRefusesBeforeAnySession")
m("canary1 the settings check refuses a plugin that is not enabled", "test/live/p_tru_1.py",
  "    enabled = any(plugins.values()) if isinstance(plugins, dict) else bool(plugins)\n",
  "    enabled = bool(plugins)\n",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script runs at ~/.claude", "test/live/p-tru-1.sh",
  "if [ \"$(cd \"$CFG\" && pwd -P)\" = \"$(cd \"$HOME\" && pwd -P)/.claude\" ]; then",
  "if false; then",
  "TestCANARY1_TheScriptRefusesBeforeAnySession")
m("canary1 the script skips the settings check", "test/live/p-tru-1.sh",
  "KEYS=$(python3 -I \"$HERE/p_tru_1.py\" settings \"$CFG/settings.json\")\n",
  "KEYS=null\n",
  "TestCANARY1_TheScriptRefusesBeforeAnySession|TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script starts a session over an existing artifact", "test/live/p-tru-1.sh",
  "  if [ -e \"$DIR/$a\" ]; then",
  "  if false; then",
  "TestCANARY1_TheScriptRefusesBeforeAnySession")
m("canary1 the script hands the sessions a relative config", "test/live/p-tru-1.sh",
  "CFG=$(cd \"$CFG\" && pwd)\n",
  "",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script runs the -p session outside the scratch config", "test/live/p-tru-1.sh",
  "(cd \"$T/work\" && CLAUDE_CONFIG_DIR=\"$CFG\" \"$CLAUDE_BIN\" -p",
  "(cd \"$T/work\" && \"$CLAUDE_BIN\" -p",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the owner's command runs outside the scratch config", "test/live/p-tru-1.sh",
  "CLAUDE_CONFIG_DIR=\"$CFG\" \"$CLAUDE_BIN\" --session-id \"$(cat \"$T/cli.sid\")\"",
  "\"$CLAUDE_BIN\" --session-id \"$(cat \"$T/cli.sid\")\"",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script reuses one nonce for both sessions", "test/live/p-tru-1.sh",
  "  python3 -I -c 'import secrets; print(secrets.token_hex(8))' > \"$T/$1.nonce\"\n",
  "  echo 0a1b2c3d4e5f6a7b > \"$T/$1.nonce\"\n",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script asks for the cli session after a failed control", "test/live/p-tru-1.sh",
  "if [ \"$rc\" -ne 0 ]; then\n  echo \"step 1 proved nothing",
  "if false; then\n  echo \"step 1 proved nothing",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script waits for the cli session without a limit", "test/live/p-tru-1.sh",
  "  if [ \"$waited\" -ge \"$WAIT\" ]; then",
  "  if false; then",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script leaves its scratch behind", "test/live/p-tru-1.sh",
  "trap 'rm -rf \"$T\"' EXIT\n",
  "trap ':' EXIT\n",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script does not record the version", "test/live/p-tru-1.sh",
  "echo \"claude --version: $(\"$CLAUDE_BIN\" --version)\"\n",
  "",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
m("canary1 the script swallows the reader's verdict", "test/live/p-tru-1.sh",
  "\nreport cli\n",
  "\nreport cli || true\n",
  "TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession")
