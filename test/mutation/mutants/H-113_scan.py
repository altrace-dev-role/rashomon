# H-113, the fixture scan CI runs on every push, and internal/secretshape,
# whose deny list and entropy test are what it calls a secret shape. A break
# here lets one planted shape through, or lets the reviewed list excuse more
# than the value a person read; the secretshape breaks are also judged by the
# package's own tests, which were lifted with it.
SCAN = "test/tools/hygiene/scan.go"
SHAPES = "test/tools/hygiene/shapes.go"
H113 = "TestH113_"

m("H-113 .ndjson captures are not read", SCAN,
  "\tcase \".ndjson\", \".jsonl\":",
  "\tcase \".jsonl\":", H113)
m("H-113 testdata directories are not read", SCAN,
  "\t\tif dir == \"testdata\" {\n\t\t\treturn true",
  "\t\tif dir == \"testdata\" {\n\t\t\treturn false", H113)
m("H-113 archive members are not read", SCAN,
  "\tif archiveKind(name) != \"\" {\n\t\treturn scanArchive(name, data)\n\t}",
  "\tif archiveKind(name) != \"\" {\n\t\treturn nil\n\t}", H113)
m("H-113 a value is not asked whether it is secret-shaped", SCAN,
  "\tif !s.digests[v] && secretshape.Secretish(v) {",
  "\tif false && secretshape.Secretish(v) {", H113)
m("H-113 a value under a secret-named member passes", SCAN,
  "v != secretshape.Redacted && secretshape.SecretNamed(member) {",
  "v != secretshape.Redacted && false {", H113)
m("H-113 an inventory list in a document passes", SCAN,
  "\t\tif n > 0 {\n\t\t\ts.add(lineAt(at)",
  "\t\tif n < 0 {\n\t\t\ts.add(lineAt(at)", H113)
m("H-113 email addresses pass", SCAN,
  "\tif len(emailSpans(v)) > 0 {", "\tif false {", H113)
m("H-113 a UUID inside a path passes", SCAN,
  "\tif uuidInPath(v) {", "\tif false {", H113)
m("H-113 home directories pass", SCAN,
  "\tif homeIn(v) {", "\tif false {", H113)
m("H-113 a fingerprint ignores the value", SCAN,
  "what + \"\\x00\" + value))", "what))", H113)
m("H-113 an entry that excuses nothing is not reported", SCAN,
  "\t\t\tkept = append(kept, Finding{Reviewed, entries[key], \"an entry that excuses nothing: delete it\", \"\"})",
  "\t\t\t_ = key", H113)
m("H-113 any member name makes a hex run a digest", SHAPES,
  "(?i:[a-z0-9_]*(?:hash|digest|checksum|sha|sha1|sha256))", "(?i:[a-z0-9_]*)", H113)

# internal/secretshape, the lifted package.
SECRET = "internal/secretshape/secret.go"
REDACT = "internal/secretshape/redact.go"
m("H-113 the AWS key id rule is dropped from the deny list", SECRET,
  "\t`(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}`,\n", "",
  "TestDenyRulesSplitAsWritten|TestSecretShapedValuesRedacted|" + H113)
m("H-113 the entropy threshold sits above the mean", SECRET,
  "\treturn shannon(run) >= m.mu-3*m.sigma", "\treturn shannon(run) >= m.mu+3*m.sigma",
  "TestEntropyTestFlagsTheCanaries|TestSecretShapedValuesRedacted|" + H113)
m("H-113 a hex segment under any directory is a digest", REDACT,
  "\tif n == 0 || len(seg) != n {", "\tif len(seg) != 40 && len(seg) != 64 || n < 0 {",
  "TestDigestKeptOnlyUnderItsAlgorithm|TestSecretShapedValuesRedacted")
m("H-113 no member name is secret", REDACT,
  "\treturn len(keySpans(asciiLower(key)+\"=\")) > 0", "\treturn false",
  "TestSecretNamedIsTheKeyRule|" + H113)
m("H-113 a deny match is replaced without the rest of its token", REDACT,
  "\t\tlo := tokenStart(s, sp[0])\n\t\thi := tokenEnd(s, sp[1])",
  "\t\tlo, hi := sp[0], sp[1]", "TestRedactSpansAndCounts|TestProseIsScreenedWordByWord")
m("H-113 a capture outside test/ is not read", SCAN,
  'if parts[0] != "test" && parts[0] != "experiments" {',
  'if parts[0] != "test" {', H113)
m("H-113 JSON Lines transcripts are not read", SCAN,
  "\tcase \".ndjson\", \".jsonl\":",
  "\tcase \".ndjson\":", H113)
m("H-113 an environment file is not read", SCAN,
  '\tif environmentFile(rel) {\n\t\treturn true\n\t}\n',
  '', H113)
m("H-113 an environment file is read as text", SCAN,
  '\tif environmentFile(name) {\n\t\twhat := "an environment file',
  '\tif false {\n\t\twhat := "an environment file', H113)
