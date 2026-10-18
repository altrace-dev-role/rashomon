# RES-7: the Stop payload is streamed, never held, and Stream copies only the
# values asked for. Reading recap's payload through the buffered Intake, or a
# path match that ignores the key, is the regression.
m("RES-7 recap reads the Stop payload whole through Intake", "cmd/rashomon/intake.go",
  "\terr := hookio.IntakeStream(cr, paths, func(i int, v []byte, truncated bool) bool {\n",
  "\twhole, rerr := hookio.Intake(cr)\n\tif rerr != nil {\n\t\treturn finalMessage{}, false\n\t}\n\terr := hookio.IntakeStream(bytes.NewReader(whole.Raw), paths, func(i int, v []byte, truncated bool) bool {\n",
  "TestRES7_")
IMPORTS["RES-7 recap reads the Stop payload whole through Intake"] = ("cmd/rashomon/intake.go", '\t"io"\n', '\t"bytes"\n\t"io"\n')
m("RES-7 Stream copies a value at a key nobody asked for", "internal/jsonscan/stream_walk.go",
  "\t\tif m&(1<<i) == 0 || d >= len(p.Elems) || !p.elemIs(d, elem) {\n",
  "\t\tif m&(1<<i) == 0 || d >= len(p.Elems) {\n",
  "TestRES7_")
