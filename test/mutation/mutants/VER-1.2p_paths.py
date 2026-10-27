# The path producers: the hook fills a declaration's cwd, project and
# file_path. Each assignment dropped, the table widened or narrowed, the wrong
# field read, and the project left unresolved, each judged by the test that
# names that field. The two edits that would leave the baseline import unused
# keep a call to it, so the break is a behaviour, not a build failure.
m("VER-1.2p the declaration's cwd is not filled", "internal/hook/handle.go",
  "\tdecl.CWD = nilIfEmpty(p.CWD)\n", "", "TestPP1_|TestPP6_")
m("VER-1.2p the declaration's project is not filled", "internal/hook/handle.go",
  "\tdecl.Project = nilIfEmpty(baseline.Key(p.CWD))\n", "\t_ = baseline.Key(p.CWD)\n", "TestPP1_|TestPP6_|TestPP8_")
m("VER-1.2p the declaration's file_path is not filled", "internal/hook/handle.go",
  "\tdecl.FilePath = shape.DeclaredPath(p.ToolName, p.ToolInput)\n", "", "TestPP2_|TestPP3_|TestPP4_|TestH44_")
m("VER-1.2p the project is the cwd unresolved", "internal/hook/handle.go",
  "nilIfEmpty(baseline.Key(p.CWD))", "nilIfEmpty(p.CWD + baseline.Key(\"\"))", "TestPP1_")
m("VER-1.2p MultiEdit carries no path", "internal/shape/declpath.go",
  "\t\"MultiEdit\":    \"file_path\",\n", "", "TestPP4_|TestDeclaredPath_")
m("VER-1.2p Read carries its path", "internal/shape/declpath.go",
  "\t\"Edit\":         \"file_path\",\n", "\t\"Edit\":         \"file_path\",\n\t\"Read\":         \"file_path\",\n",
  "TestDeclaredPath_TableIsClosed")
m("VER-1.2p NotebookEdit's path is read from file_path", "internal/shape/declpath.go",
  "\t\"NotebookEdit\": \"notebook_path\",\n", "\t\"NotebookEdit\": \"file_path\",\n", "TestPP3_|TestDeclaredPath_")
