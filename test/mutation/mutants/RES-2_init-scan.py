# RES-2: the static check finds a compile however initialisation reaches it.
# Each resolution step removed alone hides a shape the spec names (its own
# mutant is `var rules = compileRules()`), and counting sync.OnceValue's
# deferred function flags the fix as the fault.
m("RES-2 an aliased regexp import hides its compiles", "test/perf/initscan.go",
  '\t\t\tcase is.Name.Name != "_":\n\t\t\t\tfs.imports[is.Name.Name] = ip\n', '\t\t\tcase is.Name.Name != "_":\n\t\t\t\tfs.imports[name] = ip\n', "TestRES2c_")
m("RES-2 a dot import of regexp hides its compiles", "test/perf/initscan.go",
  "\t\t\t\tfs.dots = append(fs.dots, ip)\n", "", "TestRES2c_")
m("RES-2 the scan does not follow a helper", "test/perf/initscan.go",
  '\t\ts.walk(fn.decl.Body, fn.file, fn.pkg, via+" -> "+fn.decl.Name.Name)\n', "", "TestRES2c_")
m("RES-2 the scan does not follow a method", "test/perf/initscan.go",
  "\t\t\tfns = append(fns, p.methods[f.Sel.Name]...)\n", "\t\t\tfns = append(fns, p.methods[\"\"]...)\n", "TestRES2c_")
m("RES-2 the scan does not follow another package", "test/perf/initscan.go",
  "\t\t\t\t\treturn p.funcs[f.Sel.Name], p.vars[f.Sel.Name]\n", "\t\t\t\t\treturn nil, p.vars[f.Sel.Name]\n", "TestRES2c_")
m("RES-2 a function sync.OnceValue defers counts as init", "test/perf/initscan.go",
  '\t\tif fs.calls(call.Fun, "sync", syncDefers) {\n', '\t\tif false && fs.calls(call.Fun, "sync", syncDefers) {\n', "TestRES2c_")
# P08-4 round two: skipping a sync.Once* argument must not skip it when
# initialisation then calls the Once result, through a package var holding it
# (here or in another package) or inline; nor may a var holding a plain
# function hide the call. Each follow removed alone misses a compile that runs
# at init.
m("RES-2 a Once getter called at init is not followed", "test/perf/initscan.go",
  '\t\t\ts.invoke(v.value, v.file, v.pkg, via+" -> "+v.name.Name)\n', "", "TestRES2c_")
m("RES-2 a Once result called inline is not followed", "test/perf/initscan.go",
  '\t\tif fs.calls(f.Fun, "sync", syncDefers) {\n', '\t\tif false && fs.calls(f.Fun, "sync", syncDefers) {\n', "TestRES2c_")
m("RES-2 another package's getter is not followed", "test/perf/initscan.go",
  "\t\t\t\t\treturn p.funcs[f.Sel.Name], p.vars[f.Sel.Name]\n", "\t\t\t\t\treturn p.funcs[f.Sel.Name], nil\n", "TestRES2c_")
m("RES-2 a package var's value is not recorded", "test/perf/initscan.go",
  "\t\t\tif len(vs.Values) != len(vs.Names) {\n", "\t\t\tif true {\n", "TestRES2c_")
m("RES-2 a compile reached twice counts twice", "test/perf/initscan.go",
  "\t\t\tif !s.at[call.Pos()] {\n", "\t\t\tif true {\n", "TestRES2c_")
m("RES-2 MatchString is not a compile", "test/perf/initscan.go",
  '\t"Match": true, "MatchString": true, "MatchReader": true,\n', "", "TestRES2c_")
# The spec's own shape, and an aliased import, in a package the binary links.
IMPORTS["RES-2 var rules = compileRules() in a linked package"] = ("internal/shape/tokenize.go",
  'import (\n', 'import (\n\t"regexp"\n')
m("RES-2 var rules = compileRules() in a linked package", "internal/shape/tokenize.go",
  "// errUnterminated and errUncertain ",
  "var rules = compileRules()\n\nfunc compileRules() *regexp.Regexp { return regexp.MustCompile(`^[a-z]+$`) }\n\n// errUnterminated and errUncertain ", "TestRES2_")
IMPORTS["RES-2 an aliased regexp import compiles at init"] = ("internal/shape/tokenize.go",
  'import (\n', 'import (\n\tre "regexp"\n')
m("RES-2 an aliased regexp import compiles at init", "internal/shape/tokenize.go",
  "// errUnterminated and errUncertain ",
  "var wordRE = re.MustCompile(`^[a-z]+$`)\n\n// errUnterminated and errUncertain ", "TestRES2_")
# P08-4 round two: the reviewer's probe on the real tree. A OnceValue getter
# that a var initialiser calls compiles at init; the base check caught it and
# the first Once skip did not.
IMPORTS["RES-2 a OnceValue getter called at init in a linked package"] = ("internal/shape/tokenize.go",
  'import (\n', 'import (\n\t"regexp"\n\t"sync"\n')
m("RES-2 a OnceValue getter called at init in a linked package", "internal/shape/tokenize.go",
  "// errUnterminated and errUncertain ",
  "var wordRE = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[a-z]+$`) })\n\nvar eagerWord = wordRE()\n\n// errUnterminated and errUncertain ", "TestRES2_")
