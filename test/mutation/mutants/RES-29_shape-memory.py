# RES-29: a pathological 8 MiB line keeps every reading of it -- Derive,
# LeadingDirectory, Hosts -- to a few times the payload. Copying every word,
# growing a copy a quarter at a time, keeping an offset per continuation,
# forgetting that offsets were dropped, reading lines past the token bound,
# parsing every name as a number, lowering the line twice, decoding the
# command into a grown buffer, or reading the line again for the leading cds
# is each the regression.
m("RES-29 every word is copied", "internal/shape/tokenize.go",
  "\tcase w.from == w.to:\n\t\tw.from, w.to = from, to\n\t\treturn\n\tcase w.to == from:\n\t\tw.to = to\n\t\treturn\n",
  "", "TestRES29_")
m("RES-29 a copied word grows a quarter at a time", "internal/shape/tokenize.go",
  "\tw.text.Grow(to - from)\n\tw.text.WriteString(s[from:to])\n",
  "\tw.text.WriteString(s[from:to])\n", "TestRES29_")
m("RES-29 every removed backslash-newline is kept", "internal/shape/join.go",
  "\t\t\t\tif len(joins) < maxJoins {\n", "\t\t\t\tif true {\n", "TestRES29_")
m("RES-29 dropped offsets are forgotten", "internal/shape/join.go",
  "\t\t\t\t} else {\n\t\t\t\t\tlost = true\n\t\t\t\t}\n", "\t\t\t\t}\n", "TestRES29_")
m("RES-29 a span past the kept offsets is read as certain", "internal/shape/comsub.go",
  "\treturn r.lost && to >= r.joins[len(r.joins)-1]\n", "\treturn false\n", "TestRES29_")
m("RES-29 the leading cds are read again for the masked status", "internal/shape/shape.go",
  "\t\tdirs, _ = leadingRun(pshaped, perr)\n", "\t\tdirs, _ = leadingDirectories(cmd)\n", "TestRES29_")
m("RES-29 ssh lines are read past the token bound", "internal/shape/hosts.go",
  "\tfor rest, budget := text, maxTokens; rest != \"\" && budget > 0; {\n",
  "\tfor rest, budget := text, maxTokens; rest != \"\"; {\n", "TestRES29_")
m("RES-29 an empty line costs the ssh reading nothing", "internal/shape/hosts.go",
  "\t\tbudget -= max(read, 1)\n", "\t\tbudget -= read\n", "TestRES29_")
m("RES-29 every name is parsed as an inet_aton address", "internal/host/host.go",
  "\tif !atonShaped(h) {\n\t\treturn netip.Addr{}, false\n\t}\n", "", "TestRES29_")
m("RES-29 the line is lowered for each scheme list", "internal/shape/hosts.go",
  "\tssh = collect(lower, sshSchemes)\n", "\tssh = collect(strings.ToLower(text), sshSchemes)\n", "TestRES29_")
m("RES-29 the command is decoded into a buffer grown from nothing", "internal/jsonscan/unquote.go",
  "\tvar out strings.Builder\n\tout.Grow(len(b))\n", "\tvar out strings.Builder\n", "TestRES29_")
m("RES-29 an escaped blank ends a word in the token estimate", "internal/shape/tokenize.go",
  "\t\tcase c == '\\\\': // the byte it escapes is the word's, a blank included\n\t\t\ti++\n\t\t\tinWord = true\n",
  "", "TestRES29_")

