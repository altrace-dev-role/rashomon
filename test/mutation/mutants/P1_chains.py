# Part 1 -- the in-window counters. Both mutations are the two ways the
# distinction they exist to make can be lost silently: counting traffic that is
# not this session's, and counting rows rather than folded requests. Neither
# changes any existing number, so only a test written for them can catch either.
m("P1 inherited rows count toward this session's reached/failed",
  "internal/wire/wire.go",
  "\t\tif !inherited {\n\t\t\tif reached {", "\t\tif true {\n\t\t\tif reached {",
  "TestInWindow")
m("P1 the counters are derived separately from Unreached", "internal/wire/wire.go",
  "\t\t\td.Unreached = false\n\t\t\treached = true", "\t\t\td.Unreached = false",
  "TestInWindow")

# Part 1 -- the chain view. Each of these is a way the view keeps rendering
# while asserting something the store does not support.
m("P1 chains are keyed on the prompt id alone", "internal/report/chains.go",
  "\t\tk := key{transcript: d.TranscriptPath, prompt: *d.PromptID}",
  "\t\tk := key{prompt: *d.PromptID}", "TestChains")
m("P1 links are left in store order", "internal/report/chains.go",
  "\t\tsort.SliceStable(c.Links, func(i, j int) bool { return c.Links[i].Seq < c.Links[j].Seq })",
  "", "TestChains")
m("P1 an unattributable window still yields host verdicts", "internal/report/chains.go",
  "\tif !dests.WindowApplied {\n\t\treturn LinkUnknown\n\t}\n", "", "TestChains|TestH31")
m("P1 a denial reads as a missing execution record", "internal/report/chains.go",
  "\t\tif denied[id] {\n\t\t\treturn LinkOutcomeDenied, []string{}, 0\n\t\t}\n", "",
  "TestChains|TestH31")
# The aliasing one. It is the reason redactChains rebuilds three slices rather
# than copying the struct, and a shallow copy compiles and renders correctly --
# the damage is entirely to the ORIGINAL report the caller still holds.
# The notices union. CI found this one: the linked set is platform-dependent,
# so a host-only check is wrong in the stale direction on every platform.
# Pinned to windows rather than "drop the env", because dropping it is a
# MACHINE-DEPENDENT break: a darwin host links a superset of every platform, so
# removing the override changes nothing there and the mutation reads as
# undetected on the developer's machine while being detected in Linux CI. A
# mutation whose verdict depends on who runs it is not evidence. Windows is the
# one platform that lacks a module another links (google/uuid), so pinning to it
# collapses the union everywhere.
m("the notices union collapses to one platform", "test/acceptance/notices_test.go",
  "\t\tcmd.Env = append(os.Environ(), \"GOOS=\"+p.goos, \"GOARCH=\"+p.goarch)",
  "\t\tcmd.Env = append(os.Environ(), \"GOOS=windows\", \"GOARCH=\"+p.goarch)",
  "TestThirdPartyNotices")
m("the release matrix drops a shipped platform", "test/acceptance/notices_test.go",
  "\t{\"windows\", \"amd64\"}, {\"windows\", \"arm64\"},", "", "TestThirdPartyNotices")

m("P1 a forgotten host is dropped instead of named", "internal/report/chains.go",
  "\tif forgotten != nil && forgotten(h) {\n\t\treturn LinkForgotten\n\t}\n", "", "TestChains|TestH31")
m("P1 loopback reads as a finding", "internal/report/chains.go",
  "\tif loopbackHosts[h] {\n\t\treturn LinkLoopback\n\t}\n", "", "TestChains|TestH31")
m("P1 client plane is re-derived instead of read from the view",
  "internal/report/chains.go",
  "\tfor _, c := range dests.ClientPlane {\n\t\tif c == h {\n\t\t\treturn LinkClientPlane\n\t\t}\n\t}\n",
  "\tif clientPlaneHosts[h] {\n\t\treturn LinkClientPlane\n\t}\n", "TestChains")
m("P1 the structural states collapse under the window gate",
  "internal/report/chains.go",
  "\tif forgotten != nil && forgotten(h) {", "\tif !dests.WindowApplied {\n\t\treturn LinkUnknown\n\t}\n\tif forgotten != nil && forgotten(h) {",
  "TestChains")
m("P1 the later slice index wins over the higher seq", "internal/report/chains.go",
  "\t\t\treturn execSeq(recs[i]) < execSeq(recs[j])", "\t\t\treturn false", "TestChains")
m("P1 a record with no seq outranks one that has a position",
  "internal/report/chains.go", "\t\treturn -1", "\t\treturn 1<<62", "TestChains")
m("P1 the second execution record is not reported", "internal/report/chains.go",
  "\treturn all[len(all)-1], all, len(recs)", "\treturn all[len(all)-1], all[:1], 1", "TestChains")
m("P1 unattributed calls lose their ids", "internal/report/chains.go",
  "\t\t\tout.Unattributed = append(out.Unattributed,\n\t\t\t\tbuildLink(d, executed, denied, state, dests, forgotten))\n\t\t\tcontinue",
  "\t\t\tcontinue", "TestChains|TestH31")
m("P1 dropped declarations vanish", "internal/report/chains.go",
  "\tfor _, id := range run.Dropped() {", "\tfor _, id := range []string{} {", "TestChains")
m("P1 ssh hosts are joined into the observable list", "internal/report/chains.go",
  "\tl.SSHHosts = append(l.SSHHosts, d.SSHHosts...)", "", "TestChains|TestH31")
m("P1 redaction leaves ssh hosts in clear", "internal/report/redact.go",
  "\t\tfor _, h := range link.SSHHosts {\n\t\t\tl.SSHHosts = append(l.SSHHosts, redactHost(h, key))\n\t\t}",
  "\t\tl.SSHHosts = append(l.SSHHosts, link.SSHHosts...)", "TestRedact|TestH31")
m("P1 the chain count hides behind the flag too", "internal/report/text.go",
  '\tfmt.Fprintf(b, "  chains: %d\\n", len(c.Prompts))', "", "TestH31|TestChains")
m("P1 the chain flag is inverted", "cmd/rashomon/main.go",
  "\t\tif chain {\n\t\t\topts = append(opts, report.WithChain())",
  "\t\tif !chain {\n\t\t\topts = append(opts, report.WithChain())", "TestH31")
m("P1 redaction writes through to the unredacted report", "internal/report/redact.go",
  "\t\tl.Hosts = make([]LinkHost, 0, len(link.Hosts))\n\t\tfor _, h := range link.Hosts {\n\t\t\t// The state is carried through untouched: it is a verdict, not a\n\t\t\t// name, and it is the only thing left worth reading.\n\t\t\tl.Hosts = append(l.Hosts, LinkHost{Host: redactHost(h.Host, key), State: h.State})\n\t\t}",
  "\t\tfor k := range l.Hosts {\n\t\t\tl.Hosts[k].Host = redactHost(l.Hosts[k].Host, key)\n\t\t}",
  "TestRedactChains")

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
