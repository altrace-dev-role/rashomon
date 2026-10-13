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
