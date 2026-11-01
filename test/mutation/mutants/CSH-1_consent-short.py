# CONSENT-SHORT1: the screen a yes is asked under is one default terminal,
# 24 rows of 80 columns, from the inventory's short paragraphs; the five
# sections are --full's, which writes nothing.
S = "internal/consent/short.go"
W = "cmd/rashomon/watch.go"
C = "cmd/rashomon/consent.go"

m("CSH-1 the ceiling is raised", S,
  "const MaxShortLines = 24\n", "const MaxShortLines = 40\n", "TestCSH1_")
m("CSH-1 the short screen drops what a yes records", S,
  "for _, p := range paras[disclose.CiteShort] {", "for _, p := range paras[disclose.CiteShort][1:] {", "TestCSH1_")
m("CSH-1 Covers reads the scope alone", S,
  "return ok && scope >= disclose.ScopeVersion && disclosure >= steer.DisclosureVersion",
  "return ok && scope >= disclose.ScopeVersion && disclosure >= 0", "TestCSH1_")
m("CSH-1 watch asks under the five sections", W,
  "consent.Short(inv, plan.screenInput())", "consent.Screen(inv, plan.screenInput())", "TestCSH1_|TestTRU3_")
m("CSH-1 consent asks under the five sections", C,
  "screen, err := consent.Short(inv, plan.screenInput())", "screen, err := consent.Screen(inv, plan.screenInput())", "TestCSH1_")
m("CSH-1 --full prints the short screen", C,
  "screen, err := consent.Screen(inv, plan.screenInput())", "screen, err := consent.Short(inv, plan.screenInput())", "TestCSH1_")
m("CSH-1 --full takes an answer flag", C,
  "if full && (yes || show) {", "if full && yes && show {", "TestCSH1_")
