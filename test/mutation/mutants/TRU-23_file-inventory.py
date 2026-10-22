# TRU-23: every path the store writes is in the inventory. Both mutants break
# the correspondence between a FileRule and what the store actually creates,
# which is the only thing TestTRU23_ checks.
m("TRU-23 the forgotten-hosts file drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    store.FileForgottenHosts,\n\t\t\tMode:       \"0600\",\n\t\t\tPurpose:    \"every host a `forget --host` has ever named, kept so a later report still suppresses it and a repeat forget recognises it\",",
  "\t\t\tPattern:    \"forgotten-hosts-old.ndjson\",\n\t\t\tMode:       \"0600\",\n\t\t\tPurpose:    \"every host a `forget --host` has ever named, kept so a later report still suppresses it and a repeat forget recognises it\",",
  "TestTRU23_")
m("TRU-23 the run-directory rules lose their session segment", "internal/disclose/disclose.go",
  "\tconst runSegment = store.DirRuns + \"/<session>\"",
  "\tconst runSegment = store.DirRuns",
  "TestTRU23_")

# TRU-23: internal/recap and internal/baseline write under the same store
# root internal/store does, and the inventory has to cover their paths too,
# not only the store package's own. Each mutant below drops one of those
# three rules' Pattern back to a string none of the three packages' own
# constants would resolve to, which is exactly the shape of the regression
# TestTRU23_EveryWrittenPathIsInTheInventory found live (the pause, recap.json
# and baseline/<key>.json paths it drives were all uncovered before this
# change).
m("TRU-23 the pause marker drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    store.FileRecordingPaused,",
  "\t\t\tPattern:    \"paused\",",
  "TestTRU23_EveryWrittenPathIsInTheInventory")
m("TRU-23 recap.json drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    recap.StateFileName,",
  "\t\t\tPattern:    \"recap-state.json\",",
  "TestTRU23_EveryWrittenPathIsInTheInventory")
m("TRU-23 the baseline directory stops matching baseline/<key>.json", "internal/disclose/disclose.go",
  "\t\t\tPattern:    baseline.FilePattern,",
  "\t\t\tPattern:    \"*.ndjson\",",
  "TestTRU23_EveryWrittenPathIsInTheInventory")

# TRU-23: the write-call-site trip-wire (TestTRU23_WriteCallSitesArePinned)
# exists to fail when a count moves without anyone deciding it should. This
# mutant breaks its own comparison rather than the thing it counts, which is
# the only way to show the comparison is load-bearing: a pinned count that
# silently matched anything would be as useless as no test at all.
m("TRU-23 the pinned store write-call-site count stops matching the source", "internal/disclose/disclose_test.go",
  "\t\t{\"store\", 17},",
  "\t\t{\"store\", 18},",
  "TestTRU23_WriteCallSitesArePinned")
# Two root-level writes main's store gained: resolved-cap (hld-7) and the
# baseline directory's lock. The inventory test drives both, so an entry
# that no longer matches either name leaves a written path uncovered.
m("TRU-23 the inventory does not cover resolved-cap", "internal/disclose/disclose.go",
  "\t\t\tPattern:    store.FileResolvedCap,\n", "\t\t\tPattern:    store.FileResolvedCap + \"~\",\n",
  "TestTRU23_EveryWrittenPathIsInTheInventory")
m("TRU-23 the inventory does not cover the baseline lock", "internal/disclose/disclose.go",
  "\t\t\tPattern:    baseline.LockName,\n", "\t\t\tPattern:    baseline.LockName + \"~\",\n",
  "TestTRU23_EveryWrittenPathIsInTheInventory")
