# VG-1: the version gate (card VERSION-GATE1). The channel's table is a floor
# per event (ST-2.1 holds the predicate and the comparison); a shadowed or
# held-out note prints nothing of itself and the session's first prints one
# user-only notice, recorded on its line; every line carries its event's
# basis; the field holdout is off with nothing configured and turned on by
# "on" or a spec's holdout object. Each break removes one rule and names the
# test that holds it.
R = "internal/steer/route.go"
N = "internal/steer/notice.go"
D = "internal/steer/decide.go"
ST = "internal/store/notice.go"
H = "internal/hook/steerstep.go"
C = "cmd/rashomon/steerenv.go"

# The notice: once per session, only on a call that prints nothing else.
m("VG-1 the notice replaces a call's own print", R,
  "\t\tif o == (Output{}) {\n\t\t\to, marker = notice(ctx, c, lines)\n",
  "\t\tif true {\n\t\t\to, marker = notice(ctx, c, lines)\n", "TestVG1i_")
m("VG-1 the notice never prints", N,
  "\tif !ctx.Store.TakeNotice(ctx.SessionID, m) {\n", "\tif true {\n", "TestVG1g_")
m("VG-1 the notice prints on every shadowed call", ST,
  "os.O_CREATE|os.O_EXCL|os.O_WRONLY", "os.O_CREATE|os.O_WRONLY", "TestVG1g_")
m("VG-1 a failed append keeps the marker", R,
  "\t\t\tctx.Store.ReturnNotice(ctx.SessionID, marker)\n", "", "TestVG1k_")
m("VG-1 ReturnNotice removes nothing", ST,
  "\t\t_ = os.Remove(filepath.Join(s.RunDir(sessionID), name))\n", "", "TestVG1k_")
m("VG-1 the notice is not recorded on its line", N, "\tl.Notice = &n\n", "", "TestVG1g_")
m("VG-1 a held-out note gets the channel notice", N,
  "\t\t\treturn l, NoticeHoldout\n", "\t\t\treturn l, NoticeChannel\n", "TestMS5b_HeldOutNoteReachesNeitherSide")
m("VG-1 every surface without the mirror reads as claude -p", N,
  "\t\tif surface == surfaceSDK {\n", "\t\tif true {\n", "TestVG1h_")
m("VG-1 a malformed version is printed", N,
  "\tif !digitRuns(c.Version, '.') {\n\t\tversion = \"Claude Code, version unreadable\"\n",
  "\tif c.Version == \"\" {\n\t\tversion = \"Claude Code, version unreadable\"\n", "TestVG1h_")

# The basis on every line.
m("VG-1 the line carries no basis", D,
  "\t\tClientVersion: version, VerifiedFloor: basis.floor, Excluded: slices.Clip(basis.excluded),\n", "", "TestVG1g_")
m("VG-1 the line carries another event's row", D,
  "\tbasis := mirrorVerifiedOn()[string(ctx.Event)]\n", "\tbasis := mirrorVerifiedOn()[string(EventPostToolUse)]\n",
  "TestVG1l_")

# The holdout: opt-in for the alpha.
m("VG-1 the holdout is on by default again", H, "\tbp := 0\n", "\tbp := holdoutBP\n", "TestVG1e_NoSessionIsHeldOutByDefault")
m("VG-1 the on switch is ignored", H,
  "\tif env.HoldoutOn {\n\t\tbp = holdoutBP\n\t}\n", "", "TestVG1e_NoSessionIsHeldOutByDefault")
m("VG-1 a spec with no holdout object turns it on", H,
  "\tif p.sp != nil && p.sp.Holdout.RateBP != 0 {\n", "\tif p.sp != nil {\n", "TestVG1f_TheSpecsHoldoutObjectTurnsItOn")
m("VG-1 the on switch reads any case", H, "\treturn v == \"on\"\n", "\treturn strings.EqualFold(v, \"on\")\n",
  "TestVG1e_NoSessionIsHeldOutByDefault")
m("VG-1 cmd ignores the on switch", C,
  "\t\tHoldoutOn:    hook.HoldoutOn(os.Getenv(hook.EnvHoldout)),\n", "", "TestVG1m_")

# The probe a floor stands on (test/live/p-tru-1.sh): an event passes only on
# both halves, under that event; a hook that never ran is inconclusive.
P = "test/live/p-tru-1.sh"
HS = "test/live/hc_shape.py"
m("VG-1 the probe passes on additionalContext alone", HS, "    ok = to_agent and to_user\n", "    ok = to_agent\n", "TestVG1n_")
m("VG-1 the probe passes on systemMessage alone", HS, "    ok = to_agent and to_user\n", "    ok = to_user\n", "TestVG1n_")
m("VG-1 the probe reads another event's attachment", HS,
  'a.get("type") in ATTACHMENTS and a.get("hookEvent") == event:', 'a.get("type") in ATTACHMENTS:', "TestVG1n_")
m("VG-1 the probe reads a hook that never ran as a failure", HS,
  "    if not os.path.exists(fired):\n", "    if False:\n", "TestVG1n_")
m("VG-1 the probe's exit loses a failure", P, "    1) worst=1 ;;\n", "    1) ;;\n", "TestVG1n_")
m("VG-1 an inconclusive event outranks a failure", P,
  '    3) [ "$worst" = 1 ] || worst=3 ;;\n', "    3) worst=3 ;;\n", "TestVG1n_")
m("VG-1 the probe runs over a settings file", P,
  'if [ -e "$CFG/settings.json" ]; then\n', "if false; then\n", "TestVG1n_")
m("VG-1 the probe runs in the user's own configuration", P,
  "CFG=${CLAUDE_CONFIG_DIR:-$HOME/.claude-probe}\n", "CFG=${CLAUDE_CONFIG_DIR:-$HOME/.claude}\n", "TestVG1n_")
m("VG-1 PostToolUseFailure's session runs a command that succeeds", P,
  "  if [ \"$ev\" = PostToolUseFailure ]; then cmd=false; fi\n", "", "TestVG1n_")
m("VG-1 the probe keeps the session's transcript", P,
  '      rm -rf "$f" "${f%.jsonl}"; rmdir "$(dirname "$f")" 2> /dev/null\n', "      :\n", "TestVG1n_")
