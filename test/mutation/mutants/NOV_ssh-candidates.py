# A session that names an ssh host for the first time in its project must be
# told so, the same way an https one would be. Excluding ssh from the novelty
# candidates was a proxy-era carve-out: the proxy could not see ssh traffic at
# all, and nothing here still observes anything, so there is no reason left
# to leave it out.
m("destinations feeds only https hosts to the novelty baseline", "internal/report/destinations.go",
  "novelHostCandidates(append(append([]string{}, d.DeclaredHosts...), d.SSHHosts...), madeMCPCall(run)))",
  "novelHostCandidates(d.DeclaredHosts, madeMCPCall(run)))",
  "TestNovelty_SSHHostsAreCandidatesToo")
