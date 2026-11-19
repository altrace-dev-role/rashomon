# LANG-REG1 (PR 325), the registry and its wiring: Wire builds the Go step
# from the root's go.mod (its module path, directories under the root, a
# go.mod over the bound read for its complete lines) and the Python step
# from the configuration by python(); a refused step names its file and one
# of four reasons; the kinds table answers NonSource and keys a path by its
# extension, else its base name; a Claim runs the claiming reader's step
# alone, under the step's lock; Unindexed answers for an indexed reader the
# producers lack. One row per Break clause of each test, killed by that test
# alone. Written, not run: for the end sweep.
wire = "internal/graph/index/index.go"
gomod = "internal/graph/lang/gopg/imports.go"
gopgsrc = "internal/graph/lang/gopg/gopg.go"
kinds = "internal/graph/lang/kinds.go"
registry = "internal/graph/lang/lang.go"
refusalsrc = "internal/graph/lang/refusal.go"

gomap = "&gopg.ImportMap{Module: mod, IsDir: gopg.DirUnder(root)}"
goerr = "\t\tcase err != nil:\n\t\t\treturn nil, refusal(gopg.ManifestFile, err)\n"
gonoerr = "\t\tcase err != nil:\n\t\t\treturn gopg.Resolver{}, nil\n"
goimports = "\t\treturn gopg.Resolver{Imports: &gopg.ImportMap{Module: mod, IsDir: gopg.DirUnder(root)}}, nil\n"
basename = "\tbase := path.Base(rel)\n"
named = "\tfor _, n := range namedPrograms {\n\t\tif base == n {\n\t\t\treturn n\n\t\t}\n\t}\n"
extblock = "\tif ext := path.Ext(rel); ext != \"\" {\n\t\treturn ext\n\t}\n"
oneref = "\treturn &lang.Refusal{File: file, Reason: r, Err: err}\n"
overlimit = "\t\terr = fmt.Errorf(\"%s: %w\", p, errConfigOverCap)\n"
copyfunc = "func NonSourceKinds() [len(nonSourceKinds)]string { return nonSourceKinds }\n"
indexedflag = "\tif !ok || !e.indexed {\n"

# TestWire_GoMapsUnderItsOwnModuleAndRoot. Break: map under a module path
# not read from the root's go.mod, or check directories under another root.
m("LANG-REG1 the Go step maps imports under a module path go.mod does not hold", wire,
  gomap,
  "&gopg.ImportMap{Module: \"example.invalid/m\", IsDir: gopg.DirUnder(root)}",
  "^TestWire_GoMapsUnderItsOwnModuleAndRoot$")
m("LANG-REG1 the Go step checks directories under the root's parent", wire,
  gomap,
  "&gopg.ImportMap{Module: mod, IsDir: gopg.DirUnder(filepath.Dir(root))}",
  "^TestWire_GoMapsUnderItsOwnModuleAndRoot$")

# TestWire_AGoModOverTheBoundReadsItsCompleteLines. Break: refuse a go.mod
# over the bound.
m("LANG-REG1 a go.mod over the bound is refused", gomod,
  "\tif n == MaxGoModBytes {\n\t\ti := bytes.LastIndexByte(b, '\\n')\n\t\tif i < 0 {\n\t\t\treturn \"\", nil\n\t\t}\n\t\tb = b[:i+1]\n\t}\n",
  "\tif n == MaxGoModBytes {\n\t\treturn \"\", errors.New(\"go.mod over the bound\")\n\t}\n",
  "^TestWire_AGoModOverTheBoundReadsItsCompleteLines$")

# TestWire_AGoModDirectoryRefusesGo. Break: let the Go step drop the error.
m("LANG-REG1 the Go step drops its error, a go.mod directory", wire,
  goerr, gonoerr,
  "^TestWire_AGoModDirectoryRefusesGo$")

# TestWire_AnUnreadableGoModRefusesGo. Break: let the Go step drop the error.
m("LANG-REG1 the Go step drops its error, an unreadable go.mod", wire,
  goerr, gonoerr,
  "^TestWire_AnUnreadableGoModRefusesGo$")

# TestWire_PythonIsTheResolverBuildWires. Break: build Wire's Python
# resolver by a rule other than python. The test's configuration names the
# module inv, so an empty module map is not Build's.
m("LANG-REG1 Wire builds the Python resolver by a rule other than python", wire,
  "\t\treturn python(c), nil\n\t})\n",
  "\t\treturn pylite.Resolver{Units: python(c).Units, Modules: map[string]string{}}, nil\n\t})\n",
  "^TestWire_PythonIsTheResolverBuildWires$")

# TestKinds_AProgramKindIsNotNonSource. Break: invert the answer, or key
# CMakeLists.txt by .txt.
m("LANG-REG1 NonSource inverts its answer", kinds,
  "\t\tif row == k {\n\t\t\treturn true\n\t\t}\n\t}\n\treturn false\n",
  "\t\tif row == k {\n\t\t\treturn false\n\t\t}\n\t}\n\treturn true\n",
  "^TestKinds_AProgramKindIsNotNonSource$")
