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
  "\t\tif resolved, err := evalSymlinks(d); err == nil {\n\t\t\treturn resolved\n",
  "\t\tif _, err := evalSymlinks(d); err == nil {\n\t\t\treturn d\n", "TestPR1_")
m("PR-1 a file's directory not on disk is not walked up", H,
  "\t\td = up\n", "\t\treturn \"\"\n", "TestPR1_|TestPR1d_")
m("PR-1 a file proven outside under no .git is the cwd's", H,
  "\t\t\tif !inside(k, cwd) {\n", "\t\t\tif false {\n", "TestPR1d_")
m("PR-1 a directory under no .git is taken for the project", H,
  "if root := gitRoot(baseline.KeyOf(own)); root != \"\" {",
  "if root := baseline.KeyOf(own); root != \"\" {", "TestPR1b_|TestPR1d_")
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
  "err == nil && fi.IsDir() && !inside(baseline.KeyOf(own), cwd) {\n",
  "err == nil && fi.IsDir() && false {\n", "TestPR1d_")
m("PR-1 a directory not on disk is taken as proven outside", H,
  "err == nil && fi.IsDir() && !inside(baseline.KeyOf(own), cwd) {\n",
  "(err != nil || fi.IsDir()) && !inside(baseline.KeyOf(own), cwd) {\n", "TestPR1b_")
m("PR-1 a directory inside the cwd under no .git is no project", H,
  "\treturn key == root || strings.HasPrefix(key, strings.TrimSuffix(root, \"/\")+\"/\")\n",
  "\treturn key == root\n", "TestPR1b_|TestPR1d_")
# The root (manager 07:4x, eng-2's finding): a judgment has one project, the
# one its spec and inferred gate were read from, the Stop's and the post's
# cwd key, and the replay takes the one the Stop's checkpoint recorded.
m("PR-1 evaluate takes the root from the declarations", E,
  "\tif root == \"\" {\n\t\troot = projectRoot(run.Declarations, at)\n",
  "\tif true {\n\t\troot = projectRoot(run.Declarations, at)\n", "TestPR1e_|TestPR1f_")
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
