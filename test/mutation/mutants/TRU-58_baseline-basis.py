# TRU-58: a baseline file from the proxy era carries no basis marker and holds
# the hosts a session reached. Read as current, it would call a host "named
# for the first time" when it was only never reached; it is read as absent
# and the project is established again from the record.
m("TRU-58 a baseline without the basis marker is read as current", "internal/baseline/baseline.go",
  "\tif f.Basis != Basis {\n", "\tif false {\n", "TestTRU58_")
# forget --host and such a file. load reads it as absent, so forget used to
# skip it and leave the forgotten host in plaintext on disk, printing "0
# project baseline(s) cleared". It goes whole instead: rewriting it as though
# it carried the marker keeps every other reached host, and leaving it keeps
# the forgotten one.
m("TRU-58 forget rewrites a baseline without the basis marker as though current",
  "internal/baseline/baseline.go", "\tif f.Basis == Basis {\n", "\tif true {\n", "TestTRU58_")
m("TRU-58 forget leaves a baseline without the basis marker on disk", "internal/baseline/baseline.go",
  "\tif err := os.Remove(path); err != nil {\n\t\treturn 0, err\n\t}\n", "", "TestTRU58_")
# The baseline half of forget --host, end to end. A forget that kept the host
# in a current baseline would leave it "seen before" there, so a later session
# naming it again would never be told it is new. Judged by TRU-58b alone: its
# third session names the host after the forget.
m("TRU-58 forget keeps the host in a current baseline", "internal/baseline/baseline.go",
  "\t\tdelete(f.Hosts, host)\n", "", "TestTRU58b_")
m("TRU-58 forget clears no baseline at all", "internal/baseline/baseline.go",
  "\tif path == \"\" || host == \"\" {\n\t\treturn 0, nil\n\t}\n", "\tif true {\n\t\treturn 0, nil\n\t}\n",
  "TestTRU58b_")
# TRU-58c: a baseline this build cannot parse, or of a version it does not
# know, cannot be rewritten without the host. Holding the host, it is deleted
# and counted; not holding it, it is left as it was.
m("TRU-58 forget leaves an unparseable baseline that holds the host", "internal/baseline/baseline.go",
  "\t\treturn removeWhole(path)\n", "\t\treturn 0, nil\n", "TestTRU58c_")
m("TRU-58 forget deletes an unparseable baseline that does not hold the host", "internal/baseline/baseline.go",
  "\t\tif !namesHost(body, host) {\n", "\t\tif false && !namesHost(body, host) {\n", "TestTRU58_")
# Round two. An unreadable baseline may hold the host, so it is an error and
# not a zero; one file the forget cannot clear must not stop it clearing the
# others; and the byte search matches the whole quoted host, not any run of
# its letters.
m("TRU-58 forget reads an unreadable baseline as cleared", "internal/baseline/baseline.go",
  "\t\treturn 0, err\n\t}\n\tf, err := decode(body)", "\t\treturn 0, nil\n\t}\n\tf, err := decode(body)",
  "TestTRU58_ForgetFailsOnABaselineItCannotRead")
m("TRU-58 forget stops at the first baseline it cannot clear", "cmd/rashomon/main.go",
  "\t\t\tfailed = append(failed, err)\n\t\t\tcontinue", "\t\t\treturn cleared, true, err",
  "TestTRU58_ForgetClearsEveryBaselineItCanWhenOneFails")
m("TRU-58 forget matches the host inside another name", "internal/baseline/baseline.go",
  "\tquoted := []byte(`\"` + host + `\"`)\n", "\tquoted := []byte(host)\n",
  "TestTRU58_ForgetMatchesTheWholeHost")
# TRU-58b through the store-level record: a host only the sandbox's trail held
# leaves no gap when forgotten, so forgotten-hosts.ndjson is the only thing
# that keeps it out of the report. Judged end to end, through the CLI.
m("TRU-58 the report ignores the store-level forget record", "internal/store/gaps.go",
  "\t\tif json.Unmarshal(line, &r) == nil && r.Type == TypeForgottenHost {",
  "\t\tif false && json.Unmarshal(line, &r) == nil {",
  "TestTRU58b_AHostOnlyTheSandboxHeld")
