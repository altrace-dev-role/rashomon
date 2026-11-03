# WR-1 (card WRAPPER-RUN1): a test run launched through a wrapper (a script,
# a shell, make's build target) is found by its output: a runner's row reads
# the output only where the runner's signature is in it; the post hook keeps
# only a runner's read; a declared gate binds a wrapper's run by the parser at
# its location, decided by the output and never by the wrapper's exit; an
# unread wrapper after the last edit leaves the gate unknown, never "not
# run"; the Stop and the report say it ran in a wrapper.
G = "internal/gateout/gateout.go"
T = "internal/gateout/tables.go"
P = "internal/hook/post.go"
O = "internal/hook/gateoutput.go"
S = "internal/shape/gaterun.go"
V = "internal/verify/gates.go"
D = "internal/verify/declared.go"
STOP = "internal/verify/stop.go"
SV = "internal/verify/steer.go"
R = "internal/recap/value.go"
REP = "internal/report/testbending.go"

# gateout: the signature rule.
m("WR-1 a wrapper's output counts the rows with no signature", G,
  "\t\tcase !wrapper:\n", "\t\tdefault:\n", "TestWR1_AnOutputNoRunnerSignedIsNoRun")
m("WR-1 every runner row reads as signed", G,
  "if open&(1<<i) != 0 && matches(rows[i].sig, line) {", "if open&(1<<i) != 0 && (matches(rows[i].sig, line) || true) {",
  "TestWR1_AnOutputNoRunnerSignedIsNoRun|TestWR1_ADelegateCountsARunnerOnlyWhereItSigned")
m("WR-1 a delegate counts a runner it never signed (D3)", G,
  "\t\tcase len(rows[i].sig) > 0:\n", "\t\tcase wrapper && len(rows[i].sig) > 0:\n",
  "TestWR1_ADelegateCountsARunnerOnlyWhereItSigned")
m("WR-1 vitest's indented summary is not read", T,
  r"pass: compile(`^\s*Tests\s+.*\d+ passed`)", r"pass: compile(`^Tests\s+.*\d+ passed`)",
  "TestWR1_AWrapperIsReadByTheRunnerThatSignedIt")
m("WR-1 a signed run with no outcome names the wrapper", G,
  "\tif wrapper {\n\t\t// A runner signed the output", "\tif false {\n\t\t// A runner signed the output",
  "TestWR1_AnOutputNoRunnerSignedIsNoRun")

# hook: what is read, and what is kept.
m("WR-1 a wrapper call's output is never read", P,
  "} else if shape.WrapperRun(", "} else if false && shape.WrapperRun(",
  "TestWR1_AWrapperCallKeepsOnlyARunnersRead")
m("WR-1 a wrapper's unsigned read is kept", P,
  "p.now); gateout.Runner(out.Parser) {", "p.now); out.Parser != \"\" {",
  "TestWR1_AWrapperCallKeepsOnlyARunnersRead")
m("WR-1 a launcher's build target is never read", P,
  "if kind == shape.RunnerBuild && gateout.Delegated(program) {",
  "if false && kind == shape.RunnerBuild && gateout.Delegated(program) {",
  "TestWR1_AWrapperCallKeepsOnlyARunnersRead")
m("WR-1 an empty stderr overwrites stdout's signed read", O,
  "i == 0 || next.Kind != \"no_summary\" {", "true {",
  "TestWR1_AWrapperCallKeepsOnlyARunnersRead")
m("WR-1 the consent screen keeps a wrapper's word unnamed", "internal/disclose/paragraphs.go",
  "or another command whose output holds a line a test runner prints about its run, ", "",
  "TestWR1_TheDisclosureNamesTheWrapperRead")
m("WR-1 a line of any program is read as a wrapper", S,
  "toolName == \"Bash\" && wrapperProgram(program) && !GateRun(", "toolName == \"Bash\" && !GateRun(",
  "TestWR1_AWrapperRunStartsWithAShellOrAScript")
m("WR-1 a shell is no wrapper", S,
  "\tcase \"bash\", \"sh\", \"zsh\", \"dash\", \"ksh\":\n\t\treturn true\n", "\tcase \"bash\", \"sh\", \"zsh\", \"dash\", \"ksh\":\n\t\treturn false\n",
  "TestWR1_AWrapperRunStartsWithAShellOrAScript")
m("WR-1 a script named by its extension is no wrapper", S,
  "return strings.HasSuffix(p, \".sh\") || strings.HasSuffix(p, \".bash\")",
  "return false && (strings.HasSuffix(p, \".sh\") || strings.HasSuffix(p, \".bash\"))",
  "TestWR1_AWrapperRunStartsWithAShellOrAScript")

# verify: how a wrapped run is decided and bound.
m("WR-1 a wrapped run is red by the wrapper's exit", V,
  "\tif r.Wrapped {\n\t\treturn r.Output.Kind == outSummaryFail", "\tif false {\n\t\treturn r.Output.Kind == outSummaryFail",
  "TestWR1_AWrappedRunIsDecidedByItsOutput")
