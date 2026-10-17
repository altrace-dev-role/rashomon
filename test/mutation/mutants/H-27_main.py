m("H-27 run reports the newest session whether or not the command produced it", "cmd/rashomon/main.go",
  "\tif newest == \"\" || writtenAt.Before(since) {", "\tif _ = writtenAt; newest == \"\" {",
  "TestH27_ReportsNothingWhenTheCommandRecordedNothing")

# run's automatic report under a refused posture. posture.Read fills v.File
# before it decides, so without the gate a stale, crashed, enforcing or
# malformed status file chooses the database the report reads as the wire.
m("H-27 a refused posture's report reads the store it names", "cmd/rashomon/main.go",
  "\tif v.Export {\n\t\t// The proxy told us where it writes",
  "\tif true {\n\t\t// The proxy told us where it writes",
  "TestH27_ARefusedPostureReadsNoProxyStore")
# And under an ACCEPTED posture that named no causal_db, run falls back to the
# default path. Deleting the fallback left the whole suite green in loop 1:
# the accepted row named its store, so nothing read the default.
m("H-27 run's accepted report drops the default-store fallback", "cmd/rashomon/main.go",
  "\t\tproxyStore = v.File.CausalDB\n\t\tif proxyStore == \"\" {\n\t\t\tproxyStore = defaultProxyStore()\n\t\t}\n",
  "\t\tproxyStore = v.File.CausalDB\n",
  "TestH27_ARefusedPostureReadsNoProxyStore")
m("report falls back to the proxy's default store", "cmd/rashomon/main.go",
  "\trep, key, err := reportOrEmpty(sessionID, proxyStore, nonoTrail, time.Now())",
  "\tif proxyStore == \"\" {\n\t\tproxyStore = defaultProxyStore()\n\t}\n"
  "\trep, key, err := reportOrEmpty(sessionID, proxyStore, nonoTrail, time.Now())",
  "TestReport_ReadsAProxyStoreOnlyWhenNamed")
# The collapse. With no store named the proxy block is one line; forcing the
# full block back is what a reader without the proxy used to see.
m("report text renders the whole proxy block with no store named", "internal/report/text.go",
  "\tnamed := cfg.proxyStore != \"\"", "\tnamed := true",
  "TestReport_ReadsAProxyStoreOnlyWhenNamed|TestText_TheProxyBlock")
# The chain listing's share of the collapse: a state beside each host, the
# legend explaining it, "(not observable)" beside an ssh host -- and the seam
# that carries `named` into the listing at all.
m("report --chain prints a host state with no store named", "internal/report/text.go",
  "\t\tif !named {\n\t\t\tparts = append(parts, h.Host)",
  "\t\tif false {\n\t\t\tparts = append(parts, h.Host)",
  "TestText_TheCollapseReachesTheChainListing")
m("report --chain prints the host-state legend with no store named", "internal/report/text.go",
  "\tif len(c.Prompts) > 0 && named {", "\tif len(c.Prompts) > 0 {",
  "TestText_TheCollapseReachesTheChainListing|TestH31_ChainsGroupCallsUnderTheirPrompt|TestReport_ReadsAProxyStoreOnlyWhenNamed")
m("report --chain calls an ssh host not observable with no store named", "internal/report/text.go",
  "\tif !named {\n\t\treturn \"  ssh: \" + strings.Join(hosts, \", \")\n\t}",
  "",
  "TestText_TheCollapseReachesTheChainListing|TestH31_SSHHostsAreCarriedApartEndToEnd")
m("report --chain is never told whether a store was named", "internal/report/text.go",
  "\twriteChains(b, sess.Chains, cfg.chain, named)", "\twriteChains(b, sess.Chains, cfg.chain, true)",
  "TestText_TheCollapseReachesTheChainListing|TestH31_|TestReport_ReadsAProxyStoreOnlyWhenNamed")
# forget --host prints the baseline count and the proxy sentence only where a
# baseline directory shows a proxy store was read. Both directions.
m("forget --host speaks of a proxy on a machine that never read one", "cmd/rashomon/main.go",
  "\tif !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "\tif false && !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "TestH25_ForgetHostSaysNothingOfAProxyNeverRead")
m("forget --host drops the proxy sentence after a store was read", "cmd/rashomon/main.go",
  "\tif !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "\tif true || !hadBaselines {\n\t\tfmt.Fprintln(stdout)\n\t\treturn nil\n\t}",
  "TestH25_ForgottenHostStaysSuppressedInTheReport")
m("forget --host says \"1 runs\"", "cmd/rashomon/main.go",
  "\tif n == 1 {\n\t\treturn \"1 \" + noun\n\t}", "",
  "TestH25_ForgetHostSaysNothingOfAProxyNeverRead")
# Review of loop 2. Each of these is a way the collapse or the posture reasons
# regress without any line above noticing.
m("report --chain names hosts bare with no legend saying nothing observed them", "internal/report/text.go",
  "\t} else if !named && listsAHost(c) {", "\t} else if false {",
  "TestText_TheCollapseReachesTheChainListing|TestReport_ReadsAProxyStoreOnlyWhenNamed")
m("report text collapses an observed store when the render option was forgotten", "internal/report/text.go",
  "\tnamed := cfg.proxyStore != \"\" || sess.Destinations.Observed", "\tnamed := cfg.proxyStore != \"\"",
  "TestText_AnObservedStoreRendersInFullWithoutTheOption")
m("report takes --proxy-store \"\" as no store named", "cmd/rashomon/main.go",
  "\t\t\tif args[i+1] == \"\" {\n\t\t\t\treturn errors.New(\"--proxy-store needs a non-empty path\")\n\t\t\t}\n", "",
  "TestReport_RefusesAnEmptyProxyStore")
