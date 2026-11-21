# The install call goes in above openForHook: it returns nil for an entry that
# names an install with no install.json, and the test helpers pass --install,
# so a line after it is never reached. main.go's anchor is the hook's own call,
# the one with a blank line above it; the probe's has none.
m("H-18 the post path installs on every call", "cmd/rashomon/post.go",
  "\t\tst := openForHook(args, stderr)\n",
  "\t\t_ = cmdWatch([]string{\"--yes\"}, io.Discard)\n\t\tst := openForHook(args, stderr)\n", "TestH18_")

m("H-18 the hook installs on every call", "cmd/rashomon/main.go",
  "\t\t}\n\n\t\tst := openForHook(args, stderr)\n",
  "\t\t}\n\n\t\t_ = cmdWatch([]string{\"--yes\"}, io.Discard)\n\t\tst := openForHook(args, stderr)\n", "TestH18_")
m("H-18 detach writes the settings file although it changed nothing", "cmd/rashomon/main.go",
  "\t\tn, l, err := remove(doc)\n\t\tremoved, left = n, l\n\t\treturn n > 0, err",
  "\t\tn, l, err := remove(doc)\n\t\tremoved, left = n, l\n\t\treturn true, err", "TestH18_")
