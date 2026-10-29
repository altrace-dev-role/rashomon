# VER-3.1 discover: spec discovery, the managed spec and the merge (README 3.1,
# card 3.1 (b)). Each break removes one rule of the walk, the ownership check
# or the merge, and names the test that holds it. TestVER41 is written out
# whole: verify has a TestVER41 of its own.
D = "internal/spec/discover.go"
O = "internal/spec/owner_unix.go"
E = "internal/verify/evaluate.go"
SP5 = "TestSP5_NearestRepoSpecWins"
SP6 = "TestSP6_WalkDepthCapped"
SP7 = "TestSP7_AdditiveMergeManagedWinsId"
SP8 = "TestSP8_CiIdSpaces"
SP9 = "TestSP9_DigestOverSourcesInOrder"
SP10 = "TestSP10_TrustIsLowestFolded"
SP11 = "TestSP11_BrokenRepoSpecRecordedNotFatal"
SP12 = "TestSP12_NoSpecIsNil"
SP12b = "TestSP12b_OSFSReadsOnlyARegularFileToTheCap"
SP13 = "TestSP13_EvaluateSeesDiscovered"
V38 = "TestVER38_DiscoveryNeverAboveRepoRoot"
V38b = "TestVER38b_ClaudeProjectDirStopsTheWalk"
V41 = "TestVER41_ManagedSpecMustBeRootOwned"
V41b = "TestVER41b_OwnerIsLstats"
MS5j = "TestMS5j_ManagedHoldoutWins"

m("VER-3.1 the walk continues above root", D, "\t\tif dir == root {\n\t\t\tbreak\n\t\t}\n", "", V38)
m("VER-3.1 a cwd outside root is walked", D, "\tif !within(cwd, root) {\n\t\treturn nil\n\t}\n", "", V38)
m("VER-3.1 a relative path is walked", D, "\tif !filepath.IsAbs(cwd) || !filepath.IsAbs(root) {\n\t\treturn nil\n\t}\n", "", V38)
m("VER-3.1 the farthest spec wins", D, "\tfor _, path := range repoCandidates {", "\tfor _, path := range slices.Backward(repoCandidates) {", SP5)
m("VER-3.1 CLAUDE_PROJECT_DIR is ignored", D, "[2]string{projectRoot, projectDir}", "[2]string{projectRoot, projectRoot}", V38b)
m("VER-3.1 the farther root stops the walk", D, "within(cwd, r) && len(r) > len(root)", 'within(cwd, r) && (root == "" || len(r) < len(root))', V38b)
m("VER-3.1 a relative cwd has a root", D, '\tif !filepath.IsAbs(cwd) {\n\t\treturn ""\n\t}\n\tcwd = filepath.Clean(cwd)\n', "\tcwd = filepath.Clean(cwd)\n", V38b)
m("VER-3.1 the depth cap is removed", D, "len(out) < MaxWalkDepth;", "true;", SP6)
m("VER-3.1 the depth cap is 65", D, "const MaxWalkDepth = 64\n", "const MaxWalkDepth = 65\n", SP6)
m("VER-3.1 the managed file's uid is not checked", D, "file := fileUID == 0 && ", "file := ", V41)
m("VER-3.1 a group-writable managed file is accepted", D, "fileMode.Perm()&0o022 == 0", "fileMode.Perm()&0o002 == 0", V41)
m("VER-3.1 a symlinked managed file is accepted", D, "fileMode.IsRegular() && ", "", V41)
m("VER-3.1 the managed directory is not checked", D, "\treturn file && dir\n", "\treturn file && (dir || true)\n", V41)
m("VER-3.1 the managed directory's uid is not checked", D, "dir := dirUID == 0 && ", "dir := ", V41)
m("VER-3.1 the not-admin-owned line is dropped", D, '\t\td.c.Errors = append(d.c.Errors, path+": managed spec ignored: not admin-owned")\n', "", V41)
m("VER-3.1 Owner follows a symlink", O, "os.Lstat(path)", "os.Stat(path)", V41b)
m("VER-3.1 the repository overrides a managed gate id", D, "if slices.ContainsFunc(c.Gates, func(d Declared) bool { return d.ID == id }) {", "if false {", SP7)
m("VER-3.1 the repository overrides a managed rule id", D, "if slices.ContainsFunc(c.Rules, func(d DeclaredRule) bool { return d.ID == id }) {", "if false {", SP7)
m("VER-3.1 the repository overrides a managed fact id", D, "if slices.ContainsFunc(c.Facts, func(d DeclaredFact) bool { return d.ID == id }) {", "if false {", SP7)
m("VER-3.1 the repository's holdout wins", D, "\tif c.Holdout.RateBP == 0 {\n\t\tc.Holdout = f.Holdout\n", "\tif f.Holdout.RateBP != 0 {\n\t\tc.Holdout = f.Holdout\n", MS5j)
m("VER-3.1 the ci:managed prefix is missing", D, 'ci = "ci:managed:"', 'ci = "ci:"', SP8)
m("VER-3.1 a ci_commands entry is not required", D, "required := true", "required := false", SP8)
m("VER-3.1 the source digests are sorted before hashing", D, "\tfor _, s := range c.Sources {\n\t\tio.WriteString(h, s.Digest)", "\tfor _, s := range slices.SortedFunc(slices.Values(c.Sources), func(a, b Source) int { return strings.Compare(a.Digest, b.Digest) }) {\n\t\tio.WriteString(h, s.Digest)", SP9)
m("VER-3.1 the trust fold reads the first source only", D, "range c.Sources[1:]", "range c.Sources[1:1]", SP10)
m("VER-3.1 an implied source is untrusted", D, "\t\treturn Builtin\n", "\t\treturn Untrusted\n", SP10)
m("VER-3.1 a broken spec falls back to the parent's", D, "\t\td.add(path, data, Untrusted)\n\t\tbreak\n", "\t\td.add(path, data, Untrusted)\n\t\tif len(d.c.Errors) == 0 {\n\t\t\tbreak\n\t\t}\n", SP11)
m("VER-3.1 a refused file's Errors line is dropped", D, '\t\td.c.Errors = append(d.c.Errors, path+": "+err.Error())\n', "", SP11)
m("VER-3.1 nil when only errors exist", D, "if len(d.c.Sources) == 0 && len(d.c.Errors) == 0 {", "if len(d.c.Sources) == 0 {", SP11)
m("VER-3.1 a repository I/O error reads as no file", D, "\t\tif errors.Is(err, fs.ErrNotExist) {\n\t\t\tcontinue\n", "\t\tif err != nil {\n\t\t\tcontinue\n", SP12)
m("VER-3.1 a managed I/O error reads as no file", D, "\tuid, mode, err := fsys.Owner(path)\n\tif errors.Is(err, fs.ErrNotExist) {", "\tuid, mode, err := fsys.Owner(path)\n\tif err != nil {", SP12)
m("VER-3.1 OSFS opens what is not a regular file", D, "\tif !fi.Mode().IsRegular() {\n", "\tif !fi.Mode().IsRegular() && false {\n", SP12b)
m("VER-3.1 OSFS reads past the cap", D, "io.ReadAll(io.LimitReader(f, MaxBytes+1))", "io.ReadAll(f)", SP12b)
m("VER-3.1 the snapshot drops the spec's Errors", E, "Errors: append([]string{}, sp.Errors...)}", "Errors: []string{}}", SP13)
