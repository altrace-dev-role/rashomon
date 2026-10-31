# VER-40: `rashomon spec trust` binds a repository spec by its digest (card
# SPEC-TRUST1, PRD PF-SP2). One break per promise the binding check keeps: an
# edit (a new digest) still reads as trusted, the same bytes at another path
# of the project read as trusted, and a bindings file over its cap is read.
# Each term is weakened, never deleted. The kill rows are discovery's own
# test through the steer step, and the verb's edit test.
T = "internal/spec/trust.go"
HOLDS = "\t\treturn x.ProjectRoot == projectRoot && x.SpecPath == specPath && x.SpecDigest == digest\n"

m("VER-40 an edited spec stays trusted", T, HOLDS,
  "\t\treturn x.ProjectRoot == projectRoot && x.SpecPath == specPath && (x.SpecDigest == digest || true)\n",
  "TestST1d_DiscoveryReadsTheBinding|TestST1b_EditDropsTrust")
m("VER-40 a binding holds for another path", T, HOLDS,
  "\t\treturn x.ProjectRoot == projectRoot && (x.SpecPath == specPath || true) && x.SpecDigest == digest\n",
  "TestST1d_DiscoveryReadsTheBinding")
m("VER-40 a bindings file over its cap is read", T, "\tif len(data) > MaxTrustBytes {\n",
  "\tif len(data) > MaxTrustBytes && false {\n", "TestST1d_DiscoveryReadsTheBinding")

# The bindings file is a store file (D6): purge takes it and its replacement.
# Each entry is dropped; the inventory's rows are TRU-23's mutants.
P = "internal/purge/purge.go"
m("VER-40 purge leaves the bindings file", P, "\tspec.TrustFile,\n", "", "TestST1e_PurgeTakesTheBindingsFile")
m("VER-40 purge leaves the bindings replacement", P, "\t\".\" + spec.TrustFile + \".tmp-\",\n", "",
  "TestST1e_PurgeTakesTheBindingsFile")

# The MAC (D173): the bindings file carries an HMAC-SHA256 under the store's
# trust key, so a file the verb did not write binds nothing; a write under the
# store root drops trust for the rest of the session, raised at its post as
# integrity/store-touched over the store root. One break per promise: the
# MAC checked, the key's length, the MAC's coverage, the key kept, the trust
# drop on each path, the write matched at the root, the trigger, the raise's
# row and path, the verb's report and its agent refusal, purge, and QA-2
# ST9's guard. Each kill row is a D173 test (TestST1k_ to TestST1t_).
H = "internal/hook/specdiscover.go"
S = "internal/hook/steerstep.go"
R = "cmd/rashomon/recap_stop.go"
I = "internal/verify/integrity.go"
E = "internal/verify/evaluate.go"
PC = "internal/verify/pathclass.go"
V = "internal/verify/integrity_raise.go"
RD = "internal/steer/render/render.go"
C = "cmd/rashomon/spec.go"
D = "internal/spec/discover.go"
CHECK = "\tif len(key) != TrustKeyBytes || !hmac.Equal([]byte(f.MAC), []byte(trustMAC(key, f.Bindings))) {\n"
m("VER-40 the MAC is not checked", T, CHECK, "\tif len(key) != TrustKeyBytes {\n", "TestST1k_|TestST1l_|TestST1p_")
m("VER-40 a key of any length checks the MAC", T, CHECK,
  "\tif !hmac.Equal([]byte(f.MAC), []byte(trustMAC(key, f.Bindings))) {\n", "TestST1k_")
m("VER-40 a key of another length signs", T, "\tif len(key) != TrustKeyBytes {\n",
  "\tif len(key) != TrustKeyBytes && false {\n", "TestST1k_")
m("VER-40 the MAC covers no binding", T, "\tm.Write(bindings)\n", "", "TestST1k_")
KEY = "\tif b, err := os.ReadFile(path); err == nil && len(b) == spec.TrustKeyBytes {\n"
m("VER-40 each trust makes a new key", H, KEY,
  "\tif b, err := os.ReadFile(path); err == nil && len(b) == spec.TrustKeyBytes && false {\n", "TestST1n_|TestST1o_")
m("VER-40 a key of another length is kept", H, KEY, "\tif b, err := os.ReadFile(path); err == nil {\n", "TestST1n_")

# The trust drop, at the steer step and at the Stop.
m("VER-40 a store write keeps trust", H, "\tif verify.StoreWritten(run, storeRoot, at) {\n",
  "\tif verify.StoreWritten(run, storeRoot, at) && false {\n", "TestST1m_")
