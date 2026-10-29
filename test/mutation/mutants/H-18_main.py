m("H-18 the post path installs on every call", "cmd/rashomon/post.go",
  "\tp := hook.NewPost(st, entryNow)\n", "\t_ = cmdWatch(io.Discard)\n\tp := hook.NewPost(st, entryNow)\n", "TestH18_")

m("H-18 the hook installs on every call", "cmd/rashomon/main.go",
  "\th := newHandler(st, hid)\n", "\t_ = cmdWatch(io.Discard)\n\th := newHandler(st, hid)\n", "TestH18_")
m("H-18 detach writes the settings file although it changed nothing", "cmd/rashomon/main.go",
  "\t\tn, l, err := remove(doc)\n\t\tremoved, left = n, l\n\t\treturn n > 0, err",
  "\t\tn, l, err := remove(doc)\n\t\tremoved, left = n, l\n\t\treturn true, err", "TestH18_")
