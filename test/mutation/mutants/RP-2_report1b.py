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
  "receipt.Query{Project: project, Skip: sessionID, Before: first.RecordedAtUnixMS}", "receipt.Query{Project: project, Skip: sessionID}",
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
m("RP-2 the walk lifts its cap on run directories", R2, "\tif truncated = len(entries) > MaxWalk; truncated {\n\t\tentries = entries[:MaxWalk]\n\t}\n", "", "TestRP2z_")
m("RP-2 a walk the cap cut claims it read the store", R, "\tif truncated && !sc.Found && sc.Bound == \"\" {\n", "\tif false {\n", "TestRP2z_")
m("RP-2 a gate not green reads red only", R, '\t\t\tif g.State != "green" {\n\t\t\t\tp.NotGreen++', '\t\t\tif g.State == "red" {\n\t\t\t\tp.NotGreen++', "TestRP2i_")
m("RP-2 a line that does not fit keeps its reason", R, '\tif len("  "+line) > 80 {\n', "\tif false {\n", "TestRP2i_")
m("RP-2 LatestRecordTail reads the whole file", S, "\tif tail > 0 && info.Size() > tail {\n", "\tif false {\n", "TestRP2s_")
m("RP-2 EachRecordOfKind ignores the callback's stop", S, "\t\t\treturn fn(l)\n", "\t\t\tfn(l)\n\t\t\treturn true\n", "TestRP2r_")
m("RP-2 a record of any version is handed over", S, "\treturn AcceptsKind(kind, v)\n", "\treturn true\n", "TestRP2r_")

# The startup line (RP2t, RP2x).
m("RP-2 the startup line is appended to the verify line", B, "\tfacts[len(facts)-1] = f\n", "\tfacts = append(facts, f)\n", "TestRP2t_")
m("RP-2 a walk that found nothing replaces the verify line", C, '\tif err != nil || !scan.Found && scan.Bound == "" {\n', "\tif err != nil {\n", "TestRP2t_|TestBR1e_")
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
