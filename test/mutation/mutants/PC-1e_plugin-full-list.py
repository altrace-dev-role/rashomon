# PLUGIN-CONSENT1, the DE's #228 verdict: the plugin description ends with
# where the full list of what is kept is, the README section that quotes the
# full screen's STORED paragraph word for word; the first-session lines name
# the same list; an enabled plugin's records carry what the watch route's do
# (B306).
P = "cmd/rashomon/plugin_consent.go"

m("PC-1e the README quote drops a clause", "README.md",
  "> transcript path; the permission mode; at record scope v5",
  "> transcript path; at record scope v5", "TestPC1e_")
m("PC-1e the README heading loses its anchor", "README.md",
  "## What it records, and what it never stores", "## What it records", "TestPC1e_")
m("PC-1e the first-session lines drop the full list", P,
  " +\n\t\t\"\\nFull list of what it keeps: the same, with consent --full in place of report\"", "", "TestPC1b_")
m("PC-1e the plugin's start writes no consent, so its records withhold", P,
  "if phase != store.PhaseStart || payload == nil || !briefFromPlugin(args) {",
  "if true || phase != store.PhaseStart || payload == nil || !briefFromPlugin(args) {", "TestPC1d_")
