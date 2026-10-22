# TRU-5: text_sha256 is the hash of the exact bytes shown.
m("TRU-5 text_sha256 hashes only part of what was shown", "internal/consent/screen.go",
  "\tsum := sha256.Sum256(shown)\n", "\tsum := sha256.Sum256(shown[:len(shown)/2])\n",
  "TestTRU5_")
m("TRU-5 watch records the hash of other bytes than it showed", "cmd/rashomon/watch.go",
  "\t\tTextSHA256:      consent.TextSHA256(screen),\n", "\t\tTextSHA256:      consent.TextSHA256(screen[1:]),\n",
  "TestTRU5_")
m("TRU-5 the terminal shows other bytes than --show", "cmd/rashomon/watch.go",
  "\tif _, err := tty.Write(screen); err != nil {\n", "\tif _, err := tty.Write(screen[1:]); err != nil {\n",
  "TestTRU5_")
