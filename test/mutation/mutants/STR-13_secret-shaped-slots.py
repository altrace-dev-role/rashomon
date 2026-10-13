# STR-13: secret-shaped values are refused: the secret deny list, the
# entropy test (tech 1.2.4) with its exact compiled table, the fact-kind
# allowlist and hostnames only; a listed secret-shaped path is left out of
# its list, and a whole lowercase 40- or 64-hex path segment is kept. Any
# rule dropped, or the table drifting, is the whole regression.
m("STR-13 the deny list stops matching AWS key ids", "internal/steer/render/slot/secret.go",
  "\t`(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}`,\n", "\t`(?:ZZZZ)[0-9A-Z]{16}`,\n", "TestSTR13_")
m("STR-13 a 40-character key passes", "internal/steer/render/slot/secret.go",
  "\t\tif i-start == 40 && mixedClasses(s[start:i]) {\n",
  "\t\tif i-start == -1 && mixedClasses(s[start:i]) {\n", "TestSTR13_")
m("STR-13 a bare 40- or 64-hex run is no longer always secret-shaped", "internal/steer/render/slot/secret.go",
  "\tif (n == 40 || n == 64) && isHex(run) {\n", "\tif false {\n", "TestSTR13_")
m("STR-13 a path segment skips the entropy test", "internal/steer/render/slot/path.go",
  "\t\tif len(seg) >= entropyMinRun && !isDigest(seg) && highEntropy(seg) {\n",
  "\t\tif len(seg) >= entropyMinRun && !isDigest(seg) && highEntropy(\"\") {\n", "TestSTR13_")
m("STR-13 a commit-id path segment is refused", "internal/steer/render/slot/path.go",
  "\tif len(seg) != 40 && len(seg) != 64 {\n", "\tif true {\n", "TestSTR13_")
m("STR-13 a listed secret-shaped path is printed", "internal/steer/render/slot/path.go",
  "\t\tcase errors.Is(err, ErrSecretShaped):\n\t\t\tmore++\n",
  "\t\tcase errors.Is(err, ErrSecretShaped):\n\t\t\tkept = append(kept, p)\n", "TestSTR13_")
m("STR-13 a fact value skips the entropy test", "internal/steer/render/slot/fact.go",
  "\tif highEntropy(f.Value) {\n", "\tif false {\n", "TestSTR13_")
m("STR-13 the fact-kind allowlist admits any kind", "internal/steer/render/slot/fact.go",
  "\tif !slices.Contains(AllFactKinds(), f.Kind) {\n",
  "\tif !slices.Contains(append(AllFactKinds(), f.Kind), f.Kind) {\n", "TestSTR13_")
m("STR-13 a fact value may be a URL", "internal/steer/render/slot/fact.go",
  "\tif urlRE.get().MatchString(f.Value) {\n", "\tif false {\n", "TestSTR13_")
m("STR-13 a package index host may carry a path", "internal/steer/render/slot/fact.go",
  "\tcase FactPackageIndexHost:\n\t\treturn charset(\"package index host\", f.Value, hostnameRE)\n",
  "\tcase FactPackageIndexHost:\n\t\treturn nil\n", "TestSTR13_")
m("STR-13 the compiled entropy table drifts from the exact moments", "internal/steer/render/slot/entropy_table.go",
  "4.0329854008", "4.5329854008", "TestSTR13_")
m("STR-13 Approved skips the deny list", "internal/steer/render/slot/slot.go",
  "\tif err := denied(\"approved text\", s); err != nil {\n", "\tif err := denied(\"approved text\", \"\"); err != nil {\n", "TestSTR13e_")
m("STR-13 Approved skips the entropy test", "internal/steer/render/slot/slot.go",
  "\t\tif highEntropy(w) {\n", "\t\tif highEntropy(w[:0]) {\n", "TestSTR13e_")
m("STR-13 the separated runs of a token go untested", "internal/steer/render/slot/secret.go",
  "\t\t\tif runHighEntropy(s[start:i]) || tokenInRun(s[start:i]) {\n", "\t\t\tif runHighEntropy(s[start:i]) {\n", "TestSTR13d_")
m("STR-13 a token's letter prefix is never stripped", "internal/steer/render/slot/secret.go",
  "\t\t\tif len(rest) >= entropyMinRun && alphabetIndex(rest) < k && runHighEntropy(rest) {\n", "\t\t\tif len(rest) >= entropyMinRun && alphabetIndex(rest) < k && runHighEntropy(rest[:0]) {\n", "TestSTR13d_")
m("STR-13 the Twilio key rule is dropped", "internal/steer/render/slot/secret.go",
  "|SK[0-9a-fA-F]{32}|", "|", "TestSTR13_")
m("STR-13 hf_, SG. and dapi match any word again", "internal/steer/render/slot/secret.go",
  r"(?:hf_[A-Za-z]{34}|SG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}|dapi[a-f0-9]{32})",
  r"(?:hf_|SG\.|dapi)[A-Za-z0-9_.=-]{16,}", "TestSTR13c_")
m("STR-13 a bearer token needs no separator again", "internal/steer/render/slot/secret.go",
  r"(?i:bearer)[\s:=]+[A-Za-z0-9._~+/=-]{20,}", r"(?i:bearer)[\s:=_+-]*[A-Za-z0-9._~+/=-]{20,}", "TestSTR13c_")
# Each vendor rule matched by the deny list alone, and the joined-run check
# for a token cut into parts under 20 characters (P07N-7).
m("STR-13 the Mailgun key rule is dropped", "internal/steer/render/slot/secret.go",
  "(?:key-[0-9a-z]{32}|SK", "(?:SK", "TestSTR13g_")
m("STR-13 the Postman key rule is dropped", "internal/steer/render/slot/secret.go",
  "|PMAK-[a-f0-9]{24}-[a-f0-9]{34})", ")", "TestSTR13g_")
m("STR-13 the private-key header rule is dropped", "internal/steer/render/slot/secret.go",
  "\t`-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY`,\n", "", "TestSTR13g_")
m("STR-13 a token cut into short runs goes untested", "internal/steer/render/slot/secret.go",
  "\treturn joinedRunHighEntropy(s)\n", "\treturn false\n", "TestSTR13f_")
m("STR-13 the joined-run check joins a UUID's groups", "internal/steer/render/slot/secret.go",
  "const minJoinPart = 12\n", "const minJoinPart = 4\n", "TestSTR13c_")
m("STR-13 the joined-run check joins lower-case words", "internal/steer/render/slot/secret.go",
  "\treturn upper && lower\n}\n", "\treturn true\n}\n", "TestSTR13c_")
