# VERSION-BUILDINFO: resolveVersion's three branches and the one consumer that
# prints it. Each break fails TestVersion_ResolveVersion.
m("a go install of a tag still prints dev", "cmd/rashomon/version.go",
  "\t\t\treturn strings.TrimPrefix(v, \"v\")\n",
  "\t\t\t_ = v\n",
  "TestVersion_ResolveVersion")
m("the module version keeps its leading v", "cmd/rashomon/version.go",
  "\t\t\treturn strings.TrimPrefix(v, \"v\")\n",
  "\t\t\treturn v\n",
  "TestVersion_ResolveVersion")
m("a local build's (devel) is taken as a version", "cmd/rashomon/version.go",
  "v != \"\" && v != \"(devel)\"",
  "v != \"\"",
  "TestVersion_ResolveVersion")
m("an empty module version is taken as a version", "cmd/rashomon/version.go",
  "v != \"\" && v != \"(devel)\"",
  "v != \"(devel)\"",
  "TestVersion_ResolveVersion")
m("the build info outranks the linker's value", "cmd/rashomon/version.go",
  "\tif ldflags != \"\" && ldflags != \"dev\" {\n\t\treturn ldflags\n\t}\n\tif ok && bi != nil {",
  "\tif ok && bi != nil && bi.Main.Version != \"\" {\n\t\treturn strings.TrimPrefix(bi.Main.Version, \"v\")\n\t}\n\tif ldflags != \"\" && ldflags != \"dev\" {\n\t\treturn ldflags\n\t}\n\tif ok && bi != nil {",
  "TestVersion_ResolveVersion")
m("the linker's value is ignored", "cmd/rashomon/version.go",
  "\tif ldflags != \"\" && ldflags != \"dev\" {\n\t\treturn ldflags\n\t}\n\tif ok && bi != nil {",
  "\tif ok && bi != nil {",
  "TestVersion_ResolveVersion")
m("an empty linker value is taken as set", "cmd/rashomon/version.go",
  "\tif ldflags != \"\" && ldflags != \"dev\" {\n\t\treturn ldflags\n\t}\n\tif ok && bi != nil {",
  "\tif ldflags != \"dev\" {\n\t\treturn ldflags\n\t}\n\tif ok && bi != nil {",
  "TestVersion_ResolveVersion")
m("an unreadable build info is read anyway", "cmd/rashomon/version.go",
  "\tif ok && bi != nil {\n\t\tif v :=",
  "\tif bi != nil {\n\t\tif v :=",
  "TestVersion_ResolveVersion")
m("nothing resolves to dev", "cmd/rashomon/version.go",
  "\t}\n\treturn \"dev\"\n}",
  "\t}\n\treturn \"\"\n}",
  "TestVersion_ResolveVersion")
