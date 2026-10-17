# Rubric docs-10, errors-13: cmdProbe ran signal watching and the phase check
# ahead of its safe.Guard, under a comment saying it was guarded whole, so a
# panic there exited 2. The probe.start fault point sits at the top of the
# guarded body; moving it back ahead of the barrier is the old layout.
m("docs-10 the probe prologue runs ahead of the barrier", "cmd/rashomon/main.go",
  "\treturn guardedHook(stderr, func() error {\n\t\tfault.Inject(fault.PointProbeStart)\n",
  "\tfault.Inject(fault.PointProbeStart)\n\treturn guardedHook(stderr, func() error {\n",
  "TestRubricDOCS10_")
m("errors-13 the probe prologue's recovered panic is discarded", "cmd/rashomon/main.go",
  "func guardedHook(stderr io.Writer, fn func() error) int {\n\tif err := safe.Guard(fn); err != nil {\n\t\tfmt.Fprintln(stderr, \"rashomon:\", err)\n\t}\n\treturn exitOK\n}\n",
  "func guardedHook(stderr io.Writer, fn func() error) int {\n\t_ = safe.Guard(fn)\n\treturn exitOK\n}\n",
  "TestRubricDOCS10_|TestGuardedHook_")
