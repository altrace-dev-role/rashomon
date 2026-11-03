# RP-2: what the project knows, why a note fired, and the previous session's
# line at startup (card REPORT1b, R2). RP is not a row prefix, so the file
# claims no row, as BR-1 and DG-1 do (sweep.py ROW_PREFIXES). Each break removes
# one promise: the pairwise fold, the baseline, the span, the scan's stop rule
# and its bounds, the line replacing the verify line, the id prefix rules, a
# read that never writes, and the redaction pass.
G = "internal/report/gather/knowledge.go"
R = "internal/receipt/previous.go"
R2 = "internal/receipt/receipt.go"
S = "internal/store/latestrecord.go"
W = "internal/report/gather/why.go"
D = "internal/report/redact.go"
C = "cmd/rashomon/brief.go"
B = "internal/brief/brief.go"
L = "internal/learn/ledger.go"

# The fold (RP2b, RP2c, RP2d).
m("RP-2 the fold compares each checkpoint with itself", G, "\t\t\tf.pair(*prev, cur)\n", "\t\t\tf.pair(cur, cur)\n", "TestRP2b_")
m("RP-2 the fold compares Required again", G, "\t\tif !ok || from == g.State {\n", "\t\tif !ok || from == g.State && g.Required {\n", "TestRP2b_")
m("RP-2 a cut checkpoint still prints gate transitions", G, "\tif f.cut > 0 {\n\t\tf.k.GatesCut = f.cut\n\t\treturn\n\t}\n", "", "TestRP2b_")
m("RP-2 the baseline is the newest other session, not the newest before this one's first", G,
  "receipt.Query{Project: project, Skip: sessionID, Before: first.RecordedAtUnixMS, CapWalk: true}", "receipt.Query{Project: project, Skip: sessionID, CapWalk: true}",
  "TestRP2c_")
m("RP-2 a held-to-held digest change is read as invalidated", G, "\tcase hp && !hc:\n", "\tcase hp && (!hc || prev.SpecDigest != cur.SpecDigest):\n", "TestRP2c_")
m("RP-2 a quarantine before the span counts", G, "!q.IsZero() && q.UnixMilli() >= start && q.UnixMilli() <= end", "!q.IsZero()", "TestRP2c_")
m("RP-2 a lesson counts as approved without its tier", G,
  "\t\tif ls.Tier != learn.TierProven && ls.Tier != learn.TierProbation {\n\t\t\tcontinue\n\t\t}\n", "", "TestRP2d_")

# The scan (RP2g, RP2h, RP2s, RP2r).
m("RP-2 Previous stops at the first match", R, "\t\tif sc.Found && r.at < best.RecordedAtUnixMS {\n", "\t\tif sc.Found {\n", "TestRP2g_")
m("RP-2 Previous does not skip the current session", R, "\t\tif r.name == skip {\n\t\t\tcontinue\n\t\t}\n", "", "TestRP2g_")
m("RP-2 the 16-run bound is removed", R, "\t\tif q.MaxRuns > 0 && read >= q.MaxRuns {\n", "\t\tif false {\n", "TestRP2h_")
m("RP-2 the fallback reads the whole file, not the budget left", R, "\t\tleft = q.MaxBytes - spent\n", "\t\tleft = r.size\n", "TestRP2h_")
m("RP-2 the before bound is ignored", R, "p.RecordedAtUnixMS < q.Before && ", "", "TestRP2h_")
m("RP-2 the walk lifts its cap on run directories", R2, "\tif truncated = capped && len(entries) > MaxWalk; truncated {\n\t\tentries = entries[:MaxWalk]\n\t}\n", "", "TestRP2z_|TestRP2zh_")
m("RP-2 a walk the cap cut claims it read the store", R, "\tif truncated && !sc.Found && sc.Bound == \"\" {\n", "\tif false {\n", "TestRP2z_")
m("RP-2 a gate not green reads red only", R, '\t\t\tif g.State != "green" {\n\t\t\t\tp.NotGreen++', '\t\t\tif g.State == "red" {\n\t\t\t\tp.NotGreen++', "TestRP2i_")
m("RP-2 a line that does not fit keeps its reason", R, '\tif len("  "+line) > 80 {\n', "\tif false {\n", "TestRP2i_")
m("RP-2 LatestRecordTail reads the whole file", S, "\tif tail > 0 && info.Size() > tail {\n", "\tif false {\n", "TestRP2s_")
m("RP-2 EachRecordOfKind ignores the callback's stop", S, "\t\t\treturn fn(l)\n", "\t\t\tfn(l)\n\t\t\treturn true\n", "TestRP2r_")
m("RP-2 a record of any version is handed over", S, "\treturn AcceptsKind(kind, v)\n", "\treturn true\n", "TestRP2r_")

