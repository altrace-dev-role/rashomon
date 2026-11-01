# PLW1: the platform graph's producer and reader (card PL-WIRE1). One break
# per output field a reader of the ledger or of a family reads: the row's id
# (the family string), kind, source, state and evidence; a family's facts
# state; which gate the rows owe and which runs cover a package; and, through
# the real pgraph binary H-156 builds, where pgraph writes. The pgraph
# module's own tests run in its CI job, not here: the sweep tests this module.
V = "internal/verify/platform.go"
C = "internal/verify/platform_cover.go"
P = "internal/platform/platform.go"
D = "internal/platform/digests.go"
G = "pgraph/syncroot.go"

# The row: its id (family), kind, source, evidence.
m("PLW1 a known family's id leaves out its hash", V,
  '\t\treturn f.Package + "@" + f.Hash\n', '\t\treturn f.Package\n', "TestPLW1p_")
m("PLW1 a stale family's id reads as a known one's", V,
  '\treturn f.Package + "@" + string(f.Facts)\n', '\treturn f.Package + "@" + f.Hash\n', "TestPLW1p_|TestH156b_")
m("PLW1 the row's kind is not platform_impact", V,
  'Scope: scope, Kind: PlatformImpact,\n', 'Scope: scope, Kind: SpecRule,\n', "TestPLW1p_")
m("PLW1 the row's source is not platform", V,
  '\t\tSource: SourcePlatform, State: state, Evidence: []int64{}}\n', '\t\tSource: obligationSourceSpec, State: state, Evidence: []int64{}}\n', "TestPLW1p_")
m("PLW1 an unreadable log's row has null evidence", V,
  '\t\tSource: SourcePlatform, State: state, Evidence: []int64{}}\n', '\t\tSource: SourcePlatform, State: state, Evidence: nil}\n', "TestPLW1p_")
m("PLW1 an unreadable log owes no row", V,
  '\tif read.Unreadable {\n', '\tif false && read.Unreadable {\n', "TestPLW1p_|TestH156c_")

# Which gate: a required test gate, the project's own word, never a convention.
m("PLW1 an unknown family is judged", V,
  '\t\tif f.Facts != FactsUnknown && len(p.runners) > 0 {\n', '\t\tif len(p.runners) > 0 {\n', "TestPLW1p_|TestH156c_")
m("PLW1 a manifest's convention stands as the gate", V,
  ' || g.id == InferredGateID && !c.inferredDeclared {\n', ' {\n', "TestPLW1p_")
m("PLW1 a build gate stands as the test gate", V,
  '\t\tif g.kind != "test" || !g.required', '\t\tif !g.required', "TestPLW1p_")
m("PLW1 an advisory gate stands as the test gate", V,
  '\t\tif g.kind != "test" || !g.required || g.id', '\t\tif g.kind != "test" || g.id', "TestPLW1p_")

# The state: the package's own last write, the runner, the last cover, a
# later run that might cover it.
m("PLW1 every package owes from the session's last Go write", V,
  ' && path.Dir(*w.Path) == f.Dir {\n', ' {\n', "TestPLW1v_")
m("PLW1 a run before the write counts", V,
  '; r.Seq <= last || s > at {\n', '; s > at {\n', "TestPLW1m_")
m("PLW1 any runner counts", V,
  '; !ok || !p.runners[k] {\n', '; !ok || k == "" {\n', "TestPLW1m_")
m("PLW1 a cover after an unreadable run does not decide", V,
  '\t\t\tcover, maybe = r, false\n', '\t\t\tcover = r\n', "TestPLW1m_")
m("PLW1 an unreadable run after a cover is passed over", V,
  '\tcase maybe:\n\t\treturn ObligationUnknown, []int64{}\n', '\tcase maybe && cover == nil:\n\t\treturn ObligationUnknown, []int64{}\n', "TestPLW1m_")
m("PLW1 a covering run whose outcome is not known is met", V,
  '\tcase cover != nil && green(cover):\n', '\tcase cover != nil && !failed(cover):\n', "TestPLW1m_")
