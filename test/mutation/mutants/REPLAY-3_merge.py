# B2c (docs/autosteer-prescan.md, §6.2): a session is merged from all its files
# into one timeline, replayed through the live recorder, and summarised through
# verify. The merge keeps each call once, trusts no call whose copies disagree,
# recovers a subagent's type through the parent's Agent call alone (and keeps
# the subagent unsupported when that link is missing, ambiguous or contradicted
# by the line's own attributionAgent), orders files by a rule of their bytes and
# events by when they happened, and names every place where the order between
# files cannot be established and taints from it. The ledger holds a session
# once and a rescan replaces it; a session recorded live is excluded and
# removed; the family count is the number of distinct sessions with a run that
# matches recorded inputs (DEDUP-1) and no session is in the ledger and the
# live store (DEDUP-2). The summary keeps what replay reproduced apart from what
# the source shows, and a global the replay swapped is put back however it ends.
M = "internal/replay/merge.go"
I = "internal/replay/inspect.go"
P = "internal/replay/payload.go"
H = "internal/history/history.go"
PI = "cmd/rashomon/prescan_import.go"
PL = "cmd/rashomon/prescan_ledger.go"
PS = "cmd/rashomon/prescan_summary.go"
MI = "internal/replay/merge_index.go"
RP = "cmd/rashomon/replay.go"
RD = "internal/store/rundirname.go"
V = "internal/verify/runs.go"

T_MERGE = "TestREPLAY18_|TestREPLAY19_|TestREPLAY27_"
T_CONFLICT = "TestREPLAY20_|TestREPLAY27_"
T_LINK = "TestREPLAY18b_|TestREPLAY18c_|TestREPLAY25b_"
T_ORDER = "TestREPLAY18_|TestREPLAY21_"

# Calls are kept once, whichever file they are in.
m("replay-3  a call's declaration seen again is yielded again", M,
  "\t\tif st.pre {\n\t\t\tm.sum.Copies++\n\t\t\treturn true\n\t\t}\n", "\t\tif false {\n\t\t\tm.sum.Copies++\n\t\t\treturn true\n\t\t}\n", T_MERGE)
m("replay-3  a call's execution seen again is yielded again", M,
  "\t\tif st.post {\n\t\t\tm.sum.Copies++\n\t\t\treturn true\n\t\t}\n", "\t\tif false {\n\t\t\tm.sum.Copies++\n\t\t\treturn true\n\t\t}\n", T_MERGE)
m("replay-3  a prompt or a turn end seen again is yielded again", M,
  "\tif m.turns[k] {\n\t\tm.sum.Copies++\n\t\treturn true\n\t}\n", "\tif false {\n\t\tm.sum.Copies++\n\t\treturn true\n\t}\n", "TestREPLAY19_")
m("replay-3  another session's lines in a file are this session's", M,
  "\tif e.SessionID != \"\" && e.SessionID != m.sessionID {\n", "\tif false {\n", "TestREPLAY19b_")
m("replay-3  a byte for byte copy of a file is a second file", I,
  "\t\tif d := metas[r.i].Digest; seen[d] {\n", "\t\tif d := metas[r.i].Digest; false {\n", "TestREPLAY23_|TestREPLAY27_")

# Copies that disagree are trusted by neither.
m("replay-3  copies that disagree are trusted", M,
  "\tif st.conflict {\n\t\tunsupport(e, CopiesDisagree)\n\t}\n", "\tif false {\n\t\tunsupport(e, CopiesDisagree)\n\t}\n", T_CONFLICT)
m("replay-3  copies with different inputs do not disagree", MI,
  "\t} else if st.input != in {\n\t\tst.conflict = true\n\t}\n", "\t} else if false {\n\t\tst.conflict = true\n\t}\n", T_CONFLICT)
m("replay-3  copies with different results do not disagree", MI,
  "\t\tcase st.result != res:\n\t\t\tst.conflict = true\n", "\t\tcase false:\n\t\t\tst.conflict = true\n", T_CONFLICT)
m("replay-3  every result is the same result", MI,
  "\treturn hash16([]byte{byte(c.Outcome)}, code, bits, c.OutputHead, c.OutputTail), true\n", "\treturn digest16{}, true\n", T_CONFLICT)
m("replay-3  a copy cut before the result disagrees with the complete one", MI,
  "\t\tcase !st.hasResult:\n", "\t\tcase false:\n", "TestREPLAY20_")
m("replay-3  a disagreement taints nothing", M,
  "\t\tm.taint(Place{File: s.i, Pos: e.Pos}, string(e.Gap))\n", "", "TestREPLAY20_|TestREPLAY25b_|TestREPLAY22_")