# The startup line (RP2t, RP2x).
m("RP-2 the startup line is appended to the verify line", B, "\tfacts[len(facts)-1] = f\n", "\tfacts = append(facts, f)\n", "TestRP2t_")
m("RP-2 a pointer that names nothing replaces the verify line", C, '\tif err != nil || !scan.Found && scan.Bound == "" {\n', "\tif err != nil {\n", "TestRP2zb_|TestRP2t_")
m("RP-2 the pointer is not rendered from the plugin's name", B, 'const PluginReport = "/" + install.PluginName + ":report last"', 'const PluginReport = "/auto" + "steer:report last"', "TestRP2x_|TestBR1i_")
IMPORTS["RP-2 the pointer is not rendered from the plugin's name"] = (B, '\t"github.com/altrace-dev-role/rashomon/internal/install"\n', "")

# why (RP2l, RP2n, RP2o).
m("RP-2 why's ambiguity check is dropped, so the first match wins", W, "\tswitch len(byID) {\n", "\tswitch min(len(byID), 1) {\n", "TestRP2l_")
m("RP-2 why opens the ledger with a write", W, "\tv.Source = sourceOf(l)\n",
  "\t_, _ = learn.Open(st.Root(), strings.TrimSuffix(baseline.FileName(l.Project), baseline.FileExt), time.UnixMilli(l.AtUnixMS))\n\tv.Source = sourceOf(l)\n",
  "TestRP2n_")
IMPORTS["RP-2 why opens the ledger with a write"] = (W, '\t"strings"\n', '\t"strings"\n\t"time"\n\n\t"github.com/altrace-dev-role/rashomon/internal/baseline"\n')
m("RP-2 why reads the gate from the wrong slot", W, '\tif raw, ok := l.Slots["gate"]; ok {\n', '\tif raw, ok := l.Slots["program"]; ok {\n', "TestRP2o_")
m("RP-2 why prints a suppressed line as if its text were kept", "internal/report/why_text.go", "\tif v.TextKept {\n\t\tif v.UserText", "\tif true {\n\t\tif v.UserText", "TestRP2m_")

# Reads that never write, and redaction (RP2p, RP2q).
m("RP-2 the read-only ledger open writes", L, "\treturn openAt(root, project, time.Time{}, false)\n", "\treturn openAt(root, project, time.Unix(1, 0), true)\n", "TestRP2q_OpenReadOnly")
m("RP-2 Redact skips the knowledge pass", D, "\t\ts.Knowledge = redactKnowledge(sess.Knowledge, key)\n", "\t\ts.Knowledge = sess.Knowledge\n", "TestRP2p_")
m("RP-2 Redact keeps the project root in the knowledge block", D, "\tout.Project = redactHost(k.Project, key)\n", "\tout.Project = k.Project\n", "TestRP2p_")
m("RP-2 Redact keeps the project root in a why view", "internal/report/why.go", "\tout.Project = redactHost(v.Project, key)\n", "\tout.Project = v.Project\n", "TestRP2q_Why")
m("RP-2 Redact keeps a why view's texts", "internal/report/why.go", '\tout.ModelText, out.UserText = "", ""\n', "", "TestRP2q_Why")

# The startup line's pointer (RP2zb, RP2zc, RP2x): the project's checkpoint
# file names the last two sessions, the Stop writes them, and a pointer is
# trusted only when the run is there and holds that checkpoint of this
# project. Where the file lives and what reads it are IPF-1's rows.
P = "internal/receipt/previous.go"
X = "internal/verify/index_pointer.go"
m("RP-2 the Stop does not note its checkpoint", "cmd/rashomon/recap_stop.go",
  "\t\t\tnoteCheckpoint(st.Root(), key, p.SessionID, cp.PointSeq, cp.RecordedAtUnixMS)\n", "", "TestIPF1_|TestRP2x_")
m("RP-2 a pointer is taken for another project's checkpoint", P, "if !ok || p.ProjectRoot != project || p.PointSeq != seq {", "if !ok || p.PointSeq != seq {", "TestRP2zb_")
m("RP-2 a pointer is taken for another point of the run", P, "if !ok || p.ProjectRoot != project || p.PointSeq != seq {", "if !ok || p.ProjectRoot != project {", "TestRP2zb_")
m("RP-2 a pointer to a removed run is not said", P, "\t\tcase err != nil && errors.Is(err, fs.ErrNotExist):\n\t\t\tsc.Bound = BoundGone\n\t\t\treturn Last{}, sc, nil\n", "", "TestRP2zb_")
m("RP-2 the previous session may be the current one", X, "\tif c.Last != nil && c.Last.SessionID != current {\n", "\tif c.Last != nil {\n", "TestRP2zc_")
m("RP-2 a clear overwrites the previous session's pointer", X, "\tif c.Last != nil && c.Last.SessionID != session {\n", "\tif c.Last != nil {\n", "TestRP2zc_")

