# PR-1: a call's project is the repository it ran in (card PROJECT-RESOLVE1):
# a leading `cd DIR &&` fold, a `git -C DIR` first command, or a file tool's
# absolute path, when the directory is under a .git; the cwd's project for
# anything not proven, two roots in one line, a variable, a line read in part.
# Each break removes one rule and names the tests that hold it.
H = "internal/hook/handle.go"
S = "internal/shape/leading.go"
E = "internal/verify/evaluate.go"
LS = "cmd/rashomon/laststop.go"
m("PR-1 a cd line keeps the cwd's project", H,
  "\tcase len(d.Leading) > 0:\n\t\treturn start\n", "", "TestPR1_|TestPR1c_")
m("PR-1 a file tool's path is not read", H,
  "\t} else if path != nil && filepath.IsAbs(*path) {\n",
  "\t} else if path != nil && false {\n", "TestPR1_")
m("PR-1 a file's repository keeps the path's spelling", H,
  "if d, err := evalSymlinks(existingDir(dir)); err == nil {",
  "if d, err := existingDir(dir), error(nil); err == nil {", "TestPR1_")
m("PR-1 a file's directory not on disk is not walked up", H,
  "\t\tlast = d[:i]\n", "", "TestPR1_|TestPR1d_")
m("PR-1 a file proven outside under no .git is the cwd's", H,
  "\t\t\tkey, _ := walkUp(d, physical, cwd)\n\t\t\treturn key\n",
  "\t\t\tif key, ok := walkUp(d, physical, cwd); ok {\n\t\t\t\treturn key\n\t\t\t}\n", "TestPR1d_")
m("PR-1 a directory under no .git is taken for the project", H,
  "\t\t\treturn \"\", false\n\t\t}\n\t\td = up\n", "\t\t\treturn d, true\n\t\t}\n\t\td = up\n", "TestPR1b_|TestPR1d_")
m("PR-1 two roots in one line are one project", H,
  "\tcase d.Later || start == \"\":\n", "\tcase start == \"\":\n", "TestPR1b_")
m("PR-1 git -C is not read", S,
  "\treturn rest[2].text, false\n", "\treturn \"\", false\n", "TestPR1_")
m("PR-1 a cd past the leading run is not read", S,
  "\t\tif t.text == \"cd\" || t.text == \"pushd\" || t.text == \"popd\" {\n",
  "\t\tif t.text == \"\\x00\" {\n", "TestPR1b_")
m("PR-1 a line read in part is read as whole", S,
  "\tif err != nil || 3*n > len(toks) {\n", "\tif 3*n > len(toks) {\n", "TestPR1b_")
m("PR-1 git -C takes a variable's spelling", S,
  "rest[1].text != \"-C\" || rest[1].quotedAt >= 0 || !dirWord(rest[2]) ||",
  "rest[1].text != \"-C\" || rest[1].quotedAt >= 0 ||", "TestPR1b_")
m("PR-1 git -C twice is read as once", S,
  "\t\tlen(rest) > 3 && rest[3].text == \"-C\" {\n", "\t\tfalse {\n", "TestPR1b_")
m("PR-1 a directory proven outside under no .git is the cwd's", H,
  "fi, err := os.Stat(own); err == nil && fi.IsDir() {", "fi, err := os.Stat(own); err == nil && fi.IsDir() && false {", "TestPR1d_")
m("PR-1 a directory not on disk is taken as proven outside", H,
  "fi, err := os.Stat(own); err == nil && fi.IsDir() {", "fi, err := os.Stat(own); err != nil || fi.IsDir() {", "TestPR1b_")
m("PR-1 a directory inside the cwd under no .git is no project", H,
  "\t\tif d == physical || d == cwd {\n", "\t\tif false {\n", "TestPR1b_")
# The cost (RES-28, manager 07:2x): a file under the cwd is read from the cwd
# down, prefixes of its path, with no symlink resolved; a step that is a
# symlink sends the path to be resolved, since its target may be another
# repository's.
m("PR-1 a file under the cwd is read as outside", H,
  "\t\tif key, ok := underCWD(dir, spelled, physical, cwd); ok {\n", "\t\tif key, ok := underCWD(dir, spelled, physical, cwd); ok && false {\n",
  "TestPC1d_")
m("PR-1 a repository under the cwd is not read", H, "\t\t\tnearest = step\n", "", "TestPR1_")
m("PR-1 a symlink under the cwd is read as spelled", H,
  "\t\tif err == nil && fi.Mode()&fs.ModeSymlink != 0 {\n\t\t\treturn \"\", false\n",
  "\t\tif err == nil && fi.Mode()&fs.ModeSymlink != 0 && false {\n\t\t\treturn \"\", false\n", "TestPR1_")
m("PR-1 a repository under the cwd keeps the path's spelling", H,
  "\treturn physical + nearest[len(base):], true\n", "\treturn nearest, true\n", "TestPR1_")
# The root (manager 07:4x, eng-2's finding): a judgment has one project, the
# one its spec and inferred gate were read from, the Stop's and the post's
# cwd key, and the replay takes the one the Stop's checkpoint recorded.
m("PR-1 evaluate takes the root from the declarations", E,
  "root == \"\" || latest == \"\" {\n", "true {\n", "TestPR1e_|TestPR1f_")
m("PR-1 a v4 record's judgment names the cwd's project", E,
  "root == \"\" || latest == \"\" {\n", "root == \"\" {\n", "TestSTR43_")
m("PR-1 the Stop passes no root", "cmd/rashomon/recap_stop.go",
  "Tail: []byte(p.LastMessageTail), Root: key,", "Tail: []byte(p.LastMessageTail),", "TestPR1e_")
m("PR-1 the Stop view drops the root", "internal/verify/stop.go",
  "in.Inferred, in.Root, pt, now, &c)", "in.Inferred, \"\", pt, now, &c)", "TestPR1e_")
m("PR-1 the post passes no root", "internal/hook/steerstep.go",
  "p.st.Key(), key, pt, env.Now,", "p.st.Key(), \"\", pt, env.Now,", "TestPR1f_")
m("PR-1 the steer view drops the root", "internal/verify/steer.go",
  "evaluate(run, idx, sp, key, nil, root, pt, now, &c)", "evaluate(run, idx, sp, key, nil, \"\", pt, now, &c)", "TestPR1f_")
m("PR-1 the replay ignores the checkpoint's root", LS,
  "\tif cp != nil {\n\t\troot = cp.ProjectRoot\n", "\tif false {\n\t\troot = cp.ProjectRoot\n", "TestPR1e_")
m("PR-1 the replay judges under the declarations' root", LS, "\tin.Root = root\n", "", "TestPR1e_")
