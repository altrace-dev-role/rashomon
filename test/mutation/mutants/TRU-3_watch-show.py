# TRU-3: --show prints the real screen and writes nothing, from any session.
m("TRU-3 --show goes on past the screen", "cmd/rashomon/watch.go",
  "\tif show {\n\t\t_, err := stdout.Write(screen)\n", "\tif show && false {\n\t\t_, err := stdout.Write(screen)\n",
  "TestTRU3_")
