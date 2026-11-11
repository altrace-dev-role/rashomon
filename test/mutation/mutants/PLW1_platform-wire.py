# PLW1: the platform graph's producer and its reader, the ledger verb (card
# PL-WIRE1). One break per output field a reader of the ledger or of a
# family reads: the row's id (the family string), kind, source, scope, state
# and evidence; a family's facts state and test file; which gate the rows
# owe, which runs cover a package and which might have; which writes undo a
# cover; that no read waits on a named pipe; the verb's wiring; and, through
# the real pgraph binary H-156 builds, where pgraph writes. Those last rows
# mutate pgraph/ and the root module's tests judge them; a row the pgraph
# module's own tests judge runs there, from a file that declares
# MODULE = "pgraph" (PL-9_loader-env.py).
V = "internal/verify/platform.go"
C = "internal/verify/platform_cover.go"
P = "internal/platform/read.go"
D = "internal/platform/digests.go"
R = "internal/platform/resolve.go"
L = "internal/platform/platform.go"
LS = "cmd/rashomon/laststop.go"
LR = "cmd/rashomon/report_ledger.go"
RP = "cmd/rashomon/report_platform.go"
G = "pgraph/syncroot.go"

# The row: its id (family), kind, source, scope, evidence; the cap's row.
m("PLW1 a known family's id leaves out its hash", V,
  '\t\treturn f.Package + "@" + f.Hash\n', '\t\treturn f.Package\n', "TestPLW1p_")
m("PLW1 a stale or new family's id reads as a known one's", V,
  '\treturn f.Package + "@" + string(f.Facts)\n', '\treturn f.Package + "@" + f.Hash\n', "TestPLW1p_|TestH156b_")
m("PLW1 the row's kind is not platform_impact", V,
  'Scope: scope, Kind: PlatformImpact,\n', 'Scope: scope, Kind: SpecRule,\n', "TestPLW1p_")
m("PLW1 the row's scope is not the session's", V,
  'Obligation{ID: ObligationID(scope, PlatformImpact, family), Scope: scope,',
  'Obligation{ID: ObligationID(scope, PlatformImpact, family), Scope: ScopeCommit,', "TestPLW1p_|TestPLW1s_")
m("PLW1 the row's source is not platform", V,
  '\t\tSource: SourcePlatform, State: state, Evidence: []int64{}}\n', '\t\tSource: obligationSourceSpec, State: state, Evidence: []int64{}}\n', "TestPLW1p_")
m("PLW1 an unreadable log's row has null evidence", V,
  '\t\tSource: SourcePlatform, State: state, Evidence: []int64{}}\n', '\t\tSource: SourcePlatform, State: state, Evidence: nil}\n', "TestPLW1p_")
m("PLW1 an unreadable log owes no row", V,
  '\tif read.Log == LogUnreadable || read.Log == LogTimedOut {\n', '\tif false {\n', "TestPLW1p_|TestH156c_")
m("PLW1 a timed-out log is read as families", V,
  '\tif read.Log == LogUnreadable || read.Log == LogTimedOut {\n', '\tif read.Log == LogUnreadable {\n', "TestPLW1p_")
m("PLW1 the packages past the cap owe no row", V,
  '\tif read.More > 0 {\n', '\tif false {\n', "TestPLW1p_")

# Which rows are judged: a log, a go test gate, a test file.
m("PLW1 a family with no log is judged", V,
  '\t\tif f.Facts != FactsUnknown && gate && f.Tests {\n', '\t\tif gate && f.Tests {\n', "TestPLW1p_|TestH156c_")
m("PLW1 a package with no test file is judged", V,
  '\t\tif f.Facts != FactsUnknown && gate && f.Tests {\n', '\t\tif f.Facts != FactsUnknown && gate {\n', "TestPLW1p_")
m("PLW1 a gate that is not go test stands", V,
  ' && (g.id != InferredGateID || c.inferredDeclared) && g.runners[k] {\n', ' && (g.id != InferredGateID || c.inferredDeclared) && k != "" {\n', "TestPLW1p_")
m("PLW1 a manifest's convention stands as the gate", V,
  ' && (g.id != InferredGateID || c.inferredDeclared) && g.runners[k] {\n', ' && g.runners[k] {\n', "TestPLW1p_")
m("PLW1 a build-kind gate stands as the test gate", V,
  '\t\tif g.kind == "test" && g.required &&', '\t\tif g.required &&', "TestPLW1p_")
