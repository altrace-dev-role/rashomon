# Rubric conc-2: recap.json's writer takes a temporary file of its own. With
# one shared path+".tmp", two overlapping recap processes truncated the inode
# the other had just renamed into place, and the state read as corrupt.
m("RUBRIC conc-2 recap writers share one temp path", "internal/recap/state.go",
  "\ttmp, err := os.CreateTemp(filepath.Dir(path), StateTempPrefix+\"*\")",
  "\ttmp, err := os.Create(path + \".tmp\")",
  "TestRubricCONC2_ConcurrentWritersNeverCorruptTheState")
# Rubric conc-2: a write killed between CreateTemp and the rename leaves its
# temp file behind -- its own deferred Remove never runs for that process --
# and the next write must clear it rather than let it accumulate forever.
m("RUBRIC conc-2 a killed write's temp file is never cleared",
  "internal/recap/state.go",
  "\tremoveStaleTemps(filepath.Dir(path))\n",
  "",
  "TestRubricCONC2_AKilledWritesTempFileIsClearedByTheNext")
m("RUBRIC conc-2 the stale-temp age gate also clears a concurrent writer's live file",
  "internal/recap/state.go",
  "\t\tif err != nil || now.Sub(info.ModTime()) < staleTempAge {\n",
  "\t\tif err != nil || (now.Sub(info.ModTime()) < staleTempAge && false) {\n",
  "TestRubricCONC2_AKilledWritesTempFileIsClearedByTheNext")