# The parent link, and nothing else, gives a subagent its type.
m("replay-3  the result line's agent id is not read", H,
  "\tif len(tur.agentID) <= maxHeaderBytes {\n\t\tp.ResultAgentID = tur.agentID\n\t}\n", "", T_LINK)
m("replay-3  two calls that claim one agent are one parent", MI,
  "\t\tcase len(cs) > 1:\n\t\t\tl.Status = LinkAmbiguous\n", "\t\tcase false:\n\t\t\tl.Status = LinkAmbiguous\n", T_LINK)
m("replay-3  a call that claims two agents is the parent of both", MI,
  "\t\t\tcase st == nil || st.claimConflict:\n", "\t\t\tcase st == nil:\n", "TestREPLAY18c_")
m("replay-3  a result of any tool can be a parent", MI,
  "\tif st.agentCall {\n\t\tif c.ResultAgentConflict {\n", "\tif true {\n\t\tif c.ResultAgentConflict {\n", "TestREPLAY18b_")
m("replay-3  a parent whose copies disagree gives the type", MI,
  "\t\t\tcase st.conflict:\n\t\t\t\tl.Status = LinkParentConflict\n", "\t\t\tcase false:\n\t\t\t\tl.Status = LinkParentConflict\n", "TestREPLAY18c_")
m("replay-3  a parent with no subagent_type gives one", MI,
  "\t\t\tcase st.subType == \"\":\n", "\t\t\tcase false:\n", "TestREPLAY18b_")

# attributionAgent is a cross-check and never a source.
m("replay-3  a line attributed to another type does not contradict the link", MI,
  "\t\t\tcase a.attrOverflow || mismatch(a.attr, st.subType):\n", "\t\t\tcase a.attrOverflow:\n", "TestREPLAY18b_|TestREPLAY25b_")
m("replay-3  the line's attributionAgent is not read", H,
  "\t\tp.AttributionAgent = h.AttributionAgent\n", "\t\t_ = h.AttributionAgent\n", "TestREPLAY18b_|TestREPLAY25b_|TestHIST15_")
m("replay-3  a subagent's payload carries no type", P,
  "ToolName: c.ToolName, ToolInput: input, ToolUseID: c.ToolUseID, AgentID: e.AgentID, AgentType: e.AgentType})\n",
  "ToolName: c.ToolName, ToolInput: input, ToolUseID: c.ToolUseID, AgentID: e.AgentID})\n", "TestREPLAY18_|TestREPLAY25_")
m("replay-3  a subagent's execution carries no agent id", P,
  "\t\tAgentID: e.AgentID, AgentType: e.AgentType}\n", "\t\tAgentType: e.AgentType}\n", "TestREPLAY18_|TestREPLAY25_")
m("replay-3  a subagent's background launch with no result detail is fed", P,
  "\t\tassumed := c.Sidechain && c.ResultDetailAbsent && !c.Backgrounded\n", "\t\tassumed := c.Sidechain && c.ResultDetailAbsent\n", "TestREPLAY18d_")

# The file order is a rule of the files' bytes.
m("replay-3  a subagent's file comes before the session's own", I,
  "\t\tif x.main != y.main {\n\t\t\treturn x.main\n\t\t}\n", "", "TestREPLAY23_")
m("replay-3  the later-beginning file comes first", I,
  "\t\treturn fx.Before(fy)\n", "\t\treturn fy.Before(fx)\n", "TestREPLAY23_")
m("replay-3  a subagent's file is a session's own", I,
  "\t\town := !hd.IsSidechain && (hd.Type == \"user\" || hd.Type == \"assistant\")\n", "\t\town := true\n", "TestREPLAY23_|TestREPLAY27_")
m("replay-3  a subagent file with no session of its own is a session", PI,
  "\t\tif own[id] {\n", "\t\tif true {\n", "TestREPLAY27_")

# Causal order across files; where it cannot be established, a named taint.
m("replay-3  a tie between two different events is not a gap", M,
  "\t\tif o := m.tied(best); o != nil {\n", "\t\tif false {\n", "TestREPLAY21_|TestREPLAY30b_")
m("replay-3  a subagent outside its parent's call is not a gap", M,
  "\t\t\ts.viol = SubagentOutsideCall\n", "\t\t\ts.viol = \"\"\n", "TestREPLAY21_")
m("replay-3  a subagent can be placed before its parent's call", M,
  "\t\tif !a.NotBefore.IsZero() && t.Before(a.NotBefore) {\n\t\t\tt = a.NotBefore\n\t\t}\n", "", "TestREPLAY21_")
m("replay-3  a subagent's result at the instant of its parent's comes after it", M,
  "\tcase e.Kind == PostToolUse || e.Kind == PostToolUseFailure || e.Kind == Stop:\n\t\ts.kc = 2\n", "", "TestREPLAY18_")
