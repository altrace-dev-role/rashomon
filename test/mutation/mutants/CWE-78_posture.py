# NOT A MUTANT, deliberately: "env exports the file's raw listen_addr instead
# of the rebuilt one". Under the grammar every accepted address IS its rebuilt
# form -- an exact host literal, ':' and plain digits -- so that break changes
# no output and would be reported as undetected when it is equivalent. The
# rebuild is defence in depth behind the parse; the parse mutants below are
# what hold the property, and they go red.
#
# The listener grammar (CWE-78). posture read only the HOST of listen_addr, so
# "127.0.0.1:1;cmd" passed and env printed the command into the operator's
# eval. Three ways to lose the fix: a portless address defaulted to a port, a
# port read up to its first non-digit (strconv-prefix style), and the parse
# not consulted at all.
m("CWE-78 a portless listen address falls back to a default port", "internal/posture/posture.go",
  "\tif i < 0 {\n\t\treturn Listen{}, false\n\t}",
  "\tif i < 0 {\n\t\taddr, i = addr+\":18080\", len(addr)\n\t}",
  "TestParseListen|TestRead_RefusesAListenAddress|TestH22_EnvRefusesAListenAddress")
m("CWE-78 a port is read up to its first non-digit, dropping the junk after it", "internal/posture/posture.go",
  "\t\tif c < '0' || c > '9' {\n\t\t\treturn 0, false\n\t\t}",
  "\t\tif c < '0' || c > '9' {\n\t\t\tbreak\n\t\t}",
  "TestParseListen|TestRead_RefusesAListenAddress|TestH22_EnvRefusesAListenAddress|TestH22_AListenAddressNeverReachesEval")
m("CWE-78 posture never consults the listener grammar", "internal/posture/posture.go",
  "\tlisten, ok := parseListen(v.File.ListenAddr)\n\tif !ok {",
  "\tlisten, ok := parseListen(v.File.ListenAddr)\n\tif false && !ok {",
  "TestRead_RefusesAListenAddress|TestH22_EnvRefusesAListenAddress|TestH22_AListenAddressNeverReachesEval|TestH27_LaunchesWithoutVariables")
