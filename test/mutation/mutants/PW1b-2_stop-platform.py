# PW1b-ON-230 step 2: the Stop reads the platform graph (PF-PL8 on the Stop).
# It names, to the person only, each changed package whose go test has not
# run since the edit, at most two and then a pointer to the ledger, on the
# value line and the done flag alike; it reads the log and the digests only
# where the walk file vouches for both, under its own 2 s guard; and the
# full screen says so.
R = "internal/platform/read.go"
V = "internal/recap/value.go"
PL = "internal/recap/platform.go"

m("PW1b-2 the Stop reads no graph", "cmd/rashomon/recap_stop.go",
  "\t\tin.Platform = stopPlatformRead", "\t\t_ = stopPlatformRead", "TestPW1b2_TheStopNames")
m("PW1b-2 met rows print too", PL,
  "if o.State == verify.ObligationOpen || o.State == verify.ObligationUnmet {",
  "if o.State == verify.ObligationOpen || o.State == verify.ObligationUnmet || o.State == verify.ObligationMet {",
  "TestPW1b2_ATestRunAfter")
m("PW1b-2 no cap", PL,
  "const maxPlatformLines = 2", "const maxPlatformLines = 64", "TestPW1b2_TheStopNamesTwo")
m("PW1b-2 the value line alone", "internal/recap/done.go",
  "+ unboundLines(v) + platformLines(v, fromPlugin)\n", "+ unboundLines(v)\n", "TestPW1b2_TheDoneFlag")
m("PW1b-2 the done flag alone", V,
  "\tb.WriteString(platformLines(v, fromPlugin))\n", "", "TestPW1b2_TheStopNamesAChanged|TestPW1b2_TheStopNamesTwo")
m("PW1b-2 the log is not checked", "internal/platform/platform.go",
  '\tif want != "" && hex.EncodeToString(h.Sum(nil)) != want {',
  '\tif false && want != "" && hex.EncodeToString(h.Sum(nil)) != want {', "TestPW1b2_ATornPair")
m("PW1b-2 the digests are not checked", R,
  "\t\t\tif vouched.on && (err != nil || dg.sum != vouched.digests) {",
  "\t\t\tif false && vouched.on && (err != nil || dg.sum != vouched.digests) {", "TestPW1b2_ATornPair")
# A refused walk file is the sync's form with its log line replaced (the
# test's "refused" case), which since W-M1.1's symbols line is five lines, a
# count the reader admits; so the break keys on the missing log line, not on
# a count (the former `len(lines) == 4` matched nothing and survived, W-M1.9).
m("PW1b-2 a refused walk reads as none", "internal/platform/walk.go",
  "\tif (len(lines) != 5 && len(lines) != 6) || lines[0] != walkHeader",
  "\tif len(lines) > 2 && !strings.HasPrefix(lines[2], \"log\\t\") {\n\t\treturn vouch{}, nil\n\t}\n\tif (len(lines) != 5 && len(lines) != 6) || lines[0] != walkHeader", "TestPW1b2_ATornPair")
m("PW1b-2 a bad walk file is ignored", R,
  "\t\tcase err != nil || verr != nil:", "\t\tcase err != nil || errors.Is(verr, fs.ErrClosed):", "TestPW1b2_ATornPair")
m("PW1b-2 the Stop's guard is ignored", R,
  "\t\tguard = run.Deadline", "\t\t_ = run.Deadline", "TestPW1b2_TheStopsGuard")
m("PW1b-2 the screen keeps quiet", "internal/disclose/paragraphs.go",
  "At the end of each reply rashomon reads .pgraph", "At times rashomon reads .pgraph", "TestPW1b2_TheReadIsDisclosed")
