# A job key with a trailing comment, `sweep:  # on demand`, is still a job
# key. Requiring the line to end in whitespace only misses it, so a job
# written this way carries no timeout bound and TestDEL17_ never sees it.
m("a commented job key is not recognised as a job", "test/mutation/sweep_test.go",
  "`^  ([A-Za-z0-9_-]+):\\s*(#.*)?$`", "`^  ([A-Za-z0-9_-]+):\\s*$`", "TestDEL17b_")
