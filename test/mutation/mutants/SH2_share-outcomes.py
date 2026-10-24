# SH2 (Task 4.3): the outcomes bundle. Counts under five print "<5", totals are
# summed before that, every string is a closed set or a fixed form, the
# preview is the file, and the package links nothing that can reach a socket.
m("SH2 a count of four prints as a number", "internal/share/outcomes.go",
  "\tif c < MinCount {", "\tif c < MinCount-1 {",
  "TestSH2_SmallCountsPrintAsLessThanFive")
m("SH2 a count of five prints as small", "internal/share/outcomes.go",
  "\tif c < MinCount {", "\tif c < MinCount+1 {",
  "TestSH2_SmallCountsPrintAsLessThanFive")
m("SH2 totals leave out the small patterns", "internal/share/outcomes.go",
  "\t\tfired, fixed, ignored, overridden = fired+t.Fired, fixed+t.Fixed, ignored+t.Ignored, overridden+t.Overridden\n",
  "\t\tif t.Fired >= MinCount {\n\t\t\tfired, fixed, ignored, overridden = fired+t.Fired, fixed+t.Fixed, ignored+t.Ignored, overridden+t.Overridden\n\t\t}\n",
  "TestSH2_TotalsAreSummedBeforeSuppression")
m("SH2 rows follow the reader's order", "internal/share/outcomes.go",
  "\tfor _, id := range pattern.All() {\n",
  "\tfor k := len(pattern.All()) - 1; k >= 0; k-- {\n\t\tid := pattern.All()[k]\n",
  "TestSH2_OutputIsDeterministic")
m("SH2 no tallies is null, not a list", "internal/share/outcomes.go",
  "ByPattern: []PatternRow{}}", "ByPattern: nil}",
  "TestSH2_OutputIsDeterministic")
m("SH2 a pattern outside the closed set is taken", "internal/share/outcomes.go",
  "\t\tif !pattern.Valid(t.Pattern) {", "\t\tif false {",
  "TestSH2_ReaderIsClosed")
m("SH2 a pattern may be counted twice", "internal/share/outcomes.go",
  "\t\tif _, dup := byID[t.Pattern]; dup {", "\t\tif _, dup := byID[t.Pattern]; false && dup {",
  "TestSH2_ReaderIsClosed")
m("SH2 more tallies than patterns", "internal/share/outcomes.go",
  "\tif len(o.Tallies) > len(pattern.All()) {", "\tif false {",
  "TestSH2_ReaderIsClosed")
m("SH2 labelled may pass fired", "internal/share/outcomes.go",
  "\tif t.True > t.Labelled || t.Labelled > t.Fired {", "\tif t.True > t.Labelled {",
  "TestSH2_ReaderIsClosed")
m("SH2 true may pass labelled", "internal/share/outcomes.go",
  "\tif t.True > t.Labelled || t.Labelled > t.Fired {", "\tif t.Labelled > t.Fired {",
  "TestSH2_ReaderIsClosed")
m("SH2 a count may be negative", "internal/share/outcomes.go",
  "\t\tif n < 0 || n > MaxCount {\n\t\t\treturn fmt.Errorf(\"share: %s reports", "\t\tif n > MaxCount {\n\t\t\treturn fmt.Errorf(\"share: %s reports",
  "TestSH2_ReaderIsClosed")
m("SH2 a count may pass the cap", "internal/share/outcomes.go",
  "\t\tif n < 0 || n > MaxCount {\n\t\t\treturn fmt.Errorf(\"share: %s reports", "\t\tif n < 0 {\n\t\t\treturn fmt.Errorf(\"share: %s reports",
  "TestSH2_ReaderIsClosed")
m("SH2 a total may pass the cap", "internal/share/outcomes.go",
  "\t\tif n > MaxCount {\n\t\t\treturn Bundle{}, fmt.Errorf(\"share: a total", "\t\tif false {\n\t\t\treturn Bundle{}, fmt.Errorf(\"share: a total",
  "TestSH2_ReaderIsClosed")
m("SH2 a digest may be any length", "internal/share/outcomes.go",
  "\tif len(s) != digestLen {", "\tif false {",
  "TestSH2_ReaderIsClosed")
m("SH2 a digest may be upper case", "internal/share/outcomes.go",
  "c < '0' || c > '9' && c < 'a' || c > 'f'", "c < '0' || c > '9' && c < 'A' || c > 'f'",
  "TestSH2_ReaderIsClosed")
m("SH2 a period end may be any text", "internal/share/outcomes.go",
  "\t\tif err != nil {\n\t\t\treturn errors.New(\"share: a period end", "\t\tif err != nil && false {\n\t\t\treturn errors.New(\"share: a period end",
  "TestSH2_ReaderIsClosed")
