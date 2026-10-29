# --trim-cache-each bounds a sweep's disk peak to the baseline's cache plus
# one mutant's build, trimming the run's own GOCACHE back to the baseline
# after every mutant, and refuses a GOCACHE that resolves to the shared one.
# Each row drops one part: the trim, the marker's cut (the baseline goes
# too), go's own files, or the symlink resolution that recognises the shared
# cache under another name.
m("SWEEP-1 the trim after each mutant never runs", "test/mutation/sweep.py",
  "            restore()\n            if trim_root:\n",
  "            restore()\n            if False:\n",
  "TestSWEEP1_TrimsOwnCacheAfterEachMutant")
m("SWEEP-1 the trim takes the baseline's entries too", "test/mutation/sweep.py",
  "            if path not in keep and st.st_mtime_ns > since:\n",
  "            if path not in keep:\n",
  "TestSWEEP1_TrimsOwnCacheAfterEachMutant")
m("SWEEP-1 the trim takes go's own files", "test/mutation/sweep.py",
  '    keep = {os.path.join(root, n) for n in ("README", "trim.txt", "log.txt")}\n',
  "    keep = set()\n",
  "TestSWEEP1_TrimsOwnCacheAfterEachMutant")
m("SWEEP-1 the shared cache is compared without resolving links", "test/mutation/sweep.py",
  "    if shared and os.path.realpath(root) == os.path.realpath(shared):\n",
  "    if shared and root == shared:\n",
  "TestSWEEP1_TrimsOwnCacheAfterEachMutant")
