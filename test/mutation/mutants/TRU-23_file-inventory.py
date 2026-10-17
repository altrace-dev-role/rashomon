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
