# ForgetAll's whole point is listing the baseline directory under the SAME
# lock the per-file work holds, so a project baseline a concurrent render
# establishes between the listing and the walk reaching it cannot exist --
# the render would be waiting on the same lock. Listing before the lock is
# taken reopens exactly that gap.
m("ForgetAll lists the baseline directory before taking the lock", "internal/baseline/baseline.go",
  "\trelease, lockErr := store.LockPath(filepath.Join(dir, LockName))\n"
  "\tif errors.Is(lockErr, fs.ErrNotExist) {\n"
  "\t\t// No baseline directory: nothing to clear.\n"
  "\t\treturn 0, false, nil\n"
  "\t}\n"
  "\tif lockErr != nil {\n"
  "\t\treturn 0, true, lockErr\n"
  "\t}\n"
  "\tdefer release()\n"
  "\n"
  "\tentries, err := os.ReadDir(dir)\n",
  "\tentries, listErr := os.ReadDir(dir)\n"
  "\trelease, lockErr := store.LockPath(filepath.Join(dir, LockName))\n"
  "\tif errors.Is(lockErr, fs.ErrNotExist) {\n"
  "\t\treturn 0, false, nil\n"
  "\t}\n"
  "\tif lockErr != nil {\n"
  "\t\treturn 0, true, lockErr\n"
  "\t}\n"
  "\tdefer release()\n"
  "\terr = listErr\n",
  "TestRubricLLD11_ForgetAllSeesAFileCreatedWhileItWasBlockedOnTheLock")
