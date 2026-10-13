# rubric lld-9 (fix list 66): the lexer's per-word state is one value, reset
# whole when a word is emitted. Going back to a field-by-field reset that
# forgets one field is the regression; the reflection test sets every field.
m("RUBRIC lld-9 the word is reset field by field and forgets its backticks", "internal/shape/tokenize.go",
  "func (w *word) reset() { *w = newWord() }",
  "func (w *word) reset() { w.text.Reset(); w.started, w.glued, w.quotedAt, w.opaque = false, false, -1, false }",
  "TestRubricLLD9_")
m("RUBRIC lld-9 a fresh word is not quoted at -1", "internal/shape/tokenize.go",
  "func newWord() word { return word{quotedAt: -1} }",
  "func newWord() word { return word{} }",
  "TestRubricLLD9_|TestTokenize")
