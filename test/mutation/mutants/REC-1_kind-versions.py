# REC-1: every writer stamps KindVersion for its kind, and every call record
# names the binary that wrote it.
m("REC-1 the call records stay at schema 4", "internal/store/record.go",
  "const SchemaVersion = CallSchemaVersion", "const SchemaVersion = 4", "TestREC1_")
m("REC-1 a declaration names no writer", "internal/hook/handle.go",
  "\tdecl.Writer = Writer\n", "\n", "TestREC1_")
m("REC-1 a terminal names no writer", "internal/hook/handle.go",
  "\t\t\tTerminalV5:    store.TerminalV5{Writer: Writer},\n", "", "TestREC1_")
m("REC-1 an execution names no writer", "internal/hook/post.go",
  "\trec.Writer = Writer\n", "\n", "TestREC1_")
m("REC-1 the writer is not the build-time version", "cmd/rashomon/main.go",
  "\thook.Writer = buildVersion()\n", "\n", "TestREC1_")
