# RES-29: authority's scan for its terminator is bounded to authorityWindow
# bytes, past host.maxName plus room for a port. Lifting the window puts the
# unbounded scan -- to the end of a command line at every scheme match --
# back. Returning the window's prefix instead of refusing a candidate cut
# there keeps the scan bounded but hands host.Canonical a prefix that has
# lost the '@' of a credential longer than the window, so the username is
# recorded as a host.
m("RES-29 the authority scan window is lifted", "internal/shape/hosts.go",
  "const authorityWindow = 253 + 8\n", "const authorityWindow = 1 << 30\n", "TestAuthorityWindowBoundsTheScan")
m("RES-29 a cut authority is truncated instead of refused", "internal/shape/hosts.go",
  "\tif cut {\n\t\treturn \"\"\n\t}\n\treturn limit\n", "\t_ = cut\n\treturn limit\n", "TestAuthorityWindowRefusesACutAuthority")