m("PLW1 an advisory gate stands as the test gate", V,
  '\t\tif g.kind == "test" && g.required &&', '\t\tif g.kind == "test" &&', "TestPLW1p_")

# Which writes undo a cover (M2).
m("PLW1 only the package's own directory's files reset it", V,
  '\treturn within(path.Dir(p), f.Dir) ||', '\treturn path.Dir(p) == f.Dir ||', "TestPLW1x_")
m("PLW1 an edit of go.mod does not reset it", V,
  ' || p == path.Join(f.ModuleDir, "go.mod") ||', ' ||', "TestPLW1x_")
m("PLW1 an edit of go.sum does not reset it", V,
  ' || p == path.Join(f.ModuleDir, "go.sum")\n', '\n', "TestPLW1x_")
m("PLW1 an unnamed write after the cover is passed over", V,
  '\tcase slices.ContainsFunc(p.c.unnamed, func(s int64) bool { return s > cov.Seq && s <= at }) ||\n',
  '\tcase false ||\n', "TestPLW1x_")
m("PLW1 a file moved since the cover with no write is passed over", V,
  '; b == boundOwn || b == boundForeign {\n', '; b == boundOwn {\n', "TestPLW1x_")

# Which runs count, cover, or might have (M1, M4).
m("PLW1 a run before the write counts", V,
  '; r.Seq <= last || s > at {\n', '; s > at {\n', "TestPLW1m_")
m("PLW1 any named runner covers", V,
  '\t\tcase ok && k == goTest:\n', '\t\tcase ok && goTest != "":\n', "TestPLW1m_")
m("PLW1 a run with no runner key is passed over", V,
  '\t\tcase !ok || !strings.HasPrefix(k, "\\x00") || r.Kind == kindTest:\n',
  '\t\tcase strings.HasPrefix(k, "\\x01") || r.Kind == kindTest:\n', "TestPLW1m_|TestPLW1o_")
m("PLW1 a launcher's or another test runner's run is passed over", V,
  '\t\tcase !ok || !strings.HasPrefix(k, "\\x00") || r.Kind == kindTest:\n', '\t\tcase !ok:\n', "TestPLW1m_")
m("PLW1 a cover after an unreadable run does not decide", V,
  '\t\t\t\tcov, alone, maybe = r, c == coverAlone, false\n', '\t\t\t\tcov, alone = r, c == coverAlone\n', "TestPLW1m_")
m("PLW1 an unreadable run after a cover is passed over", V,
  '\tcase maybe:\n\t\treturn ObligationUnknown, []int64{}\n', '\tcase maybe && cov == nil:\n\t\treturn ObligationUnknown, []int64{}\n', "TestPLW1m_")
m("PLW1 a covering run whose outcome is not known is met", V,
  '\tcase green(cov):\n', '\tcase !failed(cov):\n', "TestPLW1m_")
m("PLW1 a red run of many packages is unmet for each", V,
  '\tcase alone && !cov.Backgrounded', '\tcase (alone || !alone) && !cov.Backgrounded', "TestPLW1m_")
m("PLW1 a red run of the package alone is not unmet", V,
  '\tcase alone && !cov.Backgrounded', '\tcase false && alone && !cov.Backgrounded', "TestPLW1m_")
m("PLW1 an open row cites no write", V,
  '\treturn ObligationOpen, []int64{last}\n', '\treturn ObligationOpen, []int64{}\n', "TestPLW1m_")
m("PLW1 a met row cites the run alone", V,
  '\t\treturn ObligationMet, []int64{last, cov.Seq}\n', '\t\treturn ObligationMet, []int64{cov.Seq}\n', "TestPLW1m_")

# Which lines cover a package (M3, M4).
m("PLW1 a run is placed at the root wherever it ran", C,
  '\t\t\treturn d, true\n', '\t\t\treturn ".", true\n', "TestPLW1m_")
m("PLW1 a run above a nested module covers it", C,
  '\tcase !within(dir, f.ModuleDir):\n\t\treturn coverNo\n', '\tcase false:\n\t\treturn coverNo\n', "TestPLW1q_")
m("PLW1 a line no covering line's whole digest matches still decides", C,
  '\tif c, ok := ds[d.Shape.Digest]; ok {\n', '\tif c, ok := ds[d.Shape.Digest]; ok || d.Shape.RunnerDigest != nil {\n', "TestPLW1m_")
