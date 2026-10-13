# docs-9, naming-5: the zero Store writes nothing and digests nothing; it
# would otherwise write under the working directory with an empty key.
m("docs-9 the zero Store writes under the working directory", "internal/store/store.go",
  "\tif s.root == \"\" {\n\t\treturn errUnopenedStore\n\t}\n", "", "TestRubricDOCS9_AZeroStoreWritesNothing")
m("docs-9 the zero Store digests hosts under an empty key", "internal/store/gaps.go",
  "\tif host == \"\" || len(s.key) != keyLen {\n", "\tif host == \"\" {\n", "TestRubricDOCS9_AZeroStoreWritesNothing")
m("docs-9 the zero Store writes a store-level forget record", "internal/store/gaps.go",
  "func (s *Store) recordForgottenHost(digest string, now time.Time) error {\n\tif err := s.opened(); err != nil {\n\t\treturn err\n\t}\n",
  "func (s *Store) recordForgottenHost(digest string, now time.Time) error {\n", "TestRubricDOCS9_AZeroStoreWritesNothing")
