# --files is written as relative to test/mutation for a path typed by hand,
# but the sweep is run from the module root, so a tab-completed path carries
# the test/mutation/ prefix. Skipping the current-directory attempt leaves
# that shape unresolved.
m("a --files pattern is never tried against the current directory", "test/mutation/sweep.py",
  "    hits = glob.glob(pattern)\n    return hits if hits else glob.glob(str(HERE / pattern))\n",
  "    return glob.glob(str(HERE / pattern))\n",
  "TestFilesPatternResolvesAgainstCurrentDirectoryFirst")
