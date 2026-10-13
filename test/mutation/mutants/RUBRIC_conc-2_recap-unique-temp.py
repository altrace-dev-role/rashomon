# Rubric conc-2: recap.json's writer takes a temporary file of its own. With
# one shared path+".tmp", two overlapping recap processes truncated the inode
# the other had just renamed into place, and the state read as corrupt.
m("RUBRIC conc-2 recap writers share one temp path", "internal/recap/state.go",
  "\ttmp, err := os.CreateTemp(filepath.Dir(path), \".\"+stateFileName+\".tmp-*\")",
  "\ttmp, err := os.Create(path + \".tmp\")",
  "TestRubricCONC2_ConcurrentWritersNeverCorruptTheState")
