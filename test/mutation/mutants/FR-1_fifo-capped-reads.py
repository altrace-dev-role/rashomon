# FR-1 (PR-1): every git file baseline reads without
# running git (the .git pointer, commondir, config) and both conflict-base
# readers open through regfile.Open, so a named pipe is a miss at once and
# never a wait; each git read is capped at MaxGitFileBytes and a file over
# it is refused, never read in part; a refused read is no id on the hook
# path and an error off it (graph index, graph knowledge import, headCommit),
# never the path form, C1, or the worktree's own config. A pipe row dies as
# a named FAIL at its test's 5 s deadline, not at go test's timeout.
B = "internal/baseline/repoid.go"
C = "cmd/rashomon/conflictbase.go"
G = "cmd/rashomon/graph.go"
PIPE = "^TestRepoID_ANamedPipeGivesNoID$"
CAP = "^TestRepoID_EachGitReadIsCapped$"
CBPIPE = "^TestConflictBase_ANamedPipeIsAMissNotAWait$"
# A read with regfile.Open's refusal and no cap: the site's own cap dropped,
# the other two sites' kept.
UNCAPPED = ("func(p string) ([]byte, error) { f, err := regfile.Open(p); if err != nil { return nil, err }; "
            "defer f.Close(); return io.ReadAll(f) }")

m("FR-1 the prompt's conflict-base head opens a named pipe and waits", C,
  "\tf, err := regfile.Open(path)\n\tif err != nil {\n\t\treturn conflictBaseHead{}, false\n",
  "\tf, err := os.Open(path)\n\tif err != nil {\n\t\treturn conflictBaseHead{}, false\n",
  CBPIPE)
m("FR-1 the post's conflict base opens a named pipe and waits", C,
  "\tf, err := regfile.Open(path)\n\tif err != nil {\n\t\treturn conflictBase{}, false\n",
  "\tf, err := os.Open(path)\n\tif err != nil {\n\t\treturn conflictBase{}, false\n",
  CBPIPE)
m("FR-1 the .git pointer is read with os.ReadFile", B,
  "\t\tb, err := readGitFile(dir)\n",
  "\t\tb, err := os.ReadFile(dir)\n",
  PIPE)
m("FR-1 commondir is read with os.ReadFile", B,
  "readGitFile(filepath.Join(dir, \"commondir\"))",
  "os.ReadFile(filepath.Join(dir, \"commondir\"))",
  PIPE)
m("FR-1 config is read with os.ReadFile", B,
  "readGitFile(filepath.Join(common, \"config\"))",
  "os.ReadFile(filepath.Join(common, \"config\"))",
  PIPE)
m("FR-1 the .git pointer read has no cap", B,
  "\t\tb, err := readGitFile(dir)\n",
  "\t\tb, err := " + UNCAPPED + "(dir)\n",
  CAP)
m("FR-1 the commondir read has no cap", B,
  "readGitFile(filepath.Join(dir, \"commondir\"))",
  UNCAPPED + "(filepath.Join(dir, \"commondir\"))",
  CAP)
m("FR-1 the config read has no cap", B,
  "readGitFile(filepath.Join(common, \"config\"))",
  UNCAPPED + "(filepath.Join(common, \"config\"))",
  CAP)
m("FR-1 readGitFile keeps a file over the cap", B,
  "\tif len(b) > MaxGitFileBytes {\n",
  "\tif len(b) > MaxGitFileBytes+1 {\n",
  CAP)
m("FR-1 a refused commondir falls back to the git directory", B,
  "\tcase !absent(err):\n\t\treturn \"\", \"\", err\n",
  "\tcase !absent(err):\n",
  "^TestGitDirs_RefusedCommondirIsNoFallback$")
m("FR-1 RepoIDErr gives the path form for a refused read", B,
  "\turl, ok, err := originURL(key)\n\tif err != nil {\n\t\treturn \"\", true, err\n\t}\n",
  "\turl, ok, _ := originURL(key)\n",
  "^TestKeptGoEdit_RefusedGitConfigCapturesNothing$")
m("FR-1 headCommit gives C1 for a refused read", G,
  "\t\treturn \"\", err\n\t}\n\thead, err := os.ReadFile(filepath.Join(gitDir, \"HEAD\"))\n",
  "\t\treturn \"C1\", nil\n\t}\n\thead, err := os.ReadFile(filepath.Join(gitDir, \"HEAD\"))\n",
  "^TestHeadCommit_RefusalIsAnErrorNotC1$")
m("FR-1 graph index keys the repository node under the path form", "internal/graph/index/index.go",
  "\trid, ephemeral, err := baseline.RepoIDErr(root)\n\tif err != nil {\n\t\treturn Result{}, err\n\t}\n",
  "\trid, ephemeral := baseline.RepoID(root)\n",
  "^TestBuild_RefusedGitReadFailsLoud$")
m("FR-1 graph knowledge import keys rows under no id", G,
  "\trid, _, err := baseline.RepoIDErr(root)\n\tif err != nil {\n\t\treturn err\n\t}\n",
  "\trid, _ := baseline.RepoID(root)\n",
  "^TestGraphVerbs_RefusedGitReadFailsLoud$")
# The first is killed by the marker check the test had; the second
# keeps no marker and no @ and is killed only by the url pin: each
# result is host.example/org/repo or "".
m("FR-1 NormalizeOrigin cuts the host at its first @", B,
  "\tif i := strings.LastIndex(host, \"@\"); i >= 0 {\n",
  "\tif i := strings.Index(host, \"@\"); i >= 0 {\n",
  "^TestNormalizeOrigin_KeepsNoUserPart$")
m("FR-1 NormalizeOrigin gives the host alone for a url with two @", B,
  "\treturn host + \"/\" + path\n}\n",
  "\tif strings.Count(whole, \"@\") > 1 {\n\t\treturn strings.ToLower(host)\n\t}\n\treturn host + \"/\" + path\n}\n",
  "^TestNormalizeOrigin_KeepsNoUserPart$")
IMPORTS["FR-1 a path through a regular file is refused, not absent"] = (B, "\t\"syscall\"\n", "")
m("FR-1 a path through a regular file is refused, not absent", B,
  "\treturn errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)\n",
  "\treturn errors.Is(err, fs.ErrNotExist)\n",
  "^TestRepoID_APathThroughAFileIsAbsent$")
m("FR-1 readGitFile refuses a symlinked config", B,
  "\tf, err := regfile.Open(p)\n",
  "\tif st, err := os.Lstat(p); err == nil && st.Mode()&fs.ModeSymlink != 0 {\n"
  "\t\treturn nil, fs.ErrPermission\n\t}\n\tf, err := regfile.Open(p)\n",
  "^TestRepoID_SymlinkedConfigReadsTheOrigin$")