m("LANG-REG1 KindKey keys CMakeLists.txt by its extension", kinds,
  basename + named, basename,
  "^TestKinds_AProgramKindIsNotNonSource$")

# TestKinds_KeyIsTheExtensionElseTheBaseName. Break: key every path by its
# base name, or check the name after the extension.
m("LANG-REG1 KindKey keys every path by its base name", kinds,
  extblock, "",
  "^TestKinds_KeyIsTheExtensionElseTheBaseName$")
m("LANG-REG1 KindKey checks the program names after the extension", kinds,
  basename + named + extblock, basename + extblock + named,
  "^TestKinds_KeyIsTheExtensionElseTheBaseName$")

# TestRegistry_ConcurrentClaimsUnderOneRootRunTheStepOnce. Break: take the
# lock out of the step's at method.
m("LANG-REG1 a step's at method runs without its lock", registry,
  "\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n", "",
  "^TestRegistry_ConcurrentClaimsUnderOneRootRunTheStepOnce$")

# TestLANGREG1_AFIFOConfigSaysWhy. Break: open the go.mod by os.Open, and
# the child is killed at the bound.
fifo = "LANG-REG1 go.mod is opened by os.Open"
IMPORTS[fifo] = (gomod, "\t\"github.com/altrace-dev-role/rashomon/internal/regfile\"\n", "")
m(fifo, gomod,
  "\tf, err := regfile.Open(filepath.Join(root, ManifestFile))\n",
  "\tf, err := os.Open(filepath.Join(root, ManifestFile))\n",
  "^TestLANGREG1_AFIFOConfigSaysWhy$")

# TestLANGREG1_AnUnreadableConfigSaysWhy. Break: one reason for every
# refusal, and the two lines match.
m("LANG-REG1 every refusal has one reason, two unread files", wire,
  oneref,
  "\t_ = r\n\treturn &lang.Refusal{File: file, Reason: lang.ReadFailed, Err: err}\n",
  "^TestLANGREG1_AnUnreadableConfigSaysWhy$")

# TestWire_ARefusalNamesItsFileAndReason. Break: one reason for every
# refusal, or name the Python file for the Go step's refusal.
m("LANG-REG1 every refusal has one reason, its file and reason", wire,
  oneref,
  "\t_ = r\n\treturn &lang.Refusal{File: file, Reason: lang.ReadFailed, Err: err}\n",
  "^TestWire_ARefusalNamesItsFileAndReason$")
m("LANG-REG1 the Go step's refusal names the Python configuration", wire,
  "\t\t\treturn nil, refusal(gopg.ManifestFile, err)\n",
  "\t\t\treturn nil, refusal(ConfigFile, err)\n",
  "^TestWire_ARefusalNamesItsFileAndReason$")

# TestWire_AConfigPastTheLimitNamesTheFileAndTheLimit. Break: drop the path
# or the bytes from the words, or build the error without the sentinel.
m("LANG-REG1 a configuration past the limit drops its path from the words", wire,
  overlimit,
  "\t\terr = fmt.Errorf(\"%w\", errConfigOverCap)\n",
  "^TestWire_AConfigPastTheLimitNamesTheFileAndTheLimit$")
m("LANG-REG1 a configuration past the limit drops the bytes from the words", wire,
  "var errConfigOverCap = errors.New(\"over 65536 bytes\")\n",
  "var errConfigOverCap = errors.New(\"over the limit\")\n",
  "^TestWire_AConfigPastTheLimitNamesTheFileAndTheLimit$")
m("LANG-REG1 a configuration past the limit is built without the sentinel", wire,
  overlimit,
  "\t\terr = fmt.Errorf(\"%s: %v\", p, errConfigOverCap)\n",
  "^TestWire_AConfigPastTheLimitNamesTheFileAndTheLimit$")

# TestWire_AnUnreadableFileIsAFailedRead. Break: read every refusal as a
# file that is not regular.
m("LANG-REG1 every refusal reads as a file that is not regular", wire,
  oneref,
  "\t_ = r\n\treturn &lang.Refusal{File: file, Reason: lang.NotRegular, Err: err}\n",
  "^TestWire_AnUnreadableFileIsAFailedRead$")

# TestRefusal_IsTheStepsErrorWithItsFileAndReason. Break: put the file in
# the text, wrap nothing, or start the reasons at another value.
m("LANG-REG1 a refusal's text holds its file", refusalsrc,
  "func (r *Refusal) Error() string { return r.Err.Error() }\n",
  "func (r *Refusal) Error() string { return r.File + \": \" + r.Err.Error() }\n",
  "^TestRefusal_IsTheStepsErrorWithItsFileAndReason$")
