# graph index leaves out a directory that holds pyvenv.cfg and prints a
# warning line naming it. The path goes through shown, so a directory name
# holding a newline cannot write a line of its own and one holding the
# right-to-left override cannot turn the rest of its line around. Printed
# as it is, the name's own rune reaches the terminal.
m("graph index prints an environment's path as it is", "cmd/rashomon/graph.go",
  "\t\tfmt.Fprintf(stdout, \"warning: %s/ not indexed: %s\\n\", shown(d.Path), shown(d.Why))\n",
  "\t\tfmt.Fprintf(stdout, \"warning: %s/ not indexed: %s\\n\", d.Path, shown(d.Why))\n",
  "TestGraphIndex_AnEnvironmentLineQuotesAControlOrFormatRune")

# LeftOut stats pyvenv.cfg without following a link. Following it takes a
# link to a regular file for an environment.
m("LeftOut follows a link named pyvenv.cfg", "internal/graph/lang/pylite/leave.go",
  "\tfi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel), envFile))\n",
  "\tfi, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel), envFile))\n",
  "TestLeftOut_ANamedPipeOrALinkNamedPyvenvCfgIsNotAnEnvironment")

# Only a regular file of that name makes an environment. Refusing only a
# directory takes a named pipe and a link for one.
m("LeftOut takes any pyvenv.cfg that is not a directory", "internal/graph/lang/pylite/leave.go",
  "\tif err != nil || !fi.Mode().IsRegular() {\n",
  "\tif err != nil || fi.IsDir() {\n",
  "TestLeftOut_ANamedPipeOrALinkNamedPyvenvCfgIsNotAnEnvironment")
