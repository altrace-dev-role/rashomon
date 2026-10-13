# Rubric docs-10, errors-13: cmdProbe ran signal watching and the phase check
# ahead of its safe.Guard, under a comment saying it was guarded whole, so a
# panic there exited 2. The probe.start fault point sits at the top of the
# guarded body; moving it back ahead of the barrier is the old layout.
m("docs-10 the probe prologue runs ahead of the barrier", "cmd/rashomon/main.go",
  "\t_ = safe.Guard(func() error {\n\t\tfault.Inject(fault.PointProbeStart)\n",
  "\tfault.Inject(fault.PointProbeStart)\n\t_ = safe.Guard(func() error {\n",
  "TestRubricDOCS10_")
