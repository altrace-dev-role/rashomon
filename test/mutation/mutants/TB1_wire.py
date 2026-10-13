# The three-state join. Each break is a way a tokened run silently loses or
# gains rows, and none of them moves a number anyone is already watching.
# THE SEAM MUTATIONS. A review deleted each of these lines and the whole suite
# stayed green -- the feature could be disconnected at three separate points
# while rendering a line that reads as correct. Unit tests on both leaves,
# nothing on the wire between them.
m("TB1 a non-loopback proxy address is accepted", "internal/posture/posture.go",
  "\tif !isLoopback(v.File.ListenAddr) {", "\tif false {", "TestIsLoopback|TestRead_Refuses")
m("TB1 the loopback check matches a prefix", "internal/posture/posture.go",
  "\tswitch strings.ToLower(host) {\n\tcase \"127.0.0.1\", \"::1\", \"localhost\":\n\t\treturn true\n\t}\n\treturn false",
  "\treturn strings.HasPrefix(strings.ToLower(host), \"127.0.0.1\") ||\n\t\tstrings.HasPrefix(strings.ToLower(host), \"localhost\") || host == \"::1\"",
  "TestIsLoopback")

m("TB1 SEAM the token never reaches the join", "internal/report/report.go",
  "\t\tw.RunID = cfg.runToken", "", "TestSeam")
m("TB1 SEAM the counters never cross into the report", "internal/report/destinations.go",
  "\t\tTokenMatched:        obs.TokenMatched,", "\t\tTokenMatched:        0,", "TestSeam")
m("TB1 SEAM the join line is never rendered", "internal/report/text.go",
  "\twriteJoin(b, d)", "", "TestSeam")
m("TB1 SEAM an unverifiable tag is treated as another session",
  "internal/wire/wire.go", "\tif w.IsOurs != nil && w.IsOurs(r.runID) {", "\tif true {",
  "TestSeam|TestToken")
m("TB1 SEAM a foreign dial failure reaches our outcome map", "internal/wire/wire.go",
  "\t\t\tif joinOf(r, w) == joinOther {\n\t\t\t\tcontinue\n\t\t\t}", "", "TestToken")
# RETIRED: "TB1 a token is minted from a posture that was refused". Its line,
# `if v.Export && v.File.SessionToken {`, no longer exists: the mint moved
# inside run's `if v.Export {` branch, so the refused path cannot reach it by
# construction and there is no conjunct left to drop. What the mutant guarded
# -- a refused posture leaking into the report -- is now held by
# "H-27 a refused posture's report reads the store it names" below.
m("TB1 untokened rows are dropped from a tokened run", "internal/wire/wire.go",
  "\tif w.RunID == \"\" || r.runID == \"\" {\n\t\treturn joinWindow\n\t}",
  "\tif w.RunID == \"\" {\n\t\treturn joinWindow\n\t}\n\tif r.runID == \"\" {\n\t\treturn joinOther\n\t}",
  "TestToken")
m("TB1 another run's rows are admitted", "internal/wire/wire.go",
  "\tcase joinOther:\n\t\treturn true", "\tcase joinOther:\n\t\treturn false", "TestToken")
m("TB1 a token-matched row outside the window is inherited", "internal/wire/wire.go",
  "\tcase joinToken:\n\t\t// The token was issued to this process and no other, so it outranks the\n\t\t// clock. A row carrying it outside the window is this run's row with a\n\t\t// bad timestamp -- which is a real case, since the proxy stamps rows\n\t\t// from its own clock and a session can outlive a skew correction.\n\t\treturn false",
  "\tcase joinToken:\n\t\tbreak", "TestToken")
# Re-anchored from `if v.Export && v.File.SessionToken {` to the capability
# check alone, now nested inside run's accepted branch.
m("TB1 the token is sent to a proxy that never advertised it", "cmd/rashomon/main.go",
  "\t\tif v.File.SessionToken {\n\t\t\ttoken = launch.NewToken(runKey)",
  "\t\tif true {\n\t\t\ttoken = launch.NewToken(runKey)", "TestH27")
m("TB1 the token is not carried into the child's environment", "internal/launch/launch.go",
  "\tif token != \"\" {\n\t\taddr = \"http://\" + ProxyUser + \":\" + token + \"@\" + listenAddr\n\t}", "",
  "TestEnv|TestTokenFromProxyURL")
m("TB1 somebody else's proxy credentials are read as a session tag",
  "internal/launch/token.go", "\tif !ok || user != ProxyUser {", "\tif !ok || user == \"\" {",
  "TestTokenFromProxyURL")
m("TB1 the render claims a token join over window-matched rows",
  "internal/report/text.go",
  "\t\tif d.WindowMatched > 0 {\n\t\t\tfmt.Fprintln(b, \"    some clients do not send the proxy credential and are \"+",
  "\t\tif false {\n\t\t\tfmt.Fprintln(b, \"    some clients do not send the proxy credential and are \"+",
  "TestJoinRender")
