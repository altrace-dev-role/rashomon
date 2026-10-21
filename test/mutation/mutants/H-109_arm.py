# H-109, the keyed holdout assigner. Each break is one way a randomiser stops
# being a fair, keyed, reproducible coin, and each names the test that says so.
# The last three are judged by the package's unit tests, which hold the edges
# the acceptance item does not reach.
ARM = "internal/arm/arm.go"

m("H-109 the holdout share is ignored", ARM,
  "\tif a.draw(feature, sessionID) < cut(holdout) {",
  "\tif a.draw(feature, sessionID) < cut(0.5) {", "TestH109_TheSplit")
m("H-109 the complement of the share is held out", ARM,
  "\tif a.draw(feature, sessionID) < cut(holdout) {",
  "\tif a.draw(feature, sessionID) >= cut(holdout) {", "TestH109_TheSplit")

m("H-109 the draw changes from call to call", ARM,
  "\treturn binary.BigEndian.Uint64(m.Sum(nil)[:8])",
  "\treturn binary.BigEndian.Uint64(m.Sum(nil)[:8]) ^ rand.Uint64()", "TestH109_TheSameSession")
IMPORTS["H-109 the draw changes from call to call"] = (ARM,
  "\t\"errors\"\n", "\t\"errors\"\n\t\"math/rand/v2\"\n")

m("H-109 the domain separator is dropped", ARM,
  "\tm.Write([]byte(domain))\n", "", "TestH109_TheDerivationIsPinned")
m("H-109 the key is left out of the digest", ARM,
  "m := hmac.New(sha256.New, a.key)", "m := hmac.New(sha256.New, nil)", "TestH109_ADifferentInstallKey")
m("H-109 the feature name is left out of the digest", ARM,
  "\tm.Write([]byte(feature))\n", "", "TestH109_TwoFeatures")
m("H-109 a layout draws a factor differently from the feature alone", ARM,
  "got, err := a.Assign(f.Feature, sessionID, f.Holdout)",
  "got, err := a.Assign(f.Feature+string(rune('0'+i)), sessionID, f.Holdout)", "TestH109_TwoFeatures")
m("H-109 a layout may name one feature twice", ARM,
  "\t\t\tif earlier.Feature == f.Feature {",
  "\t\t\tif false && earlier.Feature == f.Feature {", "TestH109_TwoFeatures")

m("H-109 a session without an id is given an arm", ARM,
  "\tif sessionID == \"\" {\n\t\treturn \"\", ErrNoSession\n\t}\n", "", "TestH109_ASessionWithoutAnId")
m("H-109 an assigner is made without a key", ARM,
  "\tif len(key) == 0 {\n\t\treturn Assigner{}, ErrNoKey\n\t}\n", "", "TestH109_ASessionWithoutAnId")

m("H-109 an environment variable overrides the arm", ARM,
  "\tif sessionID == \"\" {\n\t\treturn \"\", ErrNoSession\n\t}\n",
  "\tif v := os.Getenv(\"RASHOMON_ARM\"); v != \"\" {\n\t\treturn Arm(v), nil\n\t}\n"
  "\tif sessionID == \"\" {\n\t\treturn \"\", ErrNoSession\n\t}\n", "TestH109_TheArmDependsOnNothing")
IMPORTS["H-109 an environment variable overrides the arm"] = (ARM,
  "\t\"errors\"\n", "\t\"errors\"\n\t\"os\"\n")

m("H-109 the caller's key slice is kept rather than copied", ARM,
  "return Assigner{key: append([]byte(nil), key...)}, nil", "return Assigner{key: key}, nil",
  "TestNew_CopiesTheKey")
m("H-109 a feature name may contain a NUL", ARM,
  "feature == \"\" || strings.IndexByte(feature, 0) >= 0",
  "feature == \"\" || strings.IndexByte(feature, 0) < -1", "TestAssign_RefusesAFeatureName")
m("H-109 a share of 0, 1 or NaN is accepted", ARM,
  "\tif !(holdout > 0 && holdout < 1) {", "\tif holdout < 0 || holdout > 1 {",
  "TestAssign_RefusesAShare")
