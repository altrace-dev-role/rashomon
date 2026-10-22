# RES-11, the baseline's row (rubric lld-10, fix list 53): a project baseline
# holds at most 4,096 hosts, the least recently seen going first and never
# one the folding session named, and a session that alone names more is
# unknown rather than folded.
m("RES-11 the 4,096-host check is removed", "internal/baseline/fold.go",
  "\tres.Evicted = evict(out.Hosts, named)\n", "\tres.Evicted = 0\n", "TestRES11_Every")
m("RES-11 the baseline cap is one host larger", "internal/baseline/fold.go",
  "const maxBaselineHosts = 4096\n", "const maxBaselineHosts = 4097\n", "TestRES11_Every")
m("RES-11 a session past the cap is folded", "internal/baseline/fold.go",
  "\tif len(named) > maxBaselineHosts {\n", "\tif false {\n", "TestRES11_Every")
m("RES-11 the most recently seen host goes first", "internal/baseline/fold.go",
  "\t\tif a != b {\n\t\t\treturn a < b\n", "\t\tif a != b {\n\t\t\treturn a > b\n", "TestRES11_Every")
m("RES-11 a host the session named can be evicted", "internal/baseline/fold.go",
  "\t\tif !keep[h] {\n", "\t\tif true {\n", "TestRES11_Every")
