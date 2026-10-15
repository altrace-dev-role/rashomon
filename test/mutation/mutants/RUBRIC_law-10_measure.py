# Rubric law-10: functionSizes' own measurement must not have the three
# gaps Law 10 exists to catch everywhere else -- a second init() silently
# overwriting the first's count, a two-type-parameter receiver colliding
# with an unrelated one-parameter method of the same name, and a block
# comment counted as code.
m("RUBRIC law-10 a second same-named function overwrites the first's count",
  "test/acceptance/law10_function_length_test.go",
  "\t\tname := funcName(fn)\n\t\tseen[name]++\n\t\tkey := relPath + \":\" + name\n\t\tif n := seen[name]; n > 1 {\n\t\t\tkey += \"#\" + strconv.Itoa(n)\n\t\t}\n",
  "\t\tname := funcName(fn)\n\t\tseen[name]++\n\t\tkey := relPath + \":\" + name + strconv.Itoa(0)\n",
  "TestMeasureFile_TwoInitFunctionsBothCount")
m("RUBRIC law-10 a two-type-parameter receiver keys as the bare method name",
  "test/acceptance/law10_function_length_test.go",
  "\tswitch idx := typ.(type) {\n\tcase *ast.IndexExpr:\n\t\ttyp = idx.X\n\tcase *ast.IndexListExpr:\n\t\ttyp = idx.X\n\t}\n",
  "\tif idx, ok := typ.(*ast.IndexExpr); ok {\n\t\ttyp = idx.X\n\t}\n",
  "TestMeasureFile_AGenericMethodWithTwoTypeParametersKeysOnItsOwn")
m("RUBRIC law-10 a block comment's lines count as code",
  "test/acceptance/law10_function_length_test.go",
  "\tvar sc scanner.Scanner\n\tsc.Init(file, body, nil, 0)\n",
  "\tvar sc scanner.Scanner\n\tsc.Init(file, body, nil, scanner.ScanComments)\n",
  "TestCodeLines_ABlockCommentIsNotCode")
m("RUBRIC law-10 a blank line inside a multi-line string counts as code",
  "test/acceptance/law10_function_length_test.go",
  "\t\t\tif l-1 < len(seg) && strings.TrimSpace(seg[l-1]) == \"\" {\n\t\t\t\tcontinue\n\t\t\t}\n",
  "",
  "TestRubricLAW10_NoFileGrowsPastFiveHundredCodeLines")