m("VER-40 the steer step reads trust past a store write", S, "TrustRoot(run, root, p.landedSeq)", "NoRun(root)", "TestST1m_AWrite")
m("VER-40 the Stop reads trust past a store write", R, "hook.TrustRoot(run, st.Root(), seq)", "hook.NoRun(st.Root())", "TestST1m_TheStop")
m("VER-40 StoreWritten reads past its point", I,
  "\t\tif d.Seq > at {\n\t\t\tcontinue\n\t\t}\n\t\tif storeWrites(d, storeRoot, nil) {\n", "\t\tif storeWrites(d, storeRoot, nil) {\n", "TestST1s_|TestST1v_")
m("VER-40 StoreWritten reads no file tool", I, "\t\tif file != nil && underStore(slashed(*file), storeRoot) {\n",
  "\t\tif file != nil && underStore(slashed(*file), storeRoot) && false {\n", "TestST1s_")
m("VER-40 StoreWritten reads no shell write", I, "\t\tfor _, p := range shell {\n\t\t\tif underStore(slashed(p), storeRoot) {\n",
  "\t\tfor _, p := range shell[:0] {\n\t\t\tif underStore(slashed(p), storeRoot) {\n", "TestST1s_|TestST1m_AWrite")
m("VER-40 the store root's own path is not under it", PC, "\treturn in || p == r\n", "\treturn in\n", "TestST1s_|TestST1r_")
m("VER-40 a store path is matched uncleaned", PC, "\tp, r := path.Clean(p), path.Clean(slashed(root))\n",
  "\tp, r := p, path.Clean(slashed(root))\n", "TestST1s_")

# The flag: integrity matches the store root, from the point each path builds.
m("VER-40 integrity does not match the store root", I,
  "\t\t\t\t\t} else if rel := relativise(p, d.Project, d.CWD); !inStore(rel) {\n", "\t\t\t\t\t} else if rel := relativise(p, d.Project, d.CWD); false {\n", "TestST1s_")
m("VER-40 integrity names a store write twice", I, "relativise(p, d.Project, d.CWD); !inStore(rel) {",
  "relativise(p, d.Project, d.CWD); rel != \"\" {", "TestST1s_")
m("VER-40 integrity reads the root before from", I, "\t\t\tif d := &run.Declarations[i]; d.Seq > from && d.Seq <= at {\n",
  "\t\t\tif d := &run.Declarations[i]; d.Seq <= at {\n", "TestST1s_|TestST1v_")
m("VER-40 Evaluate hands integrity no store root", E, "integrity(run, picked, writes, 0, at, pt.StoreRoot)",
  "integrity(run, picked, writes, 0, at, \"\")", "TestST1m_")
m("VER-40 the steer step's point names no store root", S, "\t\tStoreRoot: root}\n", "\t}\n", "TestST1m_AWrite|TestST1r_")
m("VER-40 the Stop's point names no store root", R, ", StoreRoot: st.Root()}", "}", "TestST1m_TheStop")

# The trigger: the post's raw input scanned for the store root.
m("VER-40 the trigger does not scan for the store root", S, "\tif rootNamed(input, root) {\n\t\treturn true\n\t}\n", "",
  "TestST1t_|TestST1m_AWrite")
m("VER-40 an escaped store root is not scanned", S,
  "\treturn bytes.Contains(input, []byte(`\\/`)) && bytes.Contains(input, []byte(strings.ReplaceAll(root, \"/\", `\\/`)))\n",
  "\treturn false\n", "TestST1t_")
IMPORTS["VER-40 an escaped store root is not scanned"] = (S, "\t\"slices\"\n\t\"strings\"\n\t\"time\"\n",
  "\t\"slices\"\n\t\"time\"\n")
m("VER-40 the escaped form is built for every input", S,
  "\treturn bytes.Contains(input, []byte(`\\/`)) && bytes.Contains(input, []byte(strings.ReplaceAll(",
  "\treturn bytes.Contains(input, []byte(strings.ReplaceAll(", "TestST1t_")
m("VER-40 an empty root names every input", S, "\tif root == \"\" || !bytes.Contains(input, []byte(filepath.Base(root))) {\n",
  "\tif !bytes.Contains(input, []byte(filepath.Base(root))) {\n", "TestST1t_")

# The raise: the store root's row, with the path relative to the root.
m("VER-40 the raise does not mark the store root", V, "\t\t\tfact.StoreRoot, fact.StoreNamed = true, e.Path == \"\"\n",
  "\t\t\tfact.StoreNamed = e.Path == \"\"\n", "TestST1m_AWrite|TestST1r_")
m("VER-40 a project store write reads as the store root", V, "\t\tif e.Check == StoreTouched && !inStore(e.Path) {\n",
  "\t\tif e.Check == StoreTouched {\n", "TestIV3b_")
