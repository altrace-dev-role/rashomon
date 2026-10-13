# The nono adapter. A verification review found the sweep had NO entry for any
# line of it, so the sweep certified nothing about a fourth evidence source --
# and separately found that five mutations to the plain-HTTP correction all
# survived the whole suite. These are the breaks with judging tests that exist.
m("NONO suppressTrail is removed, so forget --host is bypassed",
  "internal/report/nono.go", "\tobs = suppressTrail(obs, forgotten)", "\t_ = forgotten",
  "TestSeam_AForgotten")
m("NONO redaction skips the sandbox section", "internal/report/redact.go",
  "\t\ts.Nono = redactNono(sess.Nono, key)", "", "TestRedact_EveryDeclared")
# Simulates the defect the vocabulary exists for: somebody adds a host-bearing
# field and classifies nothing. Mutating the test's own collection loop was a
# no-op, because with a complete vocabulary there is nothing for it to collect.
m("NONO a new host field appears with no classification", "internal/report/nono.go",
  "\tSessions  int `json:\"sessions\"`",
  "\tSessions  int `json:\"sessions\"`\n\tNewHosts []string `json:\"new_hosts\"`",
  "TestRedact_TheHostBearingSet")
m("NONO a file of unlearned record types reads as a quiet sandbox",
  "internal/nono/nono.go", "\t\tswitch head.Type {", "\t\tparsed++\n\t\tswitch head.Type {",
  "TestRead_AFileOfUnlearned")
m("NONO the drop counters are discarded on the bail-out path",
  "internal/nono/nono.go", "\t\tobs.Reason = NotObservedNoRecords",
  "\t\tobs = Observation{Trail: path}\n\t\tobs.Reason = NotObservedNoRecords",
  "TestRead_AFileOfUnlearned")
m("NONO the trail is not read at all from the CLI", "cmd/rashomon/main.go",
  "report.WithNonoTrail(nonoTrail)", "report.WithNonoTrail(\"\")", "TestH31_NonoTrail")

# nono integration test run, finding 1: an unknown transport went uncounted on
# the default path, because the counter sat below the no-proxy-store guard.
m("NONO an unknown mode is not counted without a proxy store", "internal/report/nono.go",
  "\t\tif e.Mode != \"reverse\" && e.Mode != \"connect\" {\n\t\t\tn.UnknownModes++\n\t\t}\n",
  "",
  "TestNono_AnUnknownMode")
m("NONO the text never reports an unknown mode", "internal/report/text.go",
  "\tif n.UnknownModes > 0 {", "\tif false {",
  "TestNono_AnUnknownModeIsCountedWithout")

# Finding 2: `--nono-audit ""` read as no trail asked for, printing nothing.
m("NONO an empty --nono-audit is read as no trail asked for", "cmd/rashomon/main.go",
  "\t\t\tif i+1 >= len(args) || args[i+1] == \"\" {\n\t\t\t\treturn errors.New(\"--nono-audit needs a value\")",
  "\t\t\tif i+1 >= len(args) {\n\t\t\t\treturn errors.New(\"--nono-audit needs a value\")",
  "TestReport_RefusesAnEmptyNonoAudit")

# Finding 4: with no end record the window ran to the end of the trail, and
# the sandbox line presented the counts as bounded.
m("NONO the reader never says the window has no end", "internal/nono/nono.go",
  "WindowOpen: w.End.IsZero()}", "WindowOpen: false}",
  "TestRead_AWindowWithNoEnd")
m("NONO the sandbox line does not say the window has no end", "internal/report/text.go",
  "\tif n.WindowOpen {", "\tif false {",
  "TestNono_AWindowWithNoEnd|TestNono_ASessionWithNoEndRecord")
m("NONO a bounded window is said to have no end", "internal/report/text.go",
  "\tif n.WindowOpen {", "\tif true {",
  "TestNono_AWindowWithNoEnd")
# Re-anchored: the proxy-era fields left the literal and gofmt realigned it,
# so the old anchor matched nothing and the mutant went with them.
m("NONO an open window does not reach the report", "internal/report/nono.go",
  "\t\tWindowOpen:         obs.WindowOpen,\n", "",
  "TestNono_AWindowWithNoEnd|TestNono_ASessionWithNoEndRecord")

# Finding 5: a trail of only unreadable lines read as "nono wrote nothing".
m("NONO a not-observed trail does not say how much it could not read", "internal/report/text.go",
  "\t\tif n.Skipped > 0 {\n\t\t\tskipped = ", "\t\tif false {\n\t\t\tskipped = ",
  "TestNono_AnUnreadableTrail")
