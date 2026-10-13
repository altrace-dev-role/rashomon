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
