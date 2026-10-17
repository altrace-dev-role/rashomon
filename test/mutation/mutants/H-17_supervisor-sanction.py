# H-17 and TRU-40: os/exec is sanctioned for internal/supervise alone, the
# package that starts the binary's own child for each hook entry. Lose the
# entry and the supervisor's import is a violation. (An import of os/exec
# anywhere else is TRU-40's mutant, the report importing it.)
m("H-17 the supervisor's os/exec import loses its sanction", "test/acceptance/h17_no_egress_test.go",
  '\t"os/exec": {rashomonModule + "/internal/supervise"},\n', "", "TestH17_NoPackageOfOursReachesForANetworkImport")
