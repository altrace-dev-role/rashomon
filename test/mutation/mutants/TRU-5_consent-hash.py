# TRU-5: text_sha256 is the hash of the exact bytes shown.
m("TRU-5 text_sha256 hashes only part of what was shown", "internal/consent/screen.go",
  "\tsum := sha256.Sum256(shown)\n", "\tsum := sha256.Sum256(shown[:len(shown)/2])\n",
  "TestTRU5_")
