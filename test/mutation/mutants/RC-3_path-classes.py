# RC-3 (PF-RC3): a written path's closed class and whether it stales a gate.
# One break per table, per guard, per adjacent precedence pair (config's case
# against the source fallback the last of them), per normalisation step, per
# Relevant class and per vocabulary word.
F = "internal/verify/pathclass.go"

# The tables.
m("RC-3 generated segments match nothing", F,
  '[]string{"vendor", "node_modules", "__pycache__", ".next", "__generated__"}', '[]string{}', "TestRC3c_")
m("RC-3 generated root segments match nothing", F,
  '[]string{"dist", "target"}', '[]string{}', "TestRC3c_")
m("RC-3 generated suffixes match nothing", F,
  '[]string{".pb.go", ".gen.go", ".gen.ts", ".gen.js", "_generated.go", ".min.js", ".min.css", ".generated.ts", ".g.dart", ".freezed.dart", ".pb.dart", ".pbenum.dart", ".pbjson.dart", ".pbserver.dart", ".pbgrpc.dart"}', '[]string{}', "TestRC3c_")
m("RC-3 CI segments match nothing", F,
  '[]string{".github", ".gitlab", ".circleci", ".buildkite"}', '[]string{}', "TestRC3c_")
m("RC-3 CI basenames match nothing", F,
  '[]string{".gitlab-ci.yml", ".travis.yml", "azure-pipelines.yml", "jenkinsfile"}', '[]string{}', "TestRC3c_")
m("RC-3 test segments match nothing", F,
  '[]string{"test", "tests", "spec", "__tests__", "testdata", "e2e"}', '[]string{}', "TestRC3c_")
m("RC-3 test suffixes match nothing", F,
  '\t\t"_test.go", "_test.py", "_spec.rb", "_test.rb", "_test.exs",\n'
  '\t\t".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".test.ts", ".test.tsx", ".test.js", ".test.jsx",\n',
  '', "TestRC3c_")
m("RC-3 test_*.py stops being a test name", F,
  'n.base == "conftest.py" || (strings.HasPrefix(n.base, "test_") && strings.HasSuffix(n.base, ".py"))',
  'n.base == "conftest.py"', "TestRC3c_")
m("RC-3 conftest.py stops being a test name", F,
  'n.base == "conftest.py" || ', 'false || ', "TestRC3c_")
m("RC-3 the CamelCase test names match nothing", F,
  'inSet(n.ext, camelTestExtensions) {', 'false {', "TestRC3c_")
m("RC-3 the CamelCase test names fold case and match Latest.java", F,
  'strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests")',
  'strings.HasSuffix(strings.ToLower(stem), "test") || strings.HasSuffix(strings.ToLower(stem), "tests")', "TestRC3c_")
m("RC-3 *Tests.java stops being a test name", F,
  ' || strings.HasSuffix(stem, "Tests")', '', "TestRC3c_")
m("RC-3 the CamelCase extension compares with case", F,
  'inSet(n.ext, camelTestExtensions)', 'inSet(ext, camelTestExtensions)', "TestRC3c_")
m("RC-3 docs root segments match nothing", F,
  '[]string{"docs", "doc"}', '[]string{}', "TestRC3c_")
m("RC-3 docs extensions match nothing", F,
  '[]string{".md", ".rst", ".adoc"}', '[]string{}', "TestRC3c_")
m("RC-3 docs basenames match nothing", F,
  '[]string{"license", "changelog", "authors", "notice"}', '[]string{}', "TestRC3c_")
m("RC-3 config extensions match nothing", F,
  '[]string{".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".lock", ".sum", ".properties", ".xml", ".pbxproj"}', '[]string{}', "TestRC3c_")
m("RC-3 config basenames match nothing", F,
  '[]string{"makefile", "dockerfile", "go.mod", "go.work", "cmakelists.txt",\n'
  '\t\t"build", "build.bazel", "workspace", "workspace.bazel", "module.bazel", "package.swift"}', '[]string{}', "TestRC3c_")
m("RC-3 requirements*.txt stops being config", F,
  'strings.HasPrefix(base, "requirements") || ', '', "TestRC3c_")
m("RC-3 constraints*.txt stops being config", F,
  ' || strings.HasPrefix(base, "constraints")', '', "TestRC3c_")
m("RC-3 a dotfile stops being config", F,
  ' || isDotfile(n.base)', '', "TestRC3c_")

