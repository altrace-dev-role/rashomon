# rubric lld-10 (fix list 53): a render that learns nothing writes nothing,
# and a baseline file past its derived size is not read whole: exactly the
# size is read, one byte more is refused.
m("RUBRIC lld-10 an unchanged render rewrites the file", "internal/baseline/baseline.go",
  "\tif res.Err != nil || !changed {\n", "\tif res.Err != nil || !changed && false {\n", "TestRubricLLD10_")
m("RUBRIC lld-10 an oversized baseline is read", "internal/baseline/fold.go",
  "\tif len(body) > maxBaselineBytes {\n\t\treturn nil, errTooLarge\n", "\tif false {\n\t\treturn nil, errTooLarge\n", "TestRubricLLD10_")
m("RUBRIC lld-10 a baseline of exactly the bound is refused", "internal/baseline/fold.go",
  "\tif len(body) > maxBaselineBytes {\n\t\treturn nil, errTooLarge\n", "\tif len(body) >= maxBaselineBytes {\n\t\treturn nil, errTooLarge\n", "TestRubricLLD10_")
