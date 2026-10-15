# KV: KindVersion is a function, not an exported map, so no importer can
# change what AcceptsKind accepts by writing into it.
m("KV KindVersion reports every kind present", "internal/store/kinds.go",
  "\tv, ok := kindVersion[kind]\n\treturn v, ok\n}\n",
  "\tv := kindVersion[kind]\n\treturn v, true\n}\n", "TestKindVersionsAreTheFrozenTable")
