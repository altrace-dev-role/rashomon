m("H-21 the exit code is read from the whole message again", "internal/hook/post.go",
  "\tfirst, _, _ := strings.Cut(msg[len(exitCodePrefix):], \"\\n\")\n\tdigits := strings.TrimSpace(first)",
  "\tdigits := strings.TrimSpace(msg[len(exitCodePrefix):])", "TestH21_")