# The root-only words (a directory word that classifies only as the first
# path segment), build and .txt.
m("RC-3 dist and target match below the root again", F,
  'inSet(n.first, generatedRootSegments)', 'hasSegment(n.dir, generatedRootSegments)', "TestRC3[ce]_")
m("RC-3 docs and doc match below the root again", F,
  'inSet(n.first, docsRootSegments)', 'hasSegment(n.dir, docsRootSegments)', "TestRC3[ce]_")
m("RC-3 the first element is the whole directory", F,
  'first, _, _ := strings.Cut(dir, "/")', 'first := dir', "TestRC3c_")
m("RC-3 .txt is a docs extension again", F,
  '[]string{".md", ".rst", ".adoc"}', '[]string{".md", ".rst", ".adoc", ".txt"}', "TestRC3c_")
m("RC-3 build is a generated segment again", F,
  '[]string{"vendor", "node_modules", "__pycache__", ".next", "__generated__"}', '[]string{"vendor", "node_modules", "__pycache__", ".next", "__generated__", "build"}', "TestRC3[ce]_")

# The cross-ecosystem rows: one break per added table entry group and guard.
m("RC-3 generated prefixes match nothing", F,
  '[]string{"zz_generated."}', '[]string{}', "TestRC3f_")
m("RC-3 the generated prefix loses its dot", F,
  '[]string{"zz_generated."}', '[]string{"zz_generated"}', "TestRC3f_")
m("RC-3 .gen.ts stops being generated", F, '".gen.ts", ', '', "TestRC3f_")
m("RC-3 .gen.js stops being generated", F, '".gen.js", ', '', "TestRC3f_")
m("RC-3 .generated.ts stops being generated", F, '".generated.ts", ', '', "TestRC3f_")
m("RC-3 .g.dart stops being generated", F, '".g.dart", ', '', "TestRC3f_")
m("RC-3 the .g.dart suffix loses its dot", F, '".g.dart", ', '"g.dart", ', "TestRC3f_")
m("RC-3 .freezed.dart stops being generated", F, '".freezed.dart", ', '', "TestRC3f_")
m("RC-3 .pb.dart stops being generated", F, '".pb.dart", ', '', "TestRC3f_")
m("RC-3 .pbenum.dart stops being generated", F, '".pbenum.dart", ', '', "TestRC3f_")
m("RC-3 .pbjson.dart stops being generated", F, '".pbjson.dart", ', '', "TestRC3f_")
m("RC-3 .pbserver.dart stops being generated", F, '".pbserver.dart", ', '', "TestRC3f_")
m("RC-3 .pbgrpc.dart stops being generated", F, ', ".pbgrpc.dart"}', '}', "TestRC3f_")
m("RC-3 the .freezed.dart suffix loses its dot", F, '".freezed.dart", ', '"freezed.dart", ', "TestRC3f_")
m("RC-3 the .pb.dart suffix loses its dot", F, '".pb.dart", ', '"pb.dart", ', "TestRC3f_")
m("RC-3 __generated__ stops being a generated segment", F, ', "__generated__"}', '}', "TestRC3f_")
m("RC-3 the CamelCase extensions are .java alone", F,
  '[]string{".java", ".kt", ".cs", ".swift", ".scala"}', '[]string{".java"}', "TestRC3f_")
m("RC-3 the CamelCase stem is cut by the folded extension's length", F,
  'if ext := path.Ext(n.raw);', 'if ext := n.ext;', "TestRC3f_")
m("RC-3 a basename of dots alone is a dotfile again", F,
  ' && strings.TrimLeft(base, ".") != ""', '', "TestRC3f_")
m("RC-3 the Bazel basenames match nothing", F,
  '"build", "build.bazel", "workspace", "workspace.bazel", "module.bazel", ', '', "TestRC3f_")
m("RC-3 Package.swift stops being config", F, ', "package.swift"}', '}', "TestRC3f_")
m("RC-3 go.work stops being config", F, '"go.work", ', '', "TestRC3f_")
m("RC-3 .pbxproj stops being config", F, ', ".pbxproj"}', '}', "TestRC3f_")

# Precedence: each adjacent pair of rules swapped.
GEN = ("\tcase hasSegment(n.dir, generatedSegments) || inSet(n.first, generatedRootSegments) || hasAnySuffix(n.base, generatedSuffixes) || hasAnyPrefix(n.base, generatedPrefixes):\n"
       "\t\treturn ClassGenerated\n")
CI = ("\tcase hasSegment(n.dir, ciSegments) || inSet(n.base, ciBasenames):\n"
      "\t\treturn ClassCI\n")
