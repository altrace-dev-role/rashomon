# VER-3.1 hook: the call sites take the discovered spec (card 3.1 (c), D119,
# D128). Each break removes one step of DiscoverSpec's resolution, its flag,
# the steer step's or the Stop path's use of the spec, or the holdout's read
# of it, and names the test that holds it. The last row is the director's
# (c) addition: OSFS's FIFO refusal, pinned by TestVER41c.
H = "internal/hook/specdiscover.go"
S = "internal/hook/steerstep.go"
V = "internal/verify/steer.go"
D = "internal/spec/discover.go"
R = "cmd/rashomon/recap_stop.go"
C = "cmd/rashomon/steerenv.go"
L = "internal/settings/locate.go"
SP15 = "TestSP15_HookDiscoversTheNearestSpec"
SP15b = "TestSP15b_SteerEnvCarriesTheSpecDirs"
SP15c = "TestSP15c_StopJudgesTheDiscoveredSpec"
SP16 = "TestSP16_BrokenSpecIsOneFlagNotAStop"
MS5h = "TestMS5h_HeldOutReadsTheSpec"
V41c = "TestVER41c_OSFSRefusesAFIFO"
m("VER-3.1 the hook passes nil with a spec present", S, "view := verify.SteerAt(run, idx, p.sp, pt", "view := verify.SteerAt(run, idx, nil, pt", SP15)
m("VER-3.1 the hook does not discover", S, "\tp.sp = discoverSpecAt(p.cwd, key, env.ProjectDir, env.ManagedDir)\n", "", SP15)
m("VER-3.1 the steer view drops the spec", V, "Dropped: dropped, Spec: snap.Spec}", "Dropped: dropped}", SP15)
m("VER-3.1 a symlinked cwd walked as given", H, "\tcwd = resolved(cwd)\n", "", SP15)
m("VER-3.1 resolved evaluates no symlink", H, "if r, err := filepath.EvalSymlinks(p); err == nil {", "if r, err := filepath.EvalSymlinks(p); err == nil && false {", SP15)
m("VER-3.1 CLAUDE_PROJECT_DIR unresolved", H, "gitRoot(key), resolved(projectDir))", "gitRoot(key), projectDir)", SP15)
m("VER-3.1 CLAUDE_PROJECT_DIR ignored", H, "gitRoot(key), resolved(projectDir))", "gitRoot(key), \"\")", SP15)
m("VER-3.1 no .git makes cwd the root", H, "if _, err := os.Lstat(filepath.Join(key, \".git\")); err != nil {", "if _, err := os.Lstat(filepath.Join(key, \".git\")); err != nil && false {", SP15)
m("VER-3.1 the managed directory ignored", H, "\tif managedDir != \"\" {", "\tif false {", SP15)
m("VER-3.1 the managed directory read as the file", H, "managed = filepath.Join(managedDir, \"rashomon\", \"spec.json\")", "managed = managedDir", SP15)
m("VER-3.1 an I/O error is no flag", H, "\t\treturn spec.Unreadable(err.Error())", "\t\treturn nil", SP16)
m("VER-3.1 no cwd is no flag", H, "\t\treturn spec.Unreadable(errNoCWD)", "\t\treturn nil", SP16)
m("VER-3.1 an unreadable discovery has no line", D, "d := discovery{c: Compiled{Errors: []string{reason}}}", "d := discovery{}", SP16)
m("VER-3.1 heldOut ignores the spec's holdout", S, "\t\ton, rate := p.sp.Holdout.On(env.Now)\n\t\tif !on {\n\t\t\treturn false\n\t\t}\n\t\tbp = rate\n", "", MS5h)
m("VER-3.1 heldOut ignores the spec's rate", S, "\t\tbp = rate\n", "\t\t_ = rate\n", MS5h)
m("VER-3.1 the Stop path passes nil", R, "v := verify.EvaluateStop(run, idx, sp, pt, now, in)", "v := verify.EvaluateStop(run, idx, nil, pt, now, in); _ = sp", SP15c)
m("VER-3.1 the Stop path ignores its cwd", R, "sp := hook.DiscoverSpec(p.CWD, projectDir, managedDir)", "sp := hook.DiscoverSpec(\"\", projectDir, managedDir)", SP15c)
m("VER-3.1 CLAUDE_PROJECT_DIR not read", C, "return os.Getenv(\"CLAUDE_PROJECT_DIR\"), settings.ManagedDir()", "return \"\", settings.ManagedDir()", SP15b)
m("VER-3.1 the managed directory is the settings file", L, "return filepath.Dir(managedPath())", "return managedPath()", SP15b)
m("VER-3.1 OSFS opens a FIFO", D, "\tif !fi.Mode().IsRegular() {\n", "\tif !fi.Mode().IsRegular() && false {\n", V41c)
