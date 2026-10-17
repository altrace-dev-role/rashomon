# TRU-1: watch's write commits or rolls back. A watch that cannot record
# what its settings edit changed puts settings.json back as it found it.
m("TRU-1 a watch whose record failed keeps its settings edit", "cmd/rashomon/main.go",
  "\t\treturn rollBackWatch(stdout, loc.User, edit, st.InstallID(), err)\n",
  "\t\treturn err\n",
  "TestTRU1_")
m("TRU-1 the rollback writes watch's edit again", "cmd/rashomon/detach.go",
  "\t\terr = settings.Write(path, edit.after, edit.before)\n",
  "\t\terr = settings.Write(path, edit.after, edit.after)\n",
  "TestTRU1_")
m("TRU-1 the rollback leaves a settings.json watch created", "cmd/rashomon/detach.go",
  "\t\terr = settings.Remove(path, edit.after)\n",
  "\t\terr = nil\n",
  "TestTRU1_")