m("VER-40 the raise shows the store path whole", V, "\t\t\t\tshown = rel\n", "\t\t\t\t_ = rel\n", "TestST1m_AWrite|TestST1r_")
m("VER-40 no store-root row", RD,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityStoreRootTouchedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ R int }{})}: {\n", "TestST1r_")
m("VER-40 no store-root unnamed row", RD,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityStoreRootTouchedUnnamedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ U int }{})}: {\n", "TestST1r_")

# The verb: its report of an unsigned file, and its refusal under the agent
# signal, which the key's write now sits behind.
m("VER-40 the verb does not report an unsigned file", C,
  "\t\tif _, err := fmt.Fprintf(stdout, \"%s does not carry this store's MAC, so it trusts nothing: something other than rashomon spec trust wrote it\\n\", spec.TrustFile); err != nil {\n\t\t\treturn err\n\t\t}\n",
  "", "TestST1p_")
m("VER-40 trust runs in an agent session", C, "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" && false {\n", "TestST1o_")

# QA-2 ST9: only the repository spec discovery read is asked about.
m("VER-40 a binding trusts the managed spec", D, "s.Path == near && bound(near, s.Digest)", "bound(near, s.Digest)", "TestST1q_")

# Purge takes the key and its replacement.
m("VER-40 purge leaves the trust key", P, "\tspec.TrustKeyFile,\n", "", "TestST1e_PurgeTakesTheBindingsFile")
m("VER-40 purge leaves the trust key's replacement", P, "\t\".\" + spec.TrustKeyFile + \".tmp-\",\n", "",
  "TestST1e_PurgeTakesTheBindingsFile")

# D173b (QA-2's blocking item): at record scope v4 the gate withholds the
# paths, so the hook keeps a store-root bit on the declaration and the drop and
# the raise read it there; where the paths are recorded it is not read. And
# QA-2's item 4: a file the verb did not write reads unsigned, so trust
# replaces it. One break per promise.
HD = "internal/hook/handle.go"
m("VER-40 the store-root bit is not set", HD, "\tdecl.StoreNamed = rootNamed(p.ToolInput, h.st.Root())\n",
  "\tdecl.StoreNamed = false\n", "TestST1u_")
m("VER-40 StoreWritten ignores the bit", I, "\tif namedWithheld(d) {\n\t\tif f != nil {\n", "\tif false {\n\t\tif f != nil {\n",
  "TestST1v_|TestST1u_|TestST1x_")
m("VER-40 integrity raises nothing for the bit", I,
  "\t\t\t\t\t\tout.Unresolved = append(out.Unresolved, IntegrityEntry{Check: StoreTouched, Seq: d.Seq})\n", "", "TestST1v_|TestST1u_")
m("VER-40 the bit is read where the paths are recorded", I,
  "\treturn d.StoreNamed && (d.CommandWithheld != nil || d.FilePathWithheld != nil || d.WrittenPathsWithheld != nil)\n",
  "\treturn d.StoreNamed\n", "TestST1v_")
m("VER-40 a file that is not JSON stops the verb", T,
  "\tif err := decodeOne(data, &f); err != nil {\n\t\treturn nil, fmt.Errorf(\"%w: %s: %v\", ErrBindingsUnsigned, path, err)\n",
  "\tif err := decodeOne(data, &f); err != nil {\n\t\treturn nil, fmt.Errorf(\"spec: %s: %v\", path, err)\n", "TestST1w_")
m("VER-40 a file over its cap stops the verb", T,
  "\t\treturn nil, fmt.Errorf(\"%w: %s is over its %d-byte cap\", ErrBindingsUnsigned, path, MaxTrustBytes)\n",
  "\t\treturn nil, fmt.Errorf(\"spec: %s is over its %d-byte cap\", path, MaxTrustBytes)\n", "TestST1w_")
m("VER-40 an unreadable key stops the verb", T,
  "\t\treturn nil, fmt.Errorf(\"%w: %s: the trust key: %v\", ErrBindingsUnsigned, path, err)\n",
  "\t\treturn nil, err\n", "TestST1w_")
m("VER-40 the bit's fact is not named", V, "\t\t\tfact.StoreRoot, fact.StoreNamed = true, e.Path == \"\"\n",
  "\t\t\tfact.StoreRoot, fact.StoreNamed = true, false\n", "TestST1u_")
m("VER-40 no store-root named row", RD,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityStoreRootNamedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ N int }{})}: {\n", "TestST1u_")
# SPEC-TRUST2: DiscoverSpec takes a TrustedRoot, so a raw root no longer
# builds; each of the three breaks above reads the root as hook.NoRun does,
# the root as it is, which is the same missing store-write check.
# D203: the prompt's preflight note reads trust as the Stop does.
m("VER-40 the preflight reads trust past a store write", "cmd/rashomon/preflight.go",
  "hook.TrustRoot(run, st.Root(), run.LastSeq())", "hook.NoRun(st.Root())", "TestST1m_TheStop")
