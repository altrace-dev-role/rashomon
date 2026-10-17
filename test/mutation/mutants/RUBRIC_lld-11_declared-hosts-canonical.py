# declaredHosts feeds the baseline novelty walk (buildNovelty). A run stored
# before host.Canonical folded an IP literal to its one spelling holds the
# raw string, and reading it back without canonicalising made that host read
# as a second, unknown one on every later render of the same old run.
#
# Re-anchored when the forgotten-check reorder (RUBRIC lld-11b) landed: the
# canonicalise-and-shape block this mutant removes now has the forgotten
# check inlined after it, so the anchor covers through set[canon] = true.
m("RUBRIC lld-11 declaredHosts does not canonicalise its keys", "internal/report/destinations.go",
  "// declaration -- 167838211 where a forget named 10.1.2.3 --\n"
  "\t\t\t// escape the check and reappear in every later render of the\n"
  "\t\t\t// same old run.\n"
  "\t\t\tcanon, ok := host.Canonical(h)\n\t\t\tif !ok || !host.Shaped(canon) {\n\t\t\t\tcontinue\n\t\t\t}\n"
  "\t\t\tif forgotten != nil && forgotten(canon, d.RecordedAtMS) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tset[canon] = true\n",
  "// declaration -- 167838211 where a forget named 10.1.2.3 --\n"
  "\t\t\t// escape the check and reappear in every later render of the\n"
  "\t\t\t// same old run.\n"
  "\t\t\tif forgotten != nil && forgotten(h, d.RecordedAtMS) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tset[h] = true\n",
  "TestNovelty_LegacyIPLiteralDeclarationIsNotNovelAgain|TestNovelty_RerenderingAPreUpgradeRunIsNotNovel")
