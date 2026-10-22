m("H-3  matcher installed as Bash", "internal/install/install.go", 'Matcher = "*"', 'Matcher = "Bash"', "TestH3_")
m("H-3  timeout installed as 600", "internal/install/install.go", "Timeout = 5\n", "Timeout = 600\n", "TestH3_")

m("H-3  executable path installed unquoted", "internal/install/install.go",
  "\tif safe {\n\t\treturn s\n\t}\n\treturn \"'\"", "\tif safe || true {\n\t\treturn s\n\t}\n\treturn \"'\"",
  "TestShellQuote|TestH3_SpacedExecutablePathRunsAsInstalled")
# The same three for cmd.exe's form, which every Windows install takes.
m("H-3  a Windows executable path installed unquoted", "internal/install/install.go",
  "\tif safe {\n\t\treturn s\n\t}\n\treturn `\"`", "\tif safe || true {\n\t\treturn s\n\t}\n\treturn `\"`",
  "TestShellQuote")
m("H-3  a Windows install is quoted for a POSIX shell", "internal/install/install.go",
  "\tif goos == \"windows\" {\n", "\tif false {\n",
  "TestShellQuote")
m("H-3  cmd.exe quoting leaves an embedded quote undoubled", "internal/install/install.go",
  "strings.ReplaceAll(s, `\"`, `\"\"`)", "s",
  "TestShellQuote")

m("tokenizer does not split on metacharacters", "internal/shape/tokenize.go",
  "\t\tcase isMeta(c):\n", "\t\tcase isMeta(c) && false:\n", "TestTokenize")
m("tokenizer emits the token an unterminated quote interrupted", "internal/shape/tokenize.go",
  "\tif !closed {\n\t\treturn i, l.unterminated()",
  "\tif !closed {\n\t\tw.started = true\n\t\tl.flush()\n\t\treturn i, l.unterminated()", "TestTokenize")
m("settings accepts a duplicate key", "internal/settings/document.go",
  "\t\tif seen[key] {\n\t\t\treturn nil, fmt.Errorf(\"duplicate key %q\", key)\n\t\t}\n\t\tseen[key] = true", "\t\tseen[key] = true",
  "TestParseRefuses|TestHookEntriesRefusesDuplicateEventKeys")
m("settings accepts trailing data after the object", "internal/settings/document.go",
  "\t\tif _, err := dec.Token(); err != io.EOF {\n\t\t\treturn nil, errors.New(\"trailing data after the top-level object\")\n\t\t}", "\t\t_ = io.EOF",
  "TestParseRefuses")
