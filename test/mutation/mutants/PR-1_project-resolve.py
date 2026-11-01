# PR-1: a call's project is the repository it ran in (card PROJECT-RESOLVE1):
# a leading `cd DIR &&` fold, a `git -C DIR` first command, or a file tool's
# absolute path, when the directory is under a .git; the cwd's project for
# anything not proven, two roots in one line, a variable, a line read in part.
# Each break removes one rule and names the tests that hold it.
H = "internal/hook/handle.go"
S = "internal/shape/leading.go"
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
