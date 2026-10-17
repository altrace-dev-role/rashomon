m("an over-cap id collapses to the empty string instead of a digest", "internal/store/limits.go",
  'func CapID(s string) string {\n\tif len(s) > MaxIDBytes {\n\t\treturn digestID(s)\n\t}\n\treturn s\n}\n',
  'func CapID(s string) string {\n\tif len(s) > MaxIDBytes {\n\t\treturn ""\n\t}\n\treturn s\n}\n',
  "TestCapID_OverCapGetsADistinctDigest")