m("replay-3  a file whose clock goes backward is placed as if it did not", M,
  "\t\ts.viol = OrderRegression\n", "\t\ts.viol = \"\"\n", "TestREPLAY21_")
m("replay-3  a result may be merged before its use", M,
  "\t\tif !st.pre {\n", "\t\tif false {\n", "TestREPLAY21_")
m("replay-3  an unreadable line in a file taints nothing", M,
  "\tif !e.Taint.IsZero() && (gapPos.IsZero() || e.Taint.Before(gapPos)) && !s.lineTaint {\n", "\tif false {\n", "TestREPLAY30b_|TestREPLAY21_")
m("replay-3  a file that ends in an unreadable line taints nothing", M,
  "\t\t\t\tm.taint(Place{File: s.i, Pos: from}, \"unreadable_line\")\n", "", "TestREPLAY21_")
m("replay-3  the merge's taint is not on the events", M,
  "\te.Taint, e.TaintFile = m.first.Pos, m.first.File\n", "\te.TaintFile = m.first.File\n", "TestREPLAY18b_|TestREPLAY20_|TestREPLAY21_")

# Bounds.
m("replay-3  the tool_use_id cap is not enforced", MI,
  "\t\tif len(ix.ids) >= ix.cap {\n", "\t\tif false {\n", "TestREPLAY22_|TestREPLAY31_")
m("replay-3  the file cap is not enforced", M,
  "\tif len(files) > MaxSessionFiles {\n", "\tif false {\n", "TestREPLAY22b_")

# The ledger: a rescan replaces; a live session is excluded and removed.
m("replay-3  a changed session is a second entry in the ledger", PL,
  "\tid := l.ID(sessionID)\n\t_, replaced = l.sessions[id]\n", "\tid := l.ID(sessionID + strings.Join(s.Files, \",\"))\n\t_, replaced = l.sessions[id]\n", "TestREPLAY27_|TestREPLAY28_")
m("replay-3  removing a session from the ledger removes nothing", PL,
  "\tdelete(l.sessions, id)\n\treturn had\n", "\treturn had\n", "TestREPLAY28_")
m("replay-3  a session whose files changed is taken as unchanged", PL,
  "\t\tif s.Files[i] != files[i] {\n", "\t\tif false {\n", "TestREPLAY28_")
m("replay-3  a forced rescan is skipped for a session that has not changed", PI,
  "\tif !im.cfg.Rescan && im.ledger.Unchanged(id, files) {\n", "\tif im.ledger.Unchanged(id, files) {\n", "TestREPLAY27_")
m("replay-3  a session recorded live is imported", PI,
  "\t\tif live {\n\t\t\tout.Status = statusLive\n", "\t\tif false {\n\t\t\tout.Status = statusLive\n", "TestREPLAY27_|TestREPLAY29_")
m("replay-3  the live store is not looked in", PI,
  "\tif im.cfg.LiveRoot != \"\" {\n", "\tif false {\n", "TestREPLAY27_|TestREPLAY28_")
m("replay-3  an import of a session now live stays in the ledger", PI,
  "\t\t\tout.Removed = im.ledger.Remove(id)\n", "\t\t\tout.Removed = false\n", "TestREPLAY28_")
m("replay-3  the live check looks in the wrong directory", RD,
  "\tfi, err := os.Lstat(filepath.Join(root, DirRuns, RunDirName(sessionID)))\n", "\tfi, err := os.Lstat(filepath.Join(root, DirRuns, sessionID+\"x\"))\n", "TestREPLAY27_|TestREPLAY28_")
m("replay-3  the live check makes the directory it looks for", RD,
  "\tfi, err := os.Lstat(filepath.Join(root, DirRuns, RunDirName(sessionID)))\n",
  "\t_ = os.MkdirAll(filepath.Join(root, DirRuns, RunDirName(sessionID)), 0o700)\n\tfi, err := os.Lstat(filepath.Join(root, DirRuns, RunDirName(sessionID)))\n", "TestREPLAY29_")

# DEDUP-1: a session counts once, for the runs that match.
m("replay-3  a family counts runs where it counts sessions", PL,
  "\t\t\tt.SessionsVerified += f.SessionsVerified\n", "\t\t\tt.SessionsVerified += f.RunsVerified\n", "TestREPLAY27_|TestREPLAY27b_")
m("replay-3  a session counts once for every run", PS,
  "\tf.SessionsVerified = 1\n", "\tf.SessionsVerified++\n", "TestREPLAY27_|TestREPLAY30_")
m("replay-3  a run that does not match counts", PS,
  "\t\tif !o.matches() {\n\t\t\tchains[r.Family] = nil\n\t\t\tcontinue\n\t\t}\n", "\t\tif false {\n\t\t\tchains[r.Family] = nil\n\t\t\tcontinue\n\t\t}\n", "TestREPLAY27b_|TestREPLAY30_|TestREPLAY30b_")

