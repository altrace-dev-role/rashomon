# VER-5.6: the spec's holdout object (PF-MS5; README 5.6, folded into card 3.1).
# Each break removes one rule of the object or moves one bound by a step; the
# kill row names it. Test names are written out whole: 2.5's steer and hook
# tests share the TestMS5 prefixes.
P = "internal/spec/parse.go"
S = "internal/spec/spec.go"
TREE = "internal/spec/tree.go"
A = "TestMS5a_NoHoldoutObjectIsOnAtDefaultRate"
B = "TestMS5b_OffIsOff"
C = "TestMS5c_RateBounds"
D = "TestMS5d_WindowClosesTheDayAfter"
E = "TestMS5e_StartedWithoutWindowRefused"
G = "TestMS5g_DigestMovesWithHoldout"

m("VER-5.6 off is ignored", S, "\tif h.Off {\n\t\treturn false, 0\n\t}\n", "\tif false {\n\t\treturn false, 0\n\t}\n", B)
m("VER-5.6 the holdout object is not read", P, '\tf.Holdout = field(r, top, "holdout", false, r.holdout)\n', "", B)
m("VER-5.6 the rate floor is 499", S, "\tMinHoldoutBP         = 500\n", "\tMinHoldoutBP         = 499\n", C)
m("VER-5.6 the rate floor is 501", S, "\tMinHoldoutBP         = 500\n", "\tMinHoldoutBP         = 501\n", C)
m("VER-5.6 the rate ceiling is 1999", S, "\tMaxHoldoutBP         = 2000\n", "\tMaxHoldoutBP         = 1999\n", C)
m("VER-5.6 the rate ceiling is 2001", S, "\tMaxHoldoutBP         = 2000\n", "\tMaxHoldoutBP         = 2001\n", C)
m("VER-5.6 the shortest window is 0 days", P, "return r.intIn(n, 1, MaxHoldoutWindowDays, ReasonHoldoutWindow)",
  "return r.intIn(n, 0, MaxHoldoutWindowDays, ReasonHoldoutWindow)", D)
m("VER-5.6 the shortest window is 2 days", P, "return r.intIn(n, 1, MaxHoldoutWindowDays, ReasonHoldoutWindow)",
  "return r.intIn(n, 2, MaxHoldoutWindowDays, ReasonHoldoutWindow)", D)
m("VER-5.6 the longest window is 364 days", S, "\tMaxHoldoutWindowDays = 365\n", "\tMaxHoldoutWindowDays = 364\n", D)
m("VER-5.6 the longest window is 366 days", S, "\tMaxHoldoutWindowDays = 365\n", "\tMaxHoldoutWindowDays = 366\n", D)
m("VER-5.6 the window comparison is inverted", S, "!now.Before(h.Started.AddDate(0, 0, h.WindowDays+1))",
  "now.Before(h.Started.AddDate(0, 0, h.WindowDays+1))", D)
m("VER-5.6 the window closes on its last day", S, "h.Started.AddDate(0, 0, h.WindowDays+1)", "h.Started.AddDate(0, 0, h.WindowDays)", D)
m("VER-5.6 started without window_days is accepted", P, "\tcase started && !window:\n", "\tcase false:\n", E)
m("VER-5.6 window_days without started is accepted", P, "\tcase window && !started:\n", "\tcase false:\n", E)
m("VER-5.6 a holdout without a rate reads no default", P, "\tif h.RateBP == 0 {\n\t\th.RateBP = DefaultHoldoutBP\n\t}\n", "", A)
m("VER-5.6 On reads no default rate", S, "\tif h.RateBP == 0 {\n\t\treturn true, DefaultHoldoutBP\n\t}\n", "", A)
m("VER-5.6 holdout is left out of the digest", TREE,
  "\t\tfor i, j := range sortedKeys(n.keys) {\n\t\t\tif i > 0 {\n",
  "\t\tfor i, j := range sortedKeys(n.keys) {\n\t\t\tif n.keys[j] == \"holdout\" {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif i > 0 {\n", G)