m("PLW1 a red cover is not unmet", V,
  '\tcase cover != nil && !cover.Backgrounded && !masked(cover) && !emptyRun(cover) && failed(cover):\n',
  '\tcase false:\n', "TestPLW1m_")
m("PLW1 an open row cites no write", V,
  '\treturn ObligationOpen, []int64{last}\n', '\treturn ObligationOpen, []int64{}\n', "TestPLW1m_")
m("PLW1 a met row cites the run alone", V,
  '\t\treturn ObligationMet, []int64{last, cover.Seq}\n', '\t\treturn ObligationMet, []int64{cover.Seq}\n', "TestPLW1m_")

# Which runs cover a package.
m("PLW1 a run is placed at the root wherever it ran", C,
  '\t\t\treturn d, true\n', '\t\t\treturn ".", true\n', "TestPLW1m_")
m("PLW1 a run above a nested module covers it", C,
  '\tcase !within(dir, f.ModuleDir):\n\t\treturn triNo\n', '\tcase false:\n\t\treturn triNo\n', "TestPLW1q_")
m("PLW1 local directory mode names nothing", C,
  '\t\tpats = append(pats, "", ".")\n', '\t\tpats = append(pats, ".")\n', "TestPLW1m_|TestPLW1r_")
m("PLW1 no ./d/... pattern below the run's directory", C,
  '\t\t\tpats = append(pats, "./"+strings.Join(parts[:i+1], "/")+"/...")\n', '\t\t\t_ = i\n', "TestPLW1m_|TestPLW1r_|TestH156_")
m("PLW1 -count=1 is not a common flag", C,
  '"-race", "-count=1", "-short"', '"-race", "-short"', "TestPLW1m_")
m("PLW1 a matching digest is no cover", C,
  '\t\treturn triYes\n', '\t\treturn triMaybe\n', "TestPLW1m_|TestPLW1s_")

# A family's facts: no log, an absent log, stale files, the cap.
m("PLW1 a package the log does not hold reads known", P,
  'ModuleDir: l.modDir, Facts: verify.FactsUnknown}', 'ModuleDir: l.modDir, Facts: verify.FactsKnown}', "TestPLW1w_")
m("PLW1 a log gone by the open is unreadable", P,
  '\t\tif err != nil && !errors.Is(err, fs.ErrNotExist) {\n', '\t\tif err != nil {\n', "TestPLW1w_")
m("PLW1 no root reads relative to the process", P,
  '\tif run.Root == "" {\n\t\treturn out\n\t}\n', '', "TestPLW1w_")
m("PLW1 a file that is no .go file names a package", P,
  '\t\tif !strings.HasSuffix(p, ".go") {\n', '\t\tif false {\n', "TestPLW1w_")
m("PLW1 a stale package keeps its hash", P,
  '\t\t\tif stale[pkg] {\n', '\t\t\tif false {\n', "TestPLW1i_|TestH156b_")
m("PLW1 an edited file is not compared", D,
  ' || herr != nil || got != want {\n', ' || herr != nil {\n', "TestPLW1i_|TestH156b_")
m("PLW1 a test file makes its package stale", D,
  '\t\tif stale[pkg] || strings.HasSuffix(rel, "_test.go") {\n', '\t\tif stale[pkg] {\n', "TestPLW1i_")
m("PLW1 no cap on the files hashed", D,
  ' || hashed == maxHashed {\n', ' {\n', "TestPLW1i_")

# Where pgraph writes, through the binary H-156 builds.
m("PLW1 pgraph writes the digests where the hook does not read them", G,
  '\tDigestsFile = "pgraph.digests"\n', '\tDigestsFile = "pgraph.digest"\n', "TestH156_")
m("PLW1 pgraph writes the log where the hook does not read it", G,
  '\tLogFile     = "pgraph.log"\n', '\tLogFile     = "facts.log"\n', "TestH156_")