m("SH2 a period may run backwards", "internal/share/outcomes.go",
  "\tif days[0].After(days[1]) {", "\tif false {",
  "TestSH2_ReaderIsClosed")
m("SH2 the file escapes the less-than sign", "internal/share/outcomes.go",
  "\tenc.SetEscapeHTML(false)", "\tenc.SetEscapeHTML(true)",
  "TestSH2_SmallCountsPrintAsLessThanFive")
m("SH2 the bundle gains a free-text field", "internal/share/outcomes.go",
  "\tByPattern     []PatternRow `json:\"by_pattern\"`\n}", "\tByPattern     []PatternRow `json:\"by_pattern\"`\n\tNote          string       `json:\"note\"`\n}",
  "TestSH2b_NoFreeTextField")
m("SH2 the pattern row gains a free-text field", "internal/share/outcomes.go",
  "\tPrecision Precision  `json:\"precision\"`\n}", "\tPrecision Precision  `json:\"precision\"`\n\tExample   string     `json:\"example\"`\n}",
  "TestSH2b_NoFreeTextField")
m("SH2 the command builds without the unit on", "cmd/rashomon/share.go",
  "\tif !on {", "\tif !on && false {",
  "TestSH2_RefusalsWriteAndShowNothing")
m("SH2 the file is not the preview", "cmd/rashomon/share.go",
  "\tif _, err := f.Write(data); err != nil {", "\tif _, err := f.Write(data[:len(data)-1]); err != nil {",
  "TestSH2_PreviewEqualsWritten")
m("SH2 the hash is of something else", "cmd/rashomon/share.go",
  "\tsum := sha256.Sum256(data)", "\tsum := sha256.Sum256(data[1:])",
  "TestSH2_PreviewEqualsWritten")
m("SH2 an existing file is overwritten", "cmd/rashomon/share.go",
  "os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)", "os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)",
  "TestSH2_RefusalsWriteAndShowNothing")
m("SH2 the file is readable by others", "cmd/rashomon/share.go",
  "os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)", "os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)",
  "TestSH2_PreviewEqualsWritten")
m("SH2 another bundle is built", "cmd/rashomon/share.go",
  "args[0] != \"build\" || args[1] != \"outcomes\"", "args[0] != \"build\"",
  "TestSH2_RefusalsWriteAndShowNothing")
m("SH2 an empty file name is taken", "cmd/rashomon/share.go",
  "\tif out == \"\" {", "\tif false {",
  "TestSH2_RefusalsWriteAndShowNothing")
m("SH2 an extra argument is taken", "cmd/rashomon/share.go",
  "\tcase len(rest) == 2 && rest[0] == \"--out\":", "\tcase len(rest) >= 2 && rest[0] == \"--out\":",
  "TestSH2_RefusalsWriteAndShowNothing")
# TRU-40b: a blank import is the cheapest way a bundle writer reaches a socket.
m("SH2 the share package imports net", "internal/share/outcomes.go",
  "\t\"strconv\"\n", "\t_ \"net\"\n\t\"strconv\"\n", "TestTRU40b_ShareLinksNoNetworkPackage")
m("SH2 the share package imports net/http", "internal/share/outcomes.go",
  "\t\"strconv\"\n", "\t_ \"net/http\"\n\t\"strconv\"\n", "TestTRU40b_ShareLinksNoNetworkPackage")
m("SH2 the ban stops seeing net", "test/acceptance/tru40b_share_no_network_test.go",
  "\t\tif _, bad := forbiddenImports[p]; bad {", "\t\tif _, bad := forbiddenImports[p]; bad && p != \"net\" {",
  "TestTRU40b_TheBanSeesTheNetworkPackages")
m("SH2 the ban sees a name that contains net", "test/acceptance/tru40b_share_no_network_test.go",
  "\t\tif _, bad := forbiddenImports[p]; bad {", "\t\tif _, bad := forbiddenImports[p]; bad || strings.Contains(p, \"net\") {",
  "TestTRU40b_TheBanSeesTheNetworkPackages")
m("SH2 the unwired reader reports an empty store as data", "cmd/rashomon/share.go",
  "return share.Outcomes{}, errNoOutcomes }", "return share.Outcomes{}, nil }",
  "TestSH2_CmdShareReadsTheStoreRoot")
m("SH2 cmdShare reads consent from somewhere other than the store root", "cmd/rashomon/share.go",
  "\treturn shareBuild(root, noOutcomes{}, args, stdout)", "\treturn shareBuild(os.TempDir(), noOutcomes{}, args, stdout)",
  "TestSH2_CmdShareReadsTheStoreRoot")
