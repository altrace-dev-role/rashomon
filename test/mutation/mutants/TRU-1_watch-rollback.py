# TRU-1: watch's write commits or rolls back. A watch that cannot record
# what its settings edit changed puts settings.json back as it found it:
# the record is a step of applyWatch's writes, and its failure undoes the
# settings edit with the rest, in reverse.
m("TRU-1 a watch whose record failed keeps its settings edit", "cmd/rashomon/watch.go",
  "\t\treturn installID, nil, w.fail(stepRecord, err)\n",
  "\t\treturn installID, nil, err\n",
  "TestTRU1_")
m("TRU-1 the rollback writes watch's edit again", "cmd/rashomon/watch.go",
  "\t\treturn settings.Write(path, res.after, res.before)\n",
  "\t\treturn settings.Write(path, res.after, res.after)\n",
  "TestTRU1_")
m("TRU-1 the rollback leaves a settings.json watch created", "cmd/rashomon/watch.go",
  "\t\t\tif err := settings.Remove(path, res.after); err != nil {\n",
  "\t\t\tif err := error(nil); err != nil {\n",
  "TestTRU1_")