TEST = ("\tcase hasSegment(n.dir, testSegments) || isTestName(n):\n"
        "\t\treturn ClassTest\n")
DOCS = ("\tcase inSet(n.first, docsRootSegments) || inSet(n.ext, docsExtensions) || inSet(n.base, docsBasenames):\n"
        "\t\treturn ClassDocs\n")
CONFIG = ("\tcase inSet(n.ext, configExtensions) || isConfigName(n.base) || isDotfile(n.base):\n"
          "\t\treturn ClassConfig\n")
FALLBACK = "\t}\n\treturn ClassSource\n}\n"
m("RC-3 CI ranks before generated", F, GEN + CI, CI + GEN, "TestRC3c_")
m("RC-3 test ranks before CI", F, CI + TEST, TEST + CI, "TestRC3c_")
m("RC-3 docs ranks before test", F, TEST + DOCS, DOCS + TEST, "TestRC3c_")
m("RC-3 config ranks before docs", F, DOCS + CONFIG, CONFIG + DOCS, "TestRC3c_")
# The source fallback taken before config's case is tried: an always-true
# case ahead of it, since the fallback follows the switch and cannot swap.
m("RC-3 source ranks before config", F, CONFIG + FALLBACK,
  "\tcase true:\n\t\treturn ClassSource\n" + CONFIG + FALLBACK, "TestRC3c_")

# Normalisation and the fallback.
m("RC-3 the path is not lower-cased", F,
  'lower := strings.ToLower(p)', 'lower := p', "TestRC3c_")
m("RC-3 backslashes stay backslashes", F,
  '\tp = strings.ReplaceAll(p, `\\`, "/")\n', '', "TestRC3c_")
m("RC-3 a trailing slash is trimmed and the directory read as a basename", F,
  '\tp = strings.ReplaceAll(p, `\\`, "/")\n',
  '\tp = strings.ReplaceAll(p, `\\`, "/")\n\tp = strings.TrimRight(p, "/")\n', "TestRC3c_")
m("RC-3 the basename counts as a directory segment", F,
  'dir, base := lower[:j+1], lower[j+1:]', 'dir, base := lower, lower[j+1:]', "TestRC3c_")
m("RC-3 the CamelCase names read a lower-cased basename", F,
  'raw := p[i+1:]', 'raw := strings.ToLower(p[i+1:])', "TestRC3c_")
m("RC-3 a path no rule matches falls back to docs", F,
  '\t}\n\treturn ClassSource\n}\n', '\t}\n\treturn ClassDocs\n}\n', "TestRC3c_")

# Relevant (source, test and config stale a gate), the closed list and its
# words.
m("RC-3 a CI edit stales a gate", F, 'c != ClassCI && ', '', "TestRC3d_")
m("RC-3 a docs edit stales a gate", F, 'c != ClassDocs && ', '', "TestRC3d_")
m("RC-3 a generated edit stales a gate", F, ' && c != ClassGenerated', '', "TestRC3d_")
m("RC-3 a source edit stops staling a gate", F,
  'c != ClassGenerated\n', 'c != ClassGenerated && c != ClassSource\n', "TestRC3d_")
m("RC-3 a test edit stops staling a gate", F,
  'c != ClassGenerated\n', 'c != ClassGenerated && c != ClassTest\n', "TestRC3d_")
m("RC-3 a config edit stops staling a gate", F,
  'c != ClassGenerated\n', 'c != ClassGenerated && c != ClassConfig\n', "TestRC3d_")
m("RC-3 a class this binary does not know stops staling a gate", F,
  'return c != ClassCI && c != ClassDocs && c != ClassGenerated',
  'return c == ClassSource || c == ClassTest || c == ClassConfig', "TestRC3d_")
m("RC-3 the closed list loses a class", F, 'ClassDocs, ClassGenerated}', 'ClassDocs}', "TestRC3d_")
m("RC-3 the word source changes", F, 'PathClass = "source"', 'PathClass = "src"', "TestRC3d_")
m("RC-3 the word test changes", F, 'PathClass = "test"', 'PathClass = "tests"', "TestRC3d_")
m("RC-3 the word config changes", F, 'PathClass = "config"', 'PathClass = "cfg"', "TestRC3d_")
m("RC-3 the word ci changes", F, 'PathClass = "ci"', 'PathClass = "CI"', "TestRC3d_")
m("RC-3 the word docs changes", F, 'PathClass = "docs"', 'PathClass = "doc"', "TestRC3d_")
m("RC-3 the word generated changes", F, 'PathClass = "generated"', 'PathClass = "gen"', "TestRC3d_")
