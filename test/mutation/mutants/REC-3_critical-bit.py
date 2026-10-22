# REC-3: a record this reader does not accept is ignored when it says
# critical false and skipped, which unverifies the session, when it says
# critical true or carries no key.
m("REC-3 a non-critical record is skipped", "internal/store/read.go",
  "\t\tif head.Critical != nil && !*head.Critical {\n",
  "\t\tif false {\n", "TestREC3_|TestREC3c_")
m("REC-3 a record with no critical key is ignored", "internal/store/read.go",
  "\t\tif head.Critical != nil && !*head.Critical {\n",
  "\t\tif head.Critical == nil || !*head.Critical {\n", "TestREC3_|TestREC3c_")
m("REC-3 a known kind's unreadable header is passed over", "internal/store/read.go",
  "\t\tvar rec EventHeader\n\t\terr = json.Unmarshal(line, &rec)\n",
  "\t\tvar rec EventHeader\n\t\t_ = json.Unmarshal(line, &rec)\n", "TestREC3_")
m("REC-3 the gaps reader reads a gap at any version", "internal/store/read.go",
  " || g.Type != TypeGap || !AcceptsKind(TypeGap, g.SchemaVersion) {",
  " || g.Type != TypeGap {", "TestREC3b_")
