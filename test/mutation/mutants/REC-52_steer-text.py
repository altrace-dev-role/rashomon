# REC-52, the steer texts: model_text and user_text are each capped at 2,048
# bytes; at the cap a text is kept, over it it is emptied and its *_withheld
# key says oversize, and the append says so.
S = "internal/store/steerlog.go"
L = "internal/steer/line.go"
m("REC-52 a steer text at its cap is withheld", S,
  "\tif len(text) <= MaxSteerTextBytes {\n", "\tif len(text) < MaxSteerTextBytes {\n", "TestREC52f_")
m("REC-52 a withheld steer text does not say why", S,
  "\t*withheld = w.oversize()\n", "\tw.oversize()\n", "TestREC52f_")
m("REC-52 the steer text cap is not 2,048", S,
  "const MaxSteerTextBytes = 2048\n", "const MaxSteerTextBytes = 4096\n", "TestREC52f_")
m("REC-52 user_text is not capped", L,
  "\tl.UserText, user = store.CapSteerText(l.UserText, &l.UserTextWithheld)\n", "", "TestREC52f_")
m("REC-52 a withheld user_text is not reported", L,
  "\treturn model || user\n", "\treturn model && user\n", "TestREC52f_")
