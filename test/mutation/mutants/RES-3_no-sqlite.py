# RES-3: the binary links neither modernc.org/sqlite nor modernc.org/libc, and
# the notices file says what ships. The notices checker's "nothing linked"
# branch must not swallow a linked module. With the condition always true,
# every dependency list is judged as an empty one, so the file is required to
# say nothing ships: the fixtures that link a module and carry its notice go
# red. The fixtures that expect red stay red, because the clauses before the
# branch still run.
m("RES-3 the notices checker reads every dependency list as empty", "test/acceptance/notices_test.go",
  "\tif len(mods) == 0 {\n\t\tif !strings.Contains(text, noNoticesSentence) {\n",
  "\tif true {\n\t\tif !strings.Contains(text, noNoticesSentence) {\n", "TestThirdPartyNotices_CheckerFailsClosed")
# The driver comes back as a requirement nothing imports: still a module every
# reader and `go mod tidy` sees, and the next import brings the subtree back
# with no go.mod change to review. An unused require builds without a go.sum
# entry (measured), so the mutant compiles and the test alone has to catch it.
m("RES-3 go.mod requires the SQLite driver again", "go.mod",
  "go 1.25.0\n", "go 1.25.0\n\nrequire modernc.org/sqlite v1.60.1\n", "TestRES3_")
# The notices file is checked against a reader and a checker, and both have to
# be shown to bite in a tree that links nothing. The reader: dropping every
# module it is handed reads exactly like a correct reader of this tree. The
# checker: a linked module the file does not list, with the licence text
# present, is caught by the missing-module clause alone.
m("RES-3 the notices reader drops every module", "test/acceptance/notices_test.go",
  "\t\t\tmods[p.module] = p.version\n", "", "TestThirdPartyNotices_ReadsEveryLinkedModule")
m("RES-3 the notices checker misses a linked module the file does not list", "test/acceptance/notices_test.go",
  '\t\tif !strings.Contains(text, path+" "+ver) {',
  '\t\tif false && !strings.Contains(text, path+" "+ver) {',
  "TestThirdPartyNotices_CheckerFailsClosed")
