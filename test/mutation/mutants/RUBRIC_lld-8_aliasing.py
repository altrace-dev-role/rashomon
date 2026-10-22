# lld-8: Store.Key and Document.Get hand out copies, never the bytes the
# Store digests with or the document renders.
m("lld-8 Key returns the Store's own slice", "internal/store/store.go",
  "func (s *Store) Key() []byte { return bytes.Clone(s.key) }", "func (s *Store) Key() []byte { return s.key }",
  "TestRubricLLD8_KeyIsACopy")
m("lld-8 Get returns the document's own bytes", "internal/settings/document.go",
  "\t\treturn bytes.Clone(m.raw), true\n", "\t\treturn m.raw, true\n", "TestRubricLLD8_GetIsACopy")
