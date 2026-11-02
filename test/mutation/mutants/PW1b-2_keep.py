# PW1b-ON-230 step 2: the Stop's read holds only the written packages' facts.
# decode checks every record in place and builds a fact only for a subject
# keep holds; Impact hands it keepFor its wanted packages; keepFor holds a
# package's own symbols of every kind, matched on the path and its dot.
P = "internal/platform/platform.go"
D = "internal/platform/decode.go"
KEEPIF = "\t\t\tif len(rest) > len(p) && rest[len(p)] == '.' && string(rest[:len(p)]) == p {"

m("PW1b-2 keep holds funcs only", P,
  KEEPIF, KEEPIF.replace("if len(rest)", 'if string(kind) == "func" && len(rest)'), "TestPW1b2_KeepGives")
m("PW1b-2 keep needs no dot after the path", P,
  KEEPIF, "\t\t\tif len(rest) > len(p) && string(rest[:len(p)]) == p {", "TestPW1b2_KeepGives")
m("PW1b-2 the fact is built before keep", D,
  "\t\tif keep != nil && !keep(c[1], c[3]) {\n\t\t\tcontinue\n\t\t}\n\t\tf := c.fact()",
  "\t\tf := c.fact()\n\t\tif keep != nil && !keep(c[1], c[3]) {\n\t\t\tcontinue\n\t\t}", "TestPW1b2_ADroppedRecord")
m("PW1b-2 Impact passes no keep", P,
  "decode(ctx, io.TeeReader(f, h), hook, keepFor(wanted))", "decode(ctx, io.TeeReader(f, h), hook, nil)",
  "TestPW1b2_ImpactHolds")
m("PW1b-2 keep is asked before the grammar", D,
  "\t\tc, assert, err := parseRecord(line)\n\t\tif err != nil {",
  "\t\tif keep != nil {\n\t\t\tif s, _, ok := bytes.Cut(line[min(2, len(line)):], []byte(\"\\t\")); ok && !keep(s, nil) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t}\n\t\tc, assert, err := parseRecord(line)\n\t\tif err != nil {",
  "TestPW1b2_ARefusalUnder")
