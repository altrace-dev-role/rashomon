# A host forget --host removes still read straight off the surviving
# declarations in three views that are not the sandbox section: the
# destinations lists, a family's host count, and a chain link's own host
# lists. Each kept its pre-filter line; these three mutants restore it.
m("destinations drop the per-declaration forgotten-host filter", "internal/report/destinations.go",
  "DeclaredHosts:         declaredHostsFiltered(run, forgotten),\n\t\tSSHHosts:              sshDeclaredFiltered(run, forgotten),",
  "DeclaredHosts:         declaredHosts(run),\n\t\tSSHHosts:              sshDeclared(run),",
  "TestSeam_APartlyFailedForgetStillSuppressesInDestinationsFamiliesAndChains")
m("families never exclude a forgotten host from the count", "internal/report/families.go",
  "\tif forgotten == nil {\n\t\tforgotten = func(string, int64) bool { return false }\n\t}",
  "\tforgotten = func(string, int64) bool { return false }",
  "TestSeam_APartlyFailedForgetStillSuppressesInDestinationsFamiliesAndChains")
m("a chain link carries a forgotten host in its own lists", "internal/report/chains.go",
  "\tfor _, h := range d.Hosts {\n\t\tif forgotten != nil && forgotten(h, d.RecordedAtMS) {\n\t\t\tcontinue\n\t\t}\n\t\tl.DeclaredHosts = append(l.DeclaredHosts, h)\n\t}",
  "\tfor _, h := range d.Hosts {\n\t\tl.DeclaredHosts = append(l.DeclaredHosts, h)\n\t}",
  "TestSeam_APartlyFailedForgetStillSuppressesInDestinationsFamiliesAndChains")

# forgotten only suppresses a declaration recorded AT OR BEFORE the latest
# forget of that name -- a stale leftover a failed removal left behind. A
# declaration recorded after is a fresh, legitimate re-use of the name and
# must still show up everywhere above: in destinations, in the family that
# named it, and in its own chain link.
m("a declaration recorded after the forget is suppressed like one from before it", "internal/store/gaps.go",
  "\t\tforgotMS, ok := times[s.HostDigest(host)]\n\t\treturn ok && declaredAtMS <= forgotMS",
  "\t\t_, ok := times[s.HostDigest(host)]\n\t\treturn ok",
  "TestTRU58b_ForgottenHostNeverReturnsToTheReport")
