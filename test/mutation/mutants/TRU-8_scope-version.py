# TRU-8: the disclosure scope version is pinned; a bump without the review the
# consent screen rests on is the whole regression.
m("TRU-8 the disclosure scope version moves without a review", "internal/disclose/disclose.go",
  "const ScopeVersion = 5", "const ScopeVersion = 6", "TestTRU8b_")