# The summary: what counts as matching recorded inputs, and what is kept apart.
m("replay-3  a run after a gap matches recorded inputs", PS,
  "&& o.postGrade == history.Verified && !o.postTainted && !o.assumed\n", "&& o.postGrade == history.Verified && !o.assumed\n", "TestREPLAY30b_|TestREPLAY27_")
m("replay-3  a run of a lesser grade matches recorded inputs", PS,
  "o.preFed && o.postFed && o.postGrade == history.Verified &&", "o.preFed && o.postFed &&", "TestREPLAY30_")
m("replay-3  a run fed on an assumption matches recorded inputs", PS,
  "&& !o.postTainted && !o.assumed\n", "&& !o.postTainted\n", "TestREPLAY27_")
m("replay-3  a transition between two files is a transition", PS,
  "\tcase span:\n\t\tf.PartialTransitions++\n", "\tcase false:\n\t\tf.PartialTransitions++\n", "TestREPLAY30b_")
m("replay-3  a timeout does not break the chain of runs", PS,
  "\tdefault:\n\t\tc[fam] = nil\n\t\treturn\n\t}\n\tch := c[fam]\n", "\tdefault:\n\t\treturn\n\t}\n\tch := c[fam]\n", "TestREPLAY30_")
m("replay-3  the last seen run is not tracked", PS,
  "\tif at.After(f.LastSeen) {\n", "\tif false {\n", "TestREPLAY30_")
m("replay-3  every session reads as complete", PS,
  "\t\tComplete: ms.Complete,", "\t\tComplete: true,", "TestREPLAY30_")
m("replay-3  a timeout is a red", V,
  "\t\tcase 124:\n\t\t\treturn RunTimeout\n", "\t\tcase 124:\n\t\t\treturn RunRed\n", "TestREPLAY30_")
m("replay-3  an exit 127 is a red", V,
  "\t\tcase 126, 127:\n", "\t\tcase 126:\n", "TestREPLAY30_")
m("replay-3  a summary claims the executed inputs were verified", PL,
  "var replayBasis = sessionBasis{Reconstruction: basisMatchesRecordedInputs, SessionEnd: \"unknown\"}\n",
  "var replayBasis = sessionBasis{Reconstruction: basisMatchesRecordedInputs, SessionEnd: \"unknown\", ExecutedInputsVerified: true}\n", "TestREPLAY30_")
m("replay-3  a summary claims capture was complete", PL,
  "var replayBasis = sessionBasis{Reconstruction: basisMatchesRecordedInputs, SessionEnd: \"unknown\"}\n",
  "var replayBasis = sessionBasis{Reconstruction: basisMatchesRecordedInputs, SessionEnd: \"unknown\", CaptureComplete: true}\n", "TestREPLAY30_")
m("replay-3  a session's end is not named unknown", RP,
  "\tflagSessionEndUnknown = \"session_end_unknown\"\n", "\tflagSessionEndUnknown = \"probe_end_not_replayed\"\n", "TestREPLAY24_|TestREPLAY30_")
m("replay-3  a session is not flagged as having an end nobody knows", RP,
  "var replayFlags = []string{flagRewriteAbsent, flagDurationNull, flagProbeStartSynthesized, flagSessionEndUnknown}\n",
  "var replayFlags = []string{flagRewriteAbsent, flagDurationNull, flagProbeStartSynthesized}\n", "TestREPLAY24_|TestREPLAY30_")

# Global state: one replay at a time, and put back whatever ends it.
m("replay-3  two replays can run at once", RP,
  "\treplayMu.Lock()\n\tdefer replayMu.Unlock()\n\n\ttype saved struct {\n", "\ttype saved struct {\n", "TestREPLAY26b_")
m("replay-3  a world is made while a replay has the environment", RP,
  "\treplayMu.Lock()\n\tdefer replayMu.Unlock()\n\tif id == (replayIdentity{}) {\n", "\tif id == (replayIdentity{}) {\n", "TestREPLAY26c_")
m("replay-3  a cancelled replay goes on", RP,
  "\tif r.cfg.Ctx != nil {\n", "\tif false {\n", "TestREPLAY26_")
m("replay-3  an observer's error does not stop the replay", RP,
  "\treturn cfg.Observe(e, fed)\n", "\t_ = cfg.Observe(e, fed)\n\treturn nil\n", "TestREPLAY26_")
m("replay-3  the clock is not put back after a panic", RP,
  "\tdefer func() { entryNow, stopSeen = prevNow, prevSeen }()\n\tif w.afterClock != nil {\n", "\tif w.afterClock != nil {\n", "TestREPLAY26_")
