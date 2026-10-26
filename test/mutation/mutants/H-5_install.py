# afterFirstWord's single-quoted branch has no requirement-row test of its
# own -- TestLUI3_ and TestLUI4_ cover the whole-remainder comparison and the
# legacy-form lookup, both downstream of this split, but neither one's
# command is ever quoted. Every Windows install goes through this branch
# (shellQuote quotes any path with a backslash), as does any POSIX install
# under a path with a space or an embedded quote. This mutant breaks the
# escaped-quote skip: resuming the search one byte past the closing quote,
# instead of four, lands inside the backslash-quote-open sequence itself and
# finds a false terminator there, so a quoted executable with an embedded
# quote never resolves to an owner.
m("afterFirstWord resumes inside the escaped-quote sequence instead of past it",
  "internal/install/install.go",
  "\t\tif strings.HasPrefix(cmd[end:], `'\\''`) {\n"
  "\t\t\t// An embedded quote in the executable, escaped as close,\n"
  "\t\t\t// backslash, quote, open -- not the terminator.\n"
  "\t\t\ti = end + 4\n"
  "\t\t\tcontinue\n"
  "\t\t}",
  "\t\tif strings.HasPrefix(cmd[end:], `'\\''`) {\n"
  "\t\t\t// An embedded quote in the executable, escaped as close,\n"
  "\t\t\t// backslash, quote, open -- not the terminator.\n"
  "\t\t\ti = end + 1\n"
  "\t\t\tcontinue\n"
  "\t\t}",
  "TestOwner/quoted_executable_with_an_embedded_single_quote")

# afterFirstWord's double-quoted branch: the entry watch writes on Windows,
# where the executable is quoted for cmd.exe. Read as a bare word, a path
# with a space ends at its first space and no Windows entry is ours; a
# doubled quote taken for the terminator ends the word inside the path.
m("afterFirstWord reads a cmd.exe-quoted executable as a bare word",
  "internal/install/install.go",
  "\tif cmd[0] == '\"' {\n\t\treturn afterDoubleQuoted(cmd)\n\t}\n", "",
  "TestOwner/cmd.exe-quoted_executable_with_a_space")
m("afterDoubleQuoted resumes inside a doubled quote instead of past it",
  "internal/install/install.go",
  "\t\t\ti = end + 2\n", "\t\t\ti = end + 1\n",
  "TestOwner/cmd.exe-quoted_executable_with_an_embedded_quote")
