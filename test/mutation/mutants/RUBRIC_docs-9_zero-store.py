# docs-9, naming-5: the zero Store writes nothing and digests nothing; it
# would otherwise write under the working directory with an empty key.
m("docs-9 the zero Store writes under the working directory", "internal/store/store.go",
  "\tif s.root == \"\" {\n\t\treturn errUnopenedStore\n\t}\n", "", "TestRubricDOCS9_AZeroStoreWritesNothing")
m("docs-9 the zero Store digests hosts under an empty key", "internal/store/gaps.go",
  "\tif host == \"\" || len(s.key) != keyLen {\n", "\tif host == \"\" {\n", "TestRubricDOCS9_AZeroStoreWritesNothing")