m("WR-1 a masked line hides a wrapped run", V,
  "\treturn statusHidden(r) && !r.FromOutput && !r.FromPrinted && !r.Wrapped\n", "\treturn statusHidden(r) && !r.FromOutput && !r.FromPrinted\n",
  "TestWR1_AWrappedRunIsDecidedByItsOutput")
m("WR-1 a wrapped run whose output did not say is green", V,
  " || r.Wrapped && !r.FromOutput", "",
  "TestWR1_AWrappedRunIsDecidedByItsOutput")
m("WR-1 a wrapped red reads failed by exit", V,
  "if f.last.Wrapped || !failedByExit(f.last) {", "if !failedByExit(f.last) {",
  "TestWR1_AWrappedRunIsDecidedByItsOutput")
m("WR-1 a wrapper binds at any location (D6)", D,
  " && r.Location == g.location &&\n", " &&\n",
  "TestWR1_AWrapperBindsOnlyByItsRunnerAtTheGatesLocation")
m("WR-1 any runner's read binds the gate", D,
  "r.Output.Parser == g.program", "r.Output.Parser != \"\"",
  "TestWR1_AWrapperBindsOnlyByItsRunnerAtTheGatesLocation")
m("WR-1 a wrapped run reads as other arguments", D,
  "!r.Wrapped && r.Runner != nil", "r.Runner != nil",
  "TestWR1_AWrapperBindsOnlyByItsRunnerAtTheGatesLocation")
m("WR-1 an unread wrapper leaves the gate not run (D5)", D,
  "if unread != nil && (", "if false && unread != nil && (",
  "TestWR1_AnUnreadWrapperAfterTheLastEditIsUnknown")
m("WR-1 a delegate's run is no wrapper", D,
  " ||\n\t\t\tisGateRun(d, masked, runner) && gateout.Delegated(g.Program)\n", "\n",
  "TestWR1_AWrappedRunIsFoundByItsOutput")
m("WR-1 a wrapped red raises regression-location", SV,
  "\t\tg.LastRun.Wrapped != WrapNone || g.LastGreen.Wrapped != WrapNone {", "\t\tg.LastGreen.Wrapped != WrapNone {",
  "TestWR1_AWrappedRunRaisesNoSteerNote")
m("WR-1 a wrapped green raises regression-location", SV,
  "g.LastRun.Wrapped != WrapNone || g.LastGreen.Wrapped != WrapNone {", "g.LastRun.Wrapped != WrapNone {",
  "TestWR1_AWrappedRunRaisesNoSteerNote")
m("WR-1 a wrapped green raises stale-evidence", SV,
  "g.LastGreen == nil || g.LastGreen.Wrapped != WrapNone {", "g.LastGreen == nil {",
  "TestWR1_AWrappedRunRaisesNoSteerNote")
m("WR-1 a wrapped run names its gate by the wrapper", STOP,
  " && g.LastRun.Wrapped == WrapNone", "",
  "TestWR1_AWrappedRunIsFoundByItsOutput")

# recap and report: what the person reads.
m("WR-1 a wrapped green reads passed after the last edit", R,
  "case wrap(g) == verify.WrapRead:", "case false:",
  "TestWR1_AWrappedRunIsFoundByItsOutput")
m("WR-1 an unread wrapper reads as a run whose result is lost", R,
  "\t\tswitch wrap(g) {\n", "\t\tswitch verify.WrapNone {\n",
  "TestWR1_AWrappedRunIsFoundByItsOutput")
# #234 ruling (a): a delegate line that printed its status keeps that status,
# not the go summary inside it, so the gate's line says only the status was
# read, never that the output named no run.
m("WR-1 a printed status reads as an output that names no run", R,
  "\t\t\tif printedStatus(g.LastRun) {\n", "\t\t\tif false {\n",
  "TestWR1_APrintedStatusOutranksTheWrapperRead")
m("WR-1 a wrapped red reads as the wrapper's exit", R,
  "if wrap(g) == verify.WrapRead {", "if false {",
  "TestWR1_AWrappedRunIsFoundByItsOutput")
m("WR-1 a wrapped green carries the pipe's clause", R,
  " || wrap(g) != verify.WrapNone", "",
  "TestWR1_AWrappedRunIsFoundByItsOutput")
m("WR-1 a wrapped run is not counted as one", REP,
  "\tout.Wrapped++\n", "",
  "TestWR1_AWrappedRunIsFoundByItsOutput")
m("WR-1 a pass beside a failed call counts ok", REP,
  "case rec.GateOutput.Kind == \"summary_pass\" && o == store.ExecOK:", "case rec.GateOutput.Kind == \"summary_pass\":",
  "TestWR1_AWrappedRunIsFoundByItsOutput")