m("NONO the unreadable count on a not-observed trail ignores the singular", "internal/report/text.go",
  "\"; %d trail record%s could not be read\", n.Skipped, plural(n.Skipped))",
  "\"; %d trail records could not be read\", n.Skipped)",
  "TestNono_AnUnreadableTrail")

# The unknown-decision line said "1 sandbox event ... were counted".
m("NONO the unknown-decision line says were for one event", "internal/report/text.go",
  "\t\tif n.UnknownDecisions == 1 {\n\t\t\tverb = \"was\"", "\t\tif false {\n\t\t\tverb = \"was\"",
  "TestNono_TheUnknownDecisionLine")
m("NONO the unknown-decision line is never printed", "internal/report/text.go",
  "\tif n.UnknownDecisions > 0 {", "\tif false {",
  "TestNono_TheUnknownDecisionLine")

# The live nono 0.79 pass (2026-10-09). The client's own traffic was counted as
# the session's on the default path, and a host-scoped forget that removed
# nothing left the host in the sandbox section (#45).
m("NONO the client's own traffic is counted as the session's", "internal/report/nono.go",
  "\t\t\tif clientsOwn(h) {\n\t\t\t\tclient = append(client, h)\n\t\t\t\tcontinue\n\t\t\t}\n",
  "\t\t\tif false && clientsOwn(h) {\n\t\t\t\tclient = append(client, h)\n\t\t\t\tcontinue\n\t\t\t}\n",
  "TestNono_TheClientsOwnTraffic|TestNono_TheMCPTransport")
m("NONO the client's own traffic is dropped rather than named", "internal/report/nono.go",
  "\t\t\t\tclient = append(client, h)\n", "",
  "TestNono_TheClientsOwnTraffic|TestNono_TheMCPTransport")
m("NONO the MCP transport is the client's on an MCP session", "internal/report/destinations.go",
  "\treturn clientPlaneHosts[h] && !(h == mcpProxyHost && mcpAttributed)", "\treturn clientPlaneHosts[h]",
  "TestNono_TheMCPTransport|TestClientPlane_")
m("NONO the session's MCP calls never reach the sandbox section", "internal/report/report.go",
  "cfg.nonoTrail != \"\", madeMCPCall(run), declaredHosts(run), forgotten)", "cfg.nonoTrail != \"\", false, declaredHosts(run), forgotten)",
  "TestSeam_TheMCPTransportReachesTheSandboxSection")
m("NONO the client's log intake is not on the client-plane list", "internal/report/destinations.go",
  "\t\"http-intake.logs.us5.datadoghq.com\": true,\n", "",
  "TestNono_TheClientsOwnTraffic")
m("NONO the text never names the client's own traffic", "internal/report/text.go",
  "\tif len(n.ClientPlaneAllowed)+len(n.ClientPlaneDenied) > 0 {", "\tif false {",
  "TestNono_TheClientsOwnTraffic")
m("NONO --redact passes the client's own allowed hosts through", "internal/report/redact.go",
  "\tout.ClientPlaneAllowed = redactList(n.ClientPlaneAllowed, key)\n", "",
  "TestRedact_")
m("NONO --redact passes the client's own refused hosts through", "internal/report/redact.go",
  "\tout.ClientPlaneDenied = redactList(n.ClientPlaneDenied, key)\n", "",
  "TestRedact_")
m("NONO a forget that removed nothing writes no store-level record", "internal/store/gaps.go",
  "\tf, err := os.OpenFile(filepath.Join(s.root, FileForgottenHosts), appendFlags, fileMode)",
  "\tf, err := os.OpenFile(filepath.Join(s.root, FileForgottenHosts+\".off\"), appendFlags, fileMode)",
  "TestForgetHost_|TestSeam_AHostOnlyTheSandboxSaw")
m("NONO the predicate does not read the store-level records", "internal/store/gaps.go",
  "\t\tif json.Unmarshal(line, &r) == nil && r.Type == TypeForgottenHost {",
  "\t\tif false && json.Unmarshal(line, &r) == nil {",
  "TestForgetHost_AHost|TestSeam_AHostOnlyTheSandboxSaw")
m("NONO the store-level record holds the hostname", "internal/store/gaps.go",
  "\tif err := s.recordForgottenHost(s.HostDigest(name), now); err != nil {",
  "\tif err := s.recordForgottenHost(s.HostDigest(name)+name, now); err != nil {",
  "TestForgetHost_TheStoreLevelRecordHoldsNoName")
