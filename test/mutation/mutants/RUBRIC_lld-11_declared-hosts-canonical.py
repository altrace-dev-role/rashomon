# declaredHosts feeds the baseline novelty walk (buildNovelty). A run stored
# before host.Canonical folded an IP literal to its one spelling holds the
# raw string, and reading it back without canonicalising made that host read
# as a second, unknown one on every later render of the same old run.
m("RUBRIC lld-11 declaredHosts does not canonicalise its keys", "internal/report/destinations.go",
  "\t\t\tcanon, ok := host.Canonical(h)\n\t\t\tif !ok || !host.Shaped(canon) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tset[canon] = true\n",
  "\t\t\tset[h] = true\n", "TestNovelty_LegacyIPLiteralDeclarationIsNotNovelAgain|TestNovelty_RerenderingAPreUpgradeRunIsNotNovel")
