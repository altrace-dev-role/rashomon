# RES-2: process init allocates at most 64 KiB and no package compiles a
# regexp at package scope. A package-scope compile, or an init trace line
# the parser skips instead of refusing, is the whole regression.
IMPORTS["RES-2 a regexp compiles at package scope"] = ("internal/shape/shape.go",
  'import (\n', 'import (\n\t"regexp"\n')
m("RES-2 a regexp compiles at package scope", "internal/shape/shape.go",
  "// Derive ", "var shapeWordRE = regexp.MustCompile(`^[a-z]+$`)\n\n// Derive ", "TestRES2_")
m("RES-2 the init trace parser skips a line it cannot read", "test/perf/gate.go",
  '\t\t\tif len(f) < 11 || f[8] != "bytes," || f[10] != "allocs" {\n\t\t\t\treturn t, fmt.Errorf("unrecognised init line: %q", ln)\n\t\t\t}\n',
  '\t\t\tif len(f) < 11 || f[8] != "bytes," || f[10] != "allocs" {\n\t\t\t\tcontinue\n\t\t\t}\n', "TestRES2b_")
