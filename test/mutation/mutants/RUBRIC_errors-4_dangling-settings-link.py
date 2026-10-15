# Rubric errors-4: a settings.json symlink that does not resolve used to read
# as "a path that does not exist yet", and watch renamed a regular file over
# the link. Write and ReadOriginal now refuse it by name.
m("errors-4 a dangling link is written over as a first install", "internal/settings/write.go",
  "\tif info.Mode()&fs.ModeSymlink == 0 {\n\t\treturn \"\", err\n\t}\n\treturn \"\", danglingLink(path, err)\n",
  "\tif info.Mode()&fs.ModeSymlink == 0 {\n\t\treturn \"\", err\n\t}\n\treturn path, nil\n",
  "TestRubricERRORS4_")
m("errors-4 reading a dangling link returns an empty file", "internal/settings/write.go",
  "\tif _, err := writeTarget(path); err != nil {\n\t\treturn nil, err\n\t}\n\tdata, err := os.ReadFile(path)",
  "\tdata, err := os.ReadFile(path)",
  "TestRubricERRORS4_")
m("errors-4 Load reads a dangling link as an absent file", "internal/settings/document.go",
  "\tif _, err := writeTarget(path); err != nil {\n\t\treturn nil, err\n\t}\n\tdata, err := os.ReadFile(path)",
  "\tdata, err := os.ReadFile(path)",
  "TestRubricERRORS4_")
