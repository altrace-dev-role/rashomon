# Part 1 -- the chain view. Each of these is a way the view keeps rendering
# while asserting something the store does not support.
m("P1 chains are keyed on the prompt id alone", "internal/report/chains.go",
  "\t\tk := key{transcript: d.TranscriptPath, prompt: *d.PromptID}",
  "\t\tk := key{prompt: *d.PromptID}", "TestChains")
m("P1 links are left in store order", "internal/report/chains.go",
  "\t\tsort.SliceStable(c.Links, func(i, j int) bool { return c.Links[i].Seq < c.Links[j].Seq })",
  "", "TestChains")
m("P1 a denial reads as a missing execution record", "internal/report/chains.go",
  "\t\tif denied[id] {\n\t\t\treturn LinkOutcomeDenied, []string{}, 0\n\t\t}\n", "",
  "TestChains|TestH31")
# Re-anchored on buildLink's new signature, which took the anchor with it.
m("P1 unattributed calls lose their ids", "internal/report/chains.go",
  "\t\t\tout.Unattributed = append(out.Unattributed,\n\t\t\t\tbuildLink(d, executed, denied, forgotten))\n\t\t\tcontinue",
  "\t\t\tcontinue", "TestChains|TestH31")
# The aliasing one. It is the reason redactChains rebuilds three slices rather
# than copying the struct, and a shallow copy compiles and renders correctly --
# the damage is entirely to the ORIGINAL report the caller still holds.
# Re-anchored on the list of names a link now carries.
m("P1 redaction writes through to the unredacted report", "internal/report/redact.go",
  "\t\tl.DeclaredHosts = redactList(link.DeclaredHosts, key)",
  "\t\tfor k := range l.DeclaredHosts {\n\t\t\tl.DeclaredHosts[k] = redactHost(l.DeclaredHosts[k], key)\n\t\t}",
  "TestRedactChains")
# The notices union. CI found this one: the linked set is platform-dependent,
# so a host-only check is wrong in the stale direction on every platform.
# Pinned to windows rather than "drop the env", because a verdict must not
# depend on which machine runs the sweep. Re-anchored on the listing every
# whole-binary check shares. With no third-party module linked, collapsing
# every listing to windows changes no module set, and this went undetected
# when the SQLite driver left; it is caught now because every line of a
# listing names the target it was listed for.
m("the notices union collapses to one platform", "test/acceptance/release_targets_test.go",
  "\t\tcmd.Env = releaseListEnv(target, m.env)",
  '\t\tcmd.Env = releaseListEnv(releaseTarget{goos: "windows", goarch: target.goarch}, m.env)',
  "TestThirdPartyNotices")
# The matrix is read from .goreleaser.yml now rather than kept beside it, so a
# shipped platform is dropped by the reader, not by a hand-written list.
m("the release matrix drops a shipped platform", "test/acceptance/release_targets_test.go",
  "\t\t\tlists[key] = append(lists[key], item)", "\t\t\tlists[key] = []string{item}",
  "TestTRU40_ReleaseTargetsAreReadFromGoreleaser")

m("P1 the later slice index wins over the higher seq", "internal/report/chains.go",
  "\t\t\treturn execSeq(recs[i]) < execSeq(recs[j])", "\t\t\treturn false", "TestChains")
m("P1 a record with no seq outranks one that has a position",
  "internal/report/chains.go", "\t\treturn -1", "\t\treturn 1<<62", "TestChains")
m("P1 the second execution record is not reported", "internal/report/chains.go",
  "\treturn all[len(all)-1], all, len(recs)", "\treturn all[len(all)-1], all[:1], 1", "TestChains")
m("P1 dropped declarations vanish", "internal/report/chains.go",
  "\tfor _, id := range run.Dropped() {", "\tfor _, id := range []string{} {", "TestChains")
m("P1 ssh hosts are joined into the observable list", "internal/report/chains.go",
  "\tfor _, h := range d.SSHHosts {\n\t\tif forgotten != nil && forgotten(h, d.RecordedAtMS) {\n\t\t\tcontinue\n\t\t}\n\t\tl.SSHHosts = append(l.SSHHosts, h)\n\t}",
  "",
  "TestChains|TestH31")
m("P1 redaction leaves ssh hosts in clear", "internal/report/redact.go",
  "\t\tfor _, h := range link.SSHHosts {\n\t\t\tl.SSHHosts = append(l.SSHHosts, redactHost(h, key))\n\t\t}",
  "\t\tl.SSHHosts = append(l.SSHHosts, link.SSHHosts...)", "TestRedact|TestH31")
m("P1 the chain count hides behind the flag too", "internal/report/text.go",
  '\tfmt.Fprintf(b, "  chains: %d\\n", len(c.Prompts))', "", "TestH31|TestChains")
m("P1 the chain flag is inverted", "cmd/rashomon/main.go",
  "\t\tif chain {\n\t\t\topts = append(opts, report.WithChain())",
  "\t\tif !chain {\n\t\t\topts = append(opts, report.WithChain())", "TestH31")

# Re-anchored after the schema-3 bump reformatted the file. The mutation is
# unchanged: remove a declared key and confirm the allowlist test notices.
m("the store schema drops a record's key", "docs/store-schema.json",
  '        "agent_type": {\n', '        "agent_type_REMOVED": {\n', "TestStoreSchema")
# Anchored on schema 2, where tool_name is no longer the last property of the
# declaration block. The previous anchor assumed it was and silently stopped
# matching when hosts/ssh_hosts were added -- reported as ANCHOR MISSING, which
# lands in the same bucket as a real gap.
m("the store schema declares a key no record carries", "docs/store-schema.json",
  '        },\n        "shape": {\n', '        },\n        "tool_response": {\n          "type": "string"\n        },\n        "shape": {\n',
  "TestStoreSchema")
m("the store schema's coverage reasons are a subset of the code's", "docs/store-schema.json",
  "            \"probe_unresolved\"\n", "", "TestStoreSchema")
