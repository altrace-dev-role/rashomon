# TRU-58: a baseline cut off mid-write ends inside the last string it was
# writing, with no closing quote. What survived can be a PREFIX of the host a
# forget names, not the whole quoted form, and that must still count as
# holding it.
m("TRU-58 forget misses a host truncated mid-write", "internal/baseline/baseline.go",
  "\treturn frag != \"\" && len(frag) <= len(host) && strings.HasPrefix(host, frag)\n",
  "\treturn false && frag != \"\" && len(frag) <= len(host) && strings.HasPrefix(host, frag)\n",
  "TestTRU58_ForgetMatchesATruncatedHostname")
