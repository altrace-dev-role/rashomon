# SETUP1: in a src layout, what lies outside src is named only when it is
# Python source. A .pyi stub is not a .py file: a directory holding stubs
# alone is not source outside the modules' root and gets no line. Reading a
# .pyi as a .py names that directory; TestSETUP1_PY4_'s stubs/pkg.pyi is the
# input that tells the two apart.
m("pylite Infer reads a .pyi stub as a .py file", "internal/graph/lang/pylite/infer.go",
  "\t\tpy := strings.HasSuffix(rel, \".py\")\n",
  "\t\tpy := strings.HasSuffix(rel, \".py\") || strings.HasSuffix(rel, \".pyi\")\n",
  "TestSETUP1_PY4_InASrcLayoutWhatLiesOutsideSrcIsNamed")