m("PLW1 a line that names no covering line is a miss", C,
  '\treturn coverMaybe\n}\n\n// coverLine is', '\treturn coverNo\n}\n\n// coverLine is', "TestPLW1m_")
m("PLW1 local directory mode names nothing", C,
  'pats = append(pats, pat{"", coverAlone}, pat{".", coverAlone})', 'pats = append(pats, pat{".", coverAlone})', "TestPLW1m_|TestPLW1r_")
m("PLW1 no ./d/... pattern below the run's directory", C,
  '\t\t\tpats = append(pats, pat{"./" + strings.Join(parts[:i+1], "/") + "/...", coverMany})\n', '\t\t\t_ = i\n',
  "TestPLW1m_|TestPLW1r_|TestH156_")
m("PLW1 ./pkg/... names the package alone", C,
  '"/...", coverMany})\n', '"/...", coverAlone})\n', "TestPLW1m_|TestPLW1r_")
m("PLW1 -count=1 is not a common flag", C,
  '"-race", "-count=1", "-cover"', '"-race", "-cover"', "TestPLW1m_")
m("PLW1 -short is a common flag", C,
  '"-race", "-count=1", "-cover"', '"-race", "-count=1", "-short", "-cover"', "TestPLW1m_|TestPLW1n_")

# A family's facts and test file (S3, N2), the cap, no log, a timed-out log.
m("PLW1 a package the log does not hold reads unknown", P,
  '\t\t\t\tf.Facts = verify.FactsNew\n', '\t\t\t\tf.Facts = verify.FactsUnknown\n', "TestPLW1w_")
m("PLW1 a family with no log reads known", P,
  'ModuleDir: l.modDir, Facts: verify.FactsUnknown}', 'ModuleDir: l.modDir, Facts: verify.FactsKnown}', "TestPLW1w_")
m("PLW1 a log gone by the open is unreadable", P,
  '\t\tcase errors.Is(err, fs.ErrNotExist):\n', '\t\tcase false:\n', "TestPLW1w_")
m("PLW1 a timed-out log reads unreadable", P,
  '\t\tcase errors.Is(err, context.DeadlineExceeded):\n', '\t\tcase false:\n', "TestPL8c_")
m("PLW1 no root reads relative to the process", P,
  '\tif run.Root == "" {\n\t\treturn out\n\t}\n', '', "TestPLW1w_")
m("PLW1 a file that is no .go file names a package", P,
  '\t\tif !strings.HasSuffix(p, ".go") {\n', '\t\tif false {\n', "TestPLW1w_")
m("PLW1 no cap on the families", P,
  '\tif len(pkgs) > MaxFamilies {\n', '\tif false {\n', "TestPLW1w_")
m("PLW1 a stale package keeps its hash", P,
  '\t\t\tcase fresh(run.Root, dg, pkg, written[pkg], &hashed):\n',
  '\t\t\tcase true || fresh(run.Root, dg, pkg, written[pkg], &hashed):\n', "TestPLW1i_|TestH156b_")
m("PLW1 every package has a test file", P,
  '\t\t\tf.Tests = hasTests(run.Root, l.dir)\n', '\t\t\tf.Tests = true\n', "TestPLW1w_")
m("PLW1 any Go file is a test file", P,
  'strings.HasSuffix(e.Name(), "_test.go")', 'strings.HasSuffix(e.Name(), ".go")', "TestPLW1w_")
m("PLW1 an edited file is not compared", D,
  ' || got != dg.files[rel] {\n', ' || false && got != dg.files[rel] {\n', "TestPLW1i_|TestH156b_")
m("PLW1 only the files the session wrote are hashed", D,
  '\tfor _, rel := range dg.byPkg[pkg] {\n', '\tfor _, rel := range written {\n', "TestPLW1i_")
m("PLW1 a written file the digests do not list passes", D,
  '; !listed && !strings.HasSuffix(rel, "_test.go") {\n', '; false && !listed && !strings.HasSuffix(rel, "_test.go") {\n', "TestPLW1i_")
m("PLW1 a test file the session wrote makes its package stale", D,
  '; !listed && !strings.HasSuffix(rel, "_test.go") {\n', '; !listed {\n', "TestPLW1i_")
m("PLW1 no cap on the files hashed", D,
  '\t\tif *hashed == maxHashed {\n', '\t\tif false {\n', "TestPLW1i_")

