# GB-1 (card GATE-BIND1, R47-R49): a declared gate binds a run of its runner,
# not of its argument list; a launcher row and match:exact keep the exact
# words; a bound run whose words are not the gate's reads "ran with other
# arguments and passed", within 80 columns.
N = "internal/shape/masked_name.go"
L = "internal/shape/masked.go"
D = "internal/verify/declared.go"
V = "internal/recap/value.go"

m("GB-1 the runner key includes the arguments", N,
  "\tcase name != \"\" && !slices.Contains(launcherRows, name):\n", "\tcase false && !slices.Contains(launcherRows, name):\n", "TestGB1a_|TestGB1e_")
m("GB-1 a launcher row keys by its name", L,
  "var launcherRows = []string{\"make\", ", "var launcherRows = []string{\"make-never\", ", "TestGB1b_|TestGB1e_")
# exact falling through to the runner is DG-1's row "exact is read as prefix",
# re-anchored here and judged by TestGB1b_ too.
m("GB-1 a run with other arguments reads passed", D,
  "\t\t\t\ts.LastRun.OtherArgs = true\n", "\t\t\t\ts.LastRun.OtherArgs = false\n", "TestGB1a_")
m("GB-1 the gate's own words read other arguments", D,
  "len(g.words) > 0 && !g.words[*r.Runner] {\n", "len(g.words) > 0 {\n", "TestGB1b_")
m("GB-1 a long id prints past 80 columns", V,
  "if short := sentence(v, 1, true); columns(short) < columns(s) {\n",
  "if short := sentence(v, 1, true); false && columns(short) < columns(s) {\n", "TestGB1c_")
# A run recorded with no runner_name (before call schema 8, an alpha.1 record)
# binds by the runner's exact words, as before GATE-BIND1.
m("GB-1 a run with no runner name never binds", D,
  "return ok && (g.runners[k] || g.words[*r.Runner]) ||", "return ok && g.runners[k] ||", "TestDG1c_|TestDG2_")
