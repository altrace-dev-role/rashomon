# RES-29 (rubric lld-5, fix list 16): maxNest is the only barrier between
# nested substitutions and a stack overflow, which exits 2 and blocks the
# call. Lifting it, moving it by one either way, or dropping the check at any
# of the three places that recurse is the regression.
m("RES-29 maxNest is lifted", "internal/shape/comsub.go",
  "const maxNest = 32\n", "const maxNest = 1 << 30\n", "TestRES29_")
m("RES-29 maxNest refuses its deepest level", "internal/shape/comsub.go",
  "const maxNest = 32\n", "const maxNest = 31\n", "TestRES29_")
m("RES-29 maxNest accepts one level past it", "internal/shape/comsub.go",
  "const maxNest = 32\n", "const maxNest = 33\n", "TestRES29_")
m("RES-29 a substitution recurses without the nesting check", "internal/shape/comsub.go",
  "func (r joined) comsubEnd(i, nest int) int {\n\tif nest > maxNest {",
  "func (r joined) comsubEnd(i, nest int) int {\n\tif false {", "TestRES29_")
m("RES-29 a quoted string recurses without the nesting check", "internal/shape/comsub.go",
  "func (r joined) dquoteEnd(i, nest int) int {\n\tif nest > maxNest {",
  "func (r joined) dquoteEnd(i, nest int) int {\n\tif false {", "TestRES29_")
m("RES-29 arithmetic recurses without the nesting check", "internal/shape/comsub.go",
  "func (r joined) arithEnd(i, nest int) int {\n\tif nest > maxNest {",
  "func (r joined) arithEnd(i, nest int) int {\n\tif false {", "TestRES29_")