# why's slot allowlist (RP2zd, RP2ze; review of PR 171).
m("RP-2 why prints a non-allowlisted string slot", W, '\t\tif !report.WhySlotShown(name) && name != "path" && isString(raw) {\n', "\t\tif false {\n", "TestRP2zd_")
m("RP-2 the allowlist admits the lesson text slot", "internal/report/why.go", '\t\t"evidence", "unknown":\n', '\t\t"evidence", "unknown", "text":\n', "TestRP2zd_|TestRP2ze_")
m("RP-2 why's text prints a non-allowlisted slot's value", "internal/report/why_text.go", '\t\tif s.Name != "path" && !WhySlotShown(s.Name) {\n', "\t\tif false {\n", "TestRP2ze_")
m("RP-2 --redact digests only the path slot", "internal/report/why.go", "\t\tcase !WhySlotShown(s.Name):\n", "\t\tcase false:\n", "TestRP2zd_|TestRP2ze_")

# Pointed's widening (RP2zf, RP2zg; review of PR 171).
m("RP-2 Pointed never widens a tail miss", R, "\t\tcase info.Size() <= TailBytes:\n", "\t\tcase true:\n", "TestRP2zf_|TestRP2zg_")
m("RP-2 Pointed calls a miss past the widened read gone", R, "\t\t\tif info.Size() > PointedMax {\n", "\t\t\tif false {\n", "TestRP2zf_|TestRP2zg_")
m("RP-2 Pointed's widened read is the whole file", R, "st.LatestRecordTail(session, store.TypeVerify, PointedMax)", "st.LatestRecordTail(session, store.TypeVerify, 0)", "TestRP2zf_")
m("RP-2 Pointed loses the missing-run case", R, "\t\tcase err != nil && errors.Is(err, fs.ErrNotExist):\n", "\t\tcase false:\n", "TestRP2zf_")

# The cap is the baseline's alone (RP2zh, RP2zi, RP2zj; review of PR 171).
m("RP-2 Latest walks only the first MaxWalk directories", R2, "scan(st, Query{Project: project})", "scan(st, Query{Project: project, CapWalk: true})", "TestRP2zh_")
m("RP-2 an uncapped walk is cut at MaxWalk too", R2, "\tif capped {\n\t\tn = MaxWalk + 1\n\t}\n", "\tn = MaxWalk + 1\n", "TestRP2zh_")
m("RP-2 a walk the cap cut does not say it was cut", R, "\tsc.Truncated = truncated\n", "", "TestRP2zh_")
m("RP-2 the report baseline walks every directory", G, "Before: first.RecordedAtUnixMS, CapWalk: true})", "Before: first.RecordedAtUnixMS})", "TestRP2zi_")
m("RP-2 the report does not carry the cut", G, "\t\tk.BaselineWalkCap = receipt.MaxWalk\n", "", "TestRP2zi_")
m("RP-2 a cut baseline walk prints the plain line", "internal/report/text_knowledge.go", "\tcase k.Baseline == nil && k.BaselineWalkCap > 0:\n", "\tcase false:\n", "TestRP2zi_|TestRP2zj_")

# A read-only ledger cannot save (RP2zk; review of PR 171).
m("RP-2 OpenReadOnly's ledger is not marked read-only", L, "tallies: map[string]Tally{}, readOnly: !write}", "tallies: map[string]Tally{}, readOnly: false}", "TestRP2zk_")
m("RP-2 Save ignores the read-only flag", L, "\tif l.readOnly {\n\t\treturn ErrReadOnly\n\t}\n\tactive, err := l.activeBytes()", "\tactive, err := l.activeBytes()", "TestRP2zk_")
m("RP-2 AppendLabel ignores the read-only flag", L,
  "\t// then refuse, and the next Open would quarantine every lesson.\n\tif l.readOnly {\n\t\treturn ErrReadOnly\n\t}\n",
  "\t// then refuse, and the next Open would quarantine every lesson.\n", "TestRP2zk_AReadOnlyLedgerCannotAppendALabel")

# A trusted spec's gate id in the knowledge block and a why view (RP2zm; census g3 W1, STEP-15).
m("RP-2 Redact keeps the knowledge block's gate keys", D, "\t\tg.Gate = redactGateKey(g.Gate, key)\n\t\tout.GateStates", "\t\tout.GateStates", "TestRP2zm_")
m("RP-2 why keeps the gate slot", "internal/report/why.go", "\t\t\ts.Value = redactWord(s.Value, key)\n\t\tcase !WhySlotShown", "\t\tcase !WhySlotShown", "TestRP2zm_")
m("RP-2 why keeps the gate source", "internal/report/why.go", "\t\tout.Source.Detail = redactWord(v.Source.Detail, key)\n", "", "TestRP2zm_")
m("RP-2 why keeps the resolution's gate", "internal/report/why.go", "\t\tr.Gate = redactGateKey(r.Gate, key)\n", "", "TestRP2zm_")
