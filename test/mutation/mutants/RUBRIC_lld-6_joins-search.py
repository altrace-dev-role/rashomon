# rubric lld-6 (fix list 29): the work done on a payload is bounded, not only
# the bytes read. The offsets at which backslash-newlines were removed are
# searched, not scanned: a scan per here-document body line made a line of
# continuations and a body of short lines quadratic (800 KB took 13.8 s).
# Scanning again in either reader, or searching from the wrong edge, is the
# regression.
m("RUBRIC lld-6 joinedIn scans every offset", "internal/shape/comsub.go",
  "\tk := r.after(from)\n\treturn k < len(r.joins) && r.joins[k] <= to || r.unkept(to)\n",
  "\tfor _, p := range r.joins {\n\t\tif p > from && p <= to {\n\t\t\treturn true\n\t\t}\n\t}\n\treturn r.unkept(to)\n",
  "TestRubricLLD6_")
m("RUBRIC lld-6 delimLine scans every offset", "internal/shape/comsub.go",
  "\tfor _, p := range r.joins[r.after(i-1):] {\n\t\tif p > e {\n\t\t\tbreak\n\t\t}\n",
  "\tfor _, p := range r.joins {\n\t\tif p < i || p > e {\n\t\t\tcontinue\n\t\t}\n",
  "TestRubricLLD6_")
m("RUBRIC lld-6 the search includes a join at from", "internal/shape/comsub.go",
  "\treturn sort.SearchInts(r.joins, from+1)\n", "\treturn sort.SearchInts(r.joins, from)\n",
  "TestRubricLLD6_")
