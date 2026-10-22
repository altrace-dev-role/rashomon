# The probe's stdout is SessionStart's, and Claude Code hands that to the
# model. One break per place a line could be printed: after the probe runs
# (both phases), inside the end phase, in the refusal of a phase the probe
# does not know -- which runs before the store opens -- and
# in the stand-down line, which the probe shares with hook and post.
m("H-107 the probe announces each phase on stdout", "cmd/rashomon/main.go",
  "\t\tif runErr != nil {\n\t\t\tfmt.Fprintln(stderr, evictionLine)\n\t\t}\n",
  "\t\tif runErr != nil {\n\t\t\tfmt.Fprintln(stderr, evictionLine)\n\t\t}\n\t\tfmt.Fprintln(os.Stdout, \"rashomon: probe\", phase)\n",
  "TestH107_ProbeStartAndEndPrintNothing")
m("H-107 the end probe reports on stdout", "internal/hook/probe.go",
  "\t\tcase store.PhaseEnd:\n\t\t\trun, err := st.ReadRun(sessionID)\n",
  "\t\tcase store.PhaseEnd:\n\t\t\tfmt.Println(\"rashomon: end of run\")\n\t\t\trun, err := st.ReadRun(sessionID)\n",
  "TestH107_ProbeStartAndEndPrintNothing")
IMPORTS["H-107 the end probe reports on stdout"] = ("internal/hook/probe.go", '\t"io"\n', '\t"fmt"\n\t"io"\n')
m("H-107 the phase refusal goes to stdout", "cmd/rashomon/main.go",
  "\t\tfmt.Fprintln(stderr, \"rashomon: probe expects start or end\")",
  "\t\tfmt.Fprintln(os.Stdout, \"rashomon: probe expects start or end\")",
  "TestH107_AnUnknownPhaseSaysSoOnStderrOnly")
m("H-107 the stand-down line goes to stdout", "cmd/rashomon/main.go",
  "\tfmt.Fprintln(stderr, standDownLine)",
  "\tfmt.Fprintln(os.Stdout, standDownLine)",
  "TestH107_ProbeThatDoesNotRecordPrintsNothing")
