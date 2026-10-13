m("H-3  matcher installed as Bash", "internal/install/install.go", 'Matcher = "*"', 'Matcher = "Bash"', "TestH3_")
m("H-3  timeout installed as 600", "internal/install/install.go", "Timeout = 5\n", "Timeout = 600\n", "TestH3_")

m("H-3  executable path installed unquoted", "internal/install/install.go",
  "\tif safe {\n\t\treturn s\n\t}\n", "\tif safe || true {\n\t\treturn s\n\t}\n",
  "TestShellQuote|TestH3_SpacedExecutablePathRunsAsInstalled")

m("tokenizer does not split on metacharacters", "internal/shape/tokenize.go",
  "\t\tcase isMeta(c):\n", "\t\tcase isMeta(c) && false:\n", "TestTokenize")
m("tokenizer emits the token an unterminated quote interrupted", "internal/shape/tokenize.go",
  "\t\t\tif !closed {\n\t\t\t\treturn toks, unterminated()",
  "\t\t\tif !closed {\n\t\t\t\tstarted = true\n\t\t\t\tflush()\n\t\t\t\treturn toks, unterminated()", "TestTokenize")
m("settings accepts a duplicate key", "internal/settings/document.go",
  "\t\tif seen[key] {\n\t\t\treturn nil, fmt.Errorf(\"duplicate key %q\", key)\n\t\t}\n\t\tseen[key] = true", "\t\tseen[key] = true",
  "TestParseRefuses|TestHookEntriesRefusesDuplicateEventKeys")
m("settings accepts trailing data after the object", "internal/settings/document.go",
  "\t\tif _, err := dec.Token(); err != io.EOF {\n\t\t\treturn nil, errors.New(\"trailing data after the top-level object\")\n\t\t}", "\t\t_ = io.EOF",
  "TestParseRefuses")