m("NONO forget --host does not say a no-op forget still holds", "cmd/rashomon/main.go",
  "\tif total == 0 {\n\t\tfmt.Fprint(stdout, \"; later reports", "\tif false {\n\t\tfmt.Fprint(stdout, \"; later reports",
  "TestH25_")

# forget --host canonicalises its argument (review of #53): every host rashomon
# holds went through host.Canonical, so a value taken as typed -- GitHub.com,
# github.com:443, https://github.com/ -- forgot nothing while the line said
# later reports would leave it out.
m("NONO the store forgets the host as typed", "internal/store/gaps.go",
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn nil, ErrNotAHostname\n\t}\n\truns, err := s.Runs()\n",
  "\tname, ok := raw, raw != \"\"\n\tif !ok {\n\t\treturn nil, ErrNotAHostname\n\t}\n\truns, err := s.Runs()\n",
  "TestForgetHost_AnySpelling|TestSeam_AnySpelling")
m("NONO the store forgets a value that is not a hostname", "internal/store/gaps.go",
  "\tif !ok {\n\t\treturn nil, ErrNotAHostname\n\t}\n", "\tif !ok && false {\n\t\treturn nil, ErrNotAHostname\n\t}\n",
  "TestForgetHost_WhatIsNot")
# Re-anchored when the rubric's lld-4 fix met this one: forget --host takes the
# value as typed and canonicalises it once, in forgetHost, which still refuses
# a non-host before anything is opened; the store and the baselines
# canonicalise again for their library callers, so the store's own refusal
# would hide a dropped command-line check from the H25 test alone.
m("NONO forget --host passes the value on as typed", "cmd/rashomon/main.go",
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn errors.New(\"--host is not a hostname",
  "\t_, ok := host.Canonical(raw)\n\tname := raw\n\tif !ok {\n\t\treturn errors.New(\"--host is not a hostname",
  "TestH25_ForgetHostTakesAnySpelling")
m("NONO forget --host accepts what is not a hostname", "cmd/rashomon/main.go",
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn errors.New(\"--host is not a hostname",
  "\tname, ok := host.Canonical(raw)\n\tif false && !ok {\n\t\treturn errors.New(\"--host is not a hostname",
  "TestH25_ForgetHostRefusesWhatIs|TestRubricLLD4_ForgetCommandAsksInTheRecordedSpelling")
# A client-plane host one of the session's own tool calls named is the
# agent's attempt (review of #53): filed as the client's, a refused curl to the
# Datadog log intake left "refused by the sandbox".
m("NONO a client-plane host a tool call named is filed as the client's", "internal/report/nono.go",
  "return clientPlane(h) && !declared[h] }", "return clientPlane(h) }",
  "TestSeam_AClientPlaneHostAToolCallNamed")
m("NONO the session's named hosts never reach the sandbox section", "internal/report/report.go",
  "madeMCPCall(run), declaredHosts(run), forgotten)", "madeMCPCall(run), nil, forgotten)",
  "TestSeam_AClientPlaneHostAToolCallNamed")
# The client-traffic line says which hosts the sandbox allowed and which it
# refused (review of #53): one list of both hid the refused log upload.
m("NONO the client-traffic line does not say what the sandbox refused", "internal/report/text.go",
  "\t\tif len(n.ClientPlaneDenied) > 0 {\n\t\t\tsaid = append(said, \"refused \"", "\t\tif false {\n\t\t\tsaid = append(said, \"refused \"",
  "TestSeam_TheClientsOwnTrafficSaysWhat|TestNono_TheClientsOwn")
m("NONO the client's refused hosts are listed as allowed", "internal/report/nono.go",
  "\tn.Denied, n.ClientPlaneDenied = split(obs.Denied())\n", "\tn.Denied, _ = split(obs.Denied())\n\t_, n.ClientPlaneAllowed = split(append(obs.Hosts(), obs.Denied()...))\n",
  "TestSeam_TheClientsOwnTrafficSaysWhat|TestNono_TheClientsOwn")
# The client's hosts keep the trail reader's order, with no map between: the
# list used to come out of one, and only a sort a two-host test rarely caught
# made it deterministic. The order now has one source, and this pins it.
m("NONO the trail's host lists come out unsorted", "internal/nono/nono.go",
  "\t\tout = append(out, e.Host)\n\t}\n\tsort.Strings(out)\n\treturn out\n}", "\t\tout = append(out, e.Host)\n\t}\n\treturn out\n}",
  "TestSeam_TheClientsOwnTrafficIsListedInOrder")

# The store-level forget record is a record file like the others: closed key
# set and the store's mode (review of #53). A field beside the digest, which
# could carry the name in any encoding, passed the canary search it had.
m("NONO the store-level forget record carries a field beyond its digest", "internal/store/record.go",
  "\tHostDigest    string `json:\"host_digest\"`\n}", "\tHostDigest    string `json:\"host_digest\"`\n\tName          string `json:\"name\"`\n}",
  "TestH25_TheStoreLevelForgetRecordIsClosed")
# The published store contract knows the record (review of #53): without a
# definition every line of forgotten-hosts.ndjson failed the schema.
m("the store schema has no forgotten_host definition", "docs/store-schema.json",
  '    "forgotten_host": {\n      "title": "forgotten_host",', '    "forgotten_host_REMOVED": {\n      "title": "forgotten_host",',
  "TestStoreSchema")
m("the store schema's forgotten_host declares a key no record carries", "docs/store-schema.json",
  '        "host_digest": {\n          "description": "An HMAC under the per-install key of the canonical host name',
  '        "name": {\n          "type": "string"\n        },\n        "host_digest": {\n          "description": "An HMAC under the per-install key of the canonical host name',
  "TestStoreSchema")

# A repeat forget of one host adds no line to a file no size cap covers
# (review of #53).
m("NONO a repeat forget appends a second store-level record", "internal/store/gaps.go",
  "\t\tif json.Unmarshal(l, &r) == nil && r.Type == TypeForgottenHost && r.HostDigest == digest {",
  "\t\tif false && json.Unmarshal(l, &r) == nil {",
  "TestForgetHost_ASecondForget")

# The store-level record lands before any run is touched, and neither of its
# error paths is swallowed (review of #53): each of these survived every
# related test.
m("NONO the store-level forget record lands after the runs", "internal/store/gaps.go",
  "\tif err := s.recordForgottenHost(s.HostDigest(name), now); err != nil {\n\t\treturn nil, err\n\t}\n\tvar gaps []Gap\n\tfor _, run := range runs {\n\t\tg, err := s.forgetHostInRun(filepath.Join(s.root, dirRuns, run), name, now)\n\t\tif err != nil {\n\t\t\treturn gaps, err\n\t\t}\n\t\tif g != nil {\n\t\t\tgaps = append(gaps, *g)\n\t\t}\n\t}\n\treturn gaps, nil",
  "\tvar gaps []Gap\n\tfor _, run := range runs {\n\t\tg, err := s.forgetHostInRun(filepath.Join(s.root, dirRuns, run), name, now)\n\t\tif err != nil {\n\t\t\treturn gaps, err\n\t\t}\n\t\tif g != nil {\n\t\t\tgaps = append(gaps, *g)\n\t\t}\n\t}\n\tif err := s.recordForgottenHost(s.HostDigest(name), now); err != nil {\n\t\treturn nil, err\n\t}\n\treturn gaps, nil",
  "TestForgetHost_AForgetThatFails")
m("NONO an unreadable store-level forget file reads as nothing forgotten", "internal/store/gaps.go",
  "\ttimes, err := s.forgottenDigestTimes()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tif len(times) == 0 {\n\t\t// A predicate",
  "\ttimes, err := s.forgottenDigestTimes()\n\t_ = err\n\tif len(times) == 0 {\n\t\t// A predicate",
  "TestForgetHost_AnUn")
m("NONO a forget whose store-level record failed reports success", "internal/store/gaps.go",
  "\tif err := s.recordForgottenHost(s.HostDigest(name), now); err != nil {\n\t\treturn nil, err\n\t}\n",
  "\t_ = s.recordForgottenHost(s.HostDigest(name), now)\n",
  "TestForgetHost_AnUn")

# The client-plane rule has one spelling, clientPlaneHost, and every view
# that asks it of this session calls it (review of #53): the novelty
# candidates used to restate it. Re-anchored when the proxy era left: the
# candidates are the declared hosts, a list of names. Re-anchored again when
# loopback came to be decided on the parsed address (host.Loopback).
m("NONO the novelty candidates drop the MCP transport on an MCP session", "internal/report/destinations.go",
  "\t\tif host.Loopback(h) || clientPlaneHost(h, mcpAttributed) {", "\t\tif host.Loopback(h) || clientPlaneHosts[h] {",
  "TestClientPlane_EveryView")
m("NONO the session's MCP calls never reach the novelty candidates", "internal/report/destinations.go",
  "madeMCPCall(run)))", "false))",
  "TestNovelty_TheMCPTransport")