# No read waits on a named pipe (M5).
m("PLW1 the log is opened with os.Open", L,
  '\tf, err := regfile.Open(logPath)\n', '\tf, err := os.Open(logPath)\n', "TestPLW1y_")
IMPORTS["PLW1 the log is opened with os.Open"] = (L, '\t"github.com/altrace-dev-role/rashomon/internal/regfile"\n', '\t"os"\n')
m("PLW1 a go.mod is opened with os.Open", R,
  '\tf, err := regfile.Open(p)\n', '\tf, err := os.Open(p)\n', "TestPLW1y_")
IMPORTS["PLW1 a go.mod is opened with os.Open"] = (R, '\t"github.com/altrace-dev-role/rashomon/internal/regfile"\n', '\t"os"\n')
m("PLW1 the digests are opened with os.Open", D,
  'func readDigests(p string) (*digests, error) {\n\tf, err := regfile.Open(p)\n',
  'func readDigests(p string) (*digests, error) {\n\tf, err := os.Open(p)\n', "TestPLW1y_")
IMPORTS["PLW1 the digests are opened with os.Open"] = (D, '\t"io"\n', '\t"io"\n\t"os"\n')
m("PLW1 a written file is hashed through os.Open", D,
  'func hashFile(p string) (string, error) {\n\tf, err := regfile.Open(p)\n',
  'func hashFile(p string) (string, error) {\n\tf, err := os.Open(p)\n', "TestPLW1y_")
IMPORTS["PLW1 a written file is hashed through os.Open"] = (D, '\t"io"\n', '\t"io"\n\t"os"\n')
m("PLW1 a package's directory is listed through os.Open", P,
  '\td, err := regfile.OpenDir(filepath.Join(root, filepath.FromSlash(dir)))\n',
  '\td, err := os.Open(filepath.Join(root, filepath.FromSlash(dir)))\n', "TestPLW1y_")
IMPORTS["PLW1 a package's directory is listed through os.Open"] = (P,
  '\t"io/fs"\n\t"path/filepath"\n\t"slices"\n\t"strings"\n\n\t"github.com/altrace-dev-role/rashomon/internal/regfile"\n',
  '\t"io/fs"\n\t"os"\n\t"path/filepath"\n\t"slices"\n\t"strings"\n\n')

# The ledger verb's wiring (N1, S1).
m("PLW1 the verb's replay reads no platform graph", LR,
  'r, err = replayLastStop(st, sessionID, platformRead)', 'r, err = replayLastStop(st, sessionID, nil)', "TestPLW1s_|TestH156_")
m("PLW1 the replay drops the reader it is given", LS,
  '\tin.Platform = platform\n', '\tin.Platform = nil\n', "TestPLW1s_|TestH156_")
m("PLW1 the ledger prints no platform header", LR,
  '\tb.WriteString(platformHeader(v.Platform.Log))\n', '', "TestPLW1s_|TestPLW1t_")
m("PLW1 the header claims a read with no log", RP,
  '\tcase verify.LogRead:\n', '\tcase verify.LogRead, verify.LogNone:\n', "TestPLW1t_|TestPLW1u_")
m("PLW1 a platform row's family prints as the package alone", LR,
  '\t\t\t\t\tout[o.ID] = f.Family()\n', '\t\t\t\t\tout[o.ID] = f.Package\n', "TestPLW1s_|TestH156_")
m("PLW1 a nested module's package is placed in the outer module", R,
  '\t\t\treturn pkgLoc{m.path + "/" + strings.TrimPrefix(dir, d+"/"), dir, d}\n',
  '\t\t\treturn pkgLoc{m.path + "/" + strings.TrimPrefix(dir, d+"/"), dir, "."}\n', "TestPLW1w_")

# Where pgraph writes, through the binary H-156 builds (S6).
m("PLW1 pgraph writes the digests where the verb does not read them", G,
  '\tDigestsFile = "pgraph.digests"\n', '\tDigestsFile = "pgraph.digest"\n', "TestH156_")
m("PLW1 pgraph writes the log where the verb does not read it", G,
  '\tLogFile     = "pgraph.log"\n', '\tLogFile     = "facts.log"\n', "TestH156_")
m("PLW1 pgraph writes into rashomon's directory", G,
  '\tStateDir    = ".pgraph"\n', '\tStateDir    = ".rashomon"\n', "TestH156_")
