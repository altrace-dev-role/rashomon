# rubric lld-4 (fix list 30): every spelling of one IP address canonicalises
# to one string, and loopback is decided on the parsed address. Keeping an
# address as written, leaving an IPv4-mapped address mapped, skipping the
# inet_aton forms or misreading their parts, or deciding loopback from a list
# of spellings is the regression.
m("RUBRIC lld-4 an IPv6 literal is kept as written", "internal/host/host.go",
  "\t\treturn spell(a), true\n", "\t\treturn h, true\n", "TestRubricLLD4_")
m("RUBRIC lld-4 an IPv4-mapped address stays mapped", "internal/host/host.go",
  "\tif a = a.Unmap(); a.Is4() {", "\tif a.Is4() {", "TestRubricLLD4_")
m("RUBRIC lld-4 the inet_aton forms are names", "internal/host/host.go",
  "\tif a, ok := inetAton(h); ok {", "\tif a, ok := inetAton(h); ok && false {", "TestRubricLLD4_")
m("RUBRIC lld-4 the last inet_aton part fills one byte", "internal/host/host.go",
  "\t\t\twidth = 8 * uint(5-len(parts))\n", "\t\t\twidth = 8\n", "TestRubricLLD4_")
m("RUBRIC lld-4 a leading 0 is read as decimal", "internal/host/host.go",
  "\t\tp, base = p[1:], 8\n", "\t\tp, base = p[1:], 10\n", "TestRubricLLD4_")
m("RUBRIC lld-4 loopback is a list of spellings", "internal/host/host.go",
  "\treturn err == nil && (a.IsLoopback() || a.IsUnspecified())\n",
  "\treturn err == nil && (a.String() == \"127.0.0.1\" || a.String() == \"::1\" || a.IsUnspecified())\n", "TestRubricLLD4_")
m("RUBRIC lld-4 a name under .localhost is not loopback", "internal/host/host.go",
  "\tif c == \"localhost\" || strings.HasSuffix(c, \".localhost\") {", "\tif c == \"localhost\" {", "TestRubricLLD4_")
m("RUBRIC lld-4 the report filters loopback by its old spellings", "internal/report/destinations.go",
  "\t\tif host.Loopback(h) || clientPlaneHost(h, mcpAttributed) {",
  "\t\tif h == \"localhost\" || h == \"127.0.0.1\" || h == \"[::1]\" || h == \"0.0.0.0\" || clientPlaneHost(h, mcpAttributed) {",
  "TestRubricLLD4_")
IMPORTS["RUBRIC lld-4 the report filters loopback by its old spellings"] = ("internal/report/destinations.go",
  "\t\"github.com/altrace-dev-role/rashomon/internal/host\"\n", "")
# forget --host asks in the recorded spelling (fix list 30, the forget half):
# `curl https://167838211/x` records 10.1.2.3, and a forget that compared the
# typed value, or a recorded one as stored, left the host on disk while
# reading as done; a value that names no host must be refused, not forgotten
# as nothing.
m("RUBRIC lld-4 forget --host compares the typed value", "internal/store/gaps.go",
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn nil, ErrNotAHostname\n\t}\n",
  "\tname, ok := raw, raw != \"\"\n\tif !ok {\n\t\treturn nil, ErrNotAHostname\n\t}\n", "TestRubricLLD4_")
m("RUBRIC lld-4 a value naming no host is forgotten as nothing", "internal/store/gaps.go",
  "\t\treturn nil, ErrNotAHostname\n", "", "TestRubricLLD4_")
m("RUBRIC lld-4 a stored host is compared only as stored", "internal/host/host.go",
  "\tc, ok := Canonical(stored)\n\treturn ok && c == canonical\n", "\treturn false\n", "TestRubricLLD4_")
m("RUBRIC lld-4 a baseline is asked in the typed spelling", "internal/baseline/baseline.go",
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn 0, store.ErrNotAHostname\n\t}\n",
  "\tname, ok := raw, raw != \"\"\n\tif !ok {\n\t\treturn 0, store.ErrNotAHostname\n\t}\n", "TestRubricLLD4_")
m("RUBRIC lld-4 a baseline forget of no host reads as done", "internal/baseline/baseline.go",
  "\t\treturn 0, store.ErrNotAHostname\n", "\t\treturn 0, nil\n", "TestRubricLLD4_")
m("RUBRIC lld-4 an unparsed baseline is searched for the typed spelling only", "internal/baseline/baseline.go",
  "\t\tif !namesHost(body, name) && !namesHost(body, raw) {\n", "\t\tif !namesHost(body, raw) {\n", "TestRubricLLD4_")
m("RUBRIC lld-4 the forget summary names the typed value", "cmd/rashomon/main.go",
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn errors.New(\"--host is not a hostname",
  "\t_, ok := host.Canonical(raw)\n\tname := raw\n\tif !ok {\n\t\treturn errors.New(\"--host is not a hostname", "TestRubricLLD4_")
m("RUBRIC lld-4 the command opens the store before asking whether the value is a host", "cmd/rashomon/main.go",
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn errors.New(\"--host is not a hostname",
  "\tname, ok := host.Canonical(raw)\n\tif st, err := openStoreForRead(); errors.Is(err, store.ErrNoStore) && st == nil {\n\t\tfmt.Fprintln(stdout, nothingRecordedHere)\n\t\treturn nil\n\t}\n\tif !ok {\n\t\treturn errors.New(\"--host is not a hostname", "TestRubricLLD4_")
