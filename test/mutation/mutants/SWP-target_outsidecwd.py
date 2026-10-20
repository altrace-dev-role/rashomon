# A mutant's target is otherwise unconstrained, and _dirty_targets() runs
# `git status -- <targets>`, which exits non-zero for a path outside the
# repository and silently drops the already-modified note for every target
# in that run, not just the offending one.
m("a target outside the current directory is accepted", "test/mutation/sweep.py",
  "        if outside_cwd(f):\n            problems.append(f\"{name!r}: target {f} is outside the current directory\")\n            continue\n",
  "",
  "TestCheckRejectsATargetOutsideTheCurrentDirectory")
