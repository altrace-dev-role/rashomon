# rubric hld-2 (fix list 28): a render's load-fold-save and a forget's
# rewrite of a project baseline happen under one lock, around a pure fold.
# Saving without the lock, releasing it before the forget's rewrite, or a
# fold that edits the file it was handed is the regression.
m("RUBRIC hld-2 a render saves without the lock", "internal/baseline/baseline.go",
  "\trelease, err := store.LockPath(filepath.Join(dir, lockName))\n\tif err != nil {\n\t\treturn Result{}, err\n\t}\n\tdefer release()\n",
  "", "TestRubricHLD2_ConcurrentRendersLoseNoHost")
m("RUBRIC hld-2 a forget rewrites after releasing the lock", "internal/baseline/baseline.go",
  "\tdefer release()\n\treturn forgetLocked(path, host)\n", "\trelease()\n\treturn forgetLocked(path, host)\n",
  "TestRubricHLD2_ForgetAndRenderLoseNoChange")
m("RUBRIC hld-2 the fold edits the file it was handed", "internal/baseline/fold.go",
  "\tout.Hosts = maps.Clone(f.Hosts)\n", "\tout.Hosts = f.Hosts\n", "TestRubricHLD2_FoldIsPure")
IMPORTS["RUBRIC hld-2 the fold edits the file it was handed"] = ("internal/baseline/fold.go", "\t\"maps\"\n", "")