m("LANG-REG1 a refusal wraps nothing", refusalsrc,
  "func (r *Refusal) Unwrap() error { return r.Err }\n",
  "func (r *Refusal) Unwrap() error { return nil }\n",
  "^TestRefusal_IsTheStepsErrorWithItsFileAndReason$")
m("LANG-REG1 the reasons start past zero", refusalsrc,
  "\tReadFailed Reason = iota\n",
  "\tReadFailed Reason = iota + 1\n",
  "^TestRefusal_IsTheStepsErrorWithItsFileAndReason$")

# TestRegistry_UnindexedIsAnIndexedClaimantTheProducersLack. Break: ignore
# the flag, and the .go path reads as unindexed under producers that lack go.
m("LANG-REG1 Unindexed ignores the indexed flag", registry,
  indexedflag, "\tif !ok {\n",
  "^TestRegistry_UnindexedIsAnIndexedClaimantTheProducersLack$")

# TestLANGREG1_AGoEditIsByteEqual. Breaks: build the Go overlay with no
# import map, or say the unindexed-language line for every language the
# producers lack.
m("LANG-REG1 the Go step gives no import map, a Go edit's group", wire,
  goimports, "\t\treturn gopg.Resolver{}, nil\n",
  "^TestLANGREG1_AGoEditIsByteEqual$")
m("LANG-REG1 Unindexed ignores the indexed flag, a Go edit", registry,
  indexedflag, "\tif !ok {\n",
  "^TestLANGREG1_AGoEditIsByteEqual$")

# TestWire_GoCarriesImportsWhereThereIsAModule. Break: give the resolver no
# import map where there is a module.
m("LANG-REG1 the Go step gives no import map where there is a module", wire,
  goimports, "\t\treturn gopg.Resolver{}, nil\n",
  "^TestWire_GoCarriesImportsWhereThereIsAModule$")

# TestLANGREG1_AnUnindexedLanguageSaysSoOnce, one Break of four (the hook
# rows file holds the other three): say that a missing configuration was
# not read.
m("LANG-REG1 a missing Python configuration reads as not read", wire,
  "\t\tcase errors.Is(err, fs.ErrNotExist):\n\t\t\treturn python(Config{}), nil\n", "",
  "^TestLANGREG1_AnUnindexedLanguageSaysSoOnce$")

# TestWire_AGoModLinkLoopIsAFailedRead. Break: read a link loop as a file
# that is not regular.
linkloop = "LANG-REG1 a go.mod link loop reads as a file that is not regular"
IMPORTS[linkloop] = (wire, "\t\"strings\"\n\n", "\t\"strings\"\n\t\"syscall\"\n\n")
m(linkloop, wire,
  "\tcase errors.Is(err, regfile.ErrNotRegular):\n",
  "\tcase errors.Is(err, regfile.ErrNotRegular) || errors.Is(err, syscall.ELOOP):\n",
  "^TestWire_AGoModLinkLoopIsAFailedRead$")

# TestKinds_TheCopyIsTheTable. Break: return an empty or a partial copy.
m("LANG-REG1 NonSourceKinds returns an empty copy", kinds,
  copyfunc,
  "func NonSourceKinds() [len(nonSourceKinds)]string { return [len(nonSourceKinds)]string{} }\n",
  "^TestKinds_TheCopyIsTheTable$")
m("LANG-REG1 NonSourceKinds returns a partial copy", kinds,
  copyfunc,
  "func NonSourceKinds() [len(nonSourceKinds)]string { c := nonSourceKinds; c[len(c)-1] = \"\"; return c }\n",
  "^TestKinds_TheCopyIsTheTable$")

# TestLANGREG1_EachRowOfTheKindsTableIsSilent, one Break of two (the hook
# rows file holds the other, and the seam's row): let a wired reader claim
# a row's kind.
m("LANG-REG1 the Go reader claims a kinds-table row", gopgsrc,
  "func (Resolver) Claims(_, rel string) bool { return strings.HasSuffix(rel, \".go\") }\n",
  "func (Resolver) Claims(_, rel string) bool { return strings.HasSuffix(rel, \".go\") || strings.HasSuffix(rel, \".md\") }\n",
  "^TestLANGREG1_EachRowOfTheKindsTableIsSilent$")

# TestUnreadConfig_AGoEditIsByteEqualWhilePythonsConfigIsUnread, one Break
# of two (the hook rows file holds the other): run every reader's step on a
# Claim, and the Go edit is refused for the Python file and keeps no landed
# group.
m("LANG-REG1 a Claim runs every reader's step", registry,
  "\tc := e.st.at(root)\n",
  "\tfor _, o := range g.es {\n\t\tif o.st != nil {\n\t\t\tif oc := o.st.at(root); oc.err != nil {\n\t\t\t\treturn nil, true, oc.err\n\t\t\t}\n\t\t}\n\t}\n\tc := e.st.at(root)\n",
  "^TestUnreadConfig_AGoEditIsByteEqualWhilePythonsConfigIsUnread$")
