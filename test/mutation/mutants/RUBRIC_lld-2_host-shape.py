# rubric lld-2 (fix list 10): a host cut out of a command line is recorded
# only if it has the shape of a host, and no host is longer than 253 bytes.
# Recording what authority cut without asking, letting `=` or `!` pass, or
# dropping either length bound is the regression.
m("RUBRIC lld-2 a URL's host is recorded without the shape check", "internal/shape/hosts.go",
  "ok && host.Shaped(h) {", "ok {", "TestRubricLLD2_")
m("RUBRIC lld-2 Canonical has no length bound", "internal/host/host.go",
  "\tif h == \"\" || h == \".\" || len(h) > maxName {", "\tif h == \"\" || h == \".\" {", "TestRubricLLD2_")
m("RUBRIC lld-2 Shaped has no length bound", "internal/host/host.go",
  "\tif s == \"\" || len(s) > maxName || !utf8.ValidString(s) {", "\tif s == \"\" || !utf8.ValidString(s) {", "TestRubricLLD2_")
m("RUBRIC lld-2 the bound is one byte long", "internal/host/host.go",
  "const maxName = 253\n", "const maxName = 254\n", "TestRubricLLD2_")
m("RUBRIC lld-2 = and ! are host bytes", "internal/host/host.go",
  "\t\tcase r == '-' || r == '.' || r == '_':\n", "\t\tcase r == '-' || r == '.' || r == '_' || r == '=' || r == '!':\n", "TestRubricLLD2_")
