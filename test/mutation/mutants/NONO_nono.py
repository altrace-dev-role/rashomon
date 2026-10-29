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
  "gather.WithNonoTrail(nonoTrail)", "gather.WithNonoTrail(\"\")", "TestH31_NonoTrail")

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
#
# Re-anchored (#44 follow-up item 2): WindowOpen used to be set in the struct
# literal, before the read could fail, so a missing trail and an all-torn one
# against the same open window disagreed on it. It is now set once the read
# has succeeded, next to obs.Observed = true.
m("NONO the reader never says the window has no end", "internal/nono/nono.go",
  "\tobs.WindowOpen = w.End.IsZero()\n", "\tobs.WindowOpen = false\n",
  "TestRead_AWindowWithNoEnd")
m("NONO window_open is set before the read can fail", "internal/nono/nono.go",
  "\tobs := Observation{Trail: path, Events: []Event{}}\n",
  "\tobs := Observation{Trail: path, Events: []Event{}, WindowOpen: w.End.IsZero()}\n",
  "TestRead_WindowOpenIsFalseWhenNotObserved")
m("NONO the sandbox line does not say the window has no end", "internal/report/text.go",
  "\t\tif n.WindowOpen {", "\t\tif false {",
  "TestNono_AWindowWithNoEnd|TestNono_ASessionWithNoEndRecord")
m("NONO a bounded window is said to have no end", "internal/report/text.go",
  "\t\tif n.WindowOpen {", "\t\tif true {",
  "TestNono_AWindowWithNoEnd")
# Re-anchored: NetworkEventsInTrail's addition made gofmt realign the struct
# literal, so the old anchor's column spacing matched nothing.
m("NONO an open window does not reach the report", "internal/report/nono.go",
  "\t\tWindowOpen:           obs.WindowOpen,\n", "",
  "TestNono_AWindowWithNoEnd|TestNono_ASessionWithNoEndRecord")

# Finding 5: a trail of only unreadable lines read as "nono wrote nothing".
#
# Re-anchored (#44 follow-up item 1): the count is now shown only when
# n.Reason == nono.NotObservedNoRecords, because that is the only reason the
# whole file was read -- on nono_audit_unreadable the scan stopped at the
# line that broke it, so Skipped there undercounts.
m("NONO a not-observed trail does not say how much it could not read", "internal/report/text.go",
  "\t\tif n.Reason == nono.NotObservedNoRecords && n.Skipped > 0 {\n\t\t\tskipped = ",
  "\t\t_ = nono.NotObservedNoRecords\n\t\tif false {\n\t\t\tskipped = ",
  "TestNono_AnUnreadableTrail")
m("NONO a read that stopped partway says how much it could not read", "internal/report/text.go",
  "\t\tif n.Reason == nono.NotObservedNoRecords && n.Skipped > 0 {",
  "\t\t_ = nono.NotObservedNoRecords\n\t\tif n.Skipped > 0 {",
  "TestNono_AReadThatStopsPartwayDoesNotClaimAnUndercount")
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
  "\treturn slices.Contains(clientPlaneHosts, h) && !(h == mcpProxyHost && mcpAttributed)", "\treturn slices.Contains(clientPlaneHosts, h)",
  "TestNono_TheMCPTransport|TestClientPlane_")
m("NONO the session's MCP calls never reach the sandbox section", "internal/report/assemble.go",
  "nonoConfigured, madeMCPCall(run), declaredHosts(run), forgotten)", "nonoConfigured, false, declaredHosts(run), forgotten)",
  "TestSeam_TheMCPTransportReachesTheSandboxSection")
m("NONO the client's log intake is not on the client-plane list", "internal/report/destinations.go",
  "\t\"http-intake.logs.us5.datadoghq.com\",\n", "",
  "TestNono_TheClientsOwnTraffic")
m("NONO the text never names the client's own traffic", "internal/report/text.go",
  "\t\tif len(n.ClientPlaneAllowed)+len(n.ClientPlaneDenied) > 0 {", "\t\tif false {",
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
  "\tname, ok := host.Canonical(raw)\n\tif !ok {\n\t\treturn nil, ErrNotAHostname\n\t}\n",
  "\tname, ok := raw, raw != \"\"\n\tif !ok {\n\t\treturn nil, ErrNotAHostname\n\t}\n",
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
m("NONO the session's named hosts never reach the sandbox section", "internal/report/assemble.go",
  "madeMCPCall(run), declaredHosts(run), forgotten)", "madeMCPCall(run), nil, forgotten)",
  "TestSeam_AClientPlaneHostAToolCallNamed")
# The client-traffic line says which hosts the sandbox allowed and which it
# refused (review of #53): one list of both hid the refused log upload.
m("NONO the client-traffic line does not say what the sandbox refused", "internal/report/text.go",
  "\t\t\tif len(n.ClientPlaneDenied) > 0 {\n\t\t\t\tsaid = append(said, \"refused \"", "\t\t\tif false {\n\t\t\t\tsaid = append(said, \"refused \"",
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
  "\tif err := s.recordForgottenHost(s.HostDigest(name), now); err != nil {\n\t\treturn nil, err\n\t}\n\tif unfolded, ok := host.CanonicalName(raw); ok && unfolded != name {\n\t\tif err := s.recordForgottenHost(s.HostDigest(unfolded), now); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t}\n\treturn s.forgetEach(func(dir string) (*Gap, error) { return s.forgetHostInRun(dir, name, now) })",
  "\tgaps, err := s.forgetEach(func(dir string) (*Gap, error) { return s.forgetHostInRun(dir, name, now) })\n\tif err != nil {\n\t\treturn gaps, err\n\t}\n\tif err := s.recordForgottenHost(s.HostDigest(name), now); err != nil {\n\t\treturn nil, err\n\t}\n\tif unfolded, ok := host.CanonicalName(raw); ok && unfolded != name {\n\t\tif err := s.recordForgottenHost(s.HostDigest(unfolded), now); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t}\n\treturn gaps, nil",
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
  "\t\t\tif loopback(h) || clientPlaneHost(h, mcpAttributed) {\n", "\t\t\tif loopback(h) || clientPlaneHosts[h] {\n",
  "TestClientPlane_EveryView")
m("NONO the session's MCP calls never reach the novelty candidates", "internal/report/destinations.go",
  "\twire, ssh := novelHostCandidates(declaredHostsFiltered(run, forgotten), sshDeclaredFiltered(run, forgotten), madeMCPCall(run))\n",
  "\twire, ssh := novelHostCandidates(declaredHostsFiltered(run, forgotten), sshDeclaredFiltered(run, forgotten), false)\n",
  "TestNovelty_TheMCPTransport")

# #53 follow-up, applied here: a trail holding only session_started and
# session_ended is Observed but may have written zero network decisions at
# all, which is a different fact from a filtered session that saw nothing.
# NetworkEventsInTrail is the whole-file count that tells the two apart.
m("NONO network records are never counted toward the whole-file total", "internal/nono/nono.go",
  "\t\t\tobs.NetworkEventsInTrail++\n", "",
  "TestRead_NetworkEventsInTrailCountsTheWholeFile")
m("NONO the whole-file network count never reaches the report", "internal/report/nono.go",
  "\t\tNetworkEventsInTrail: obs.NetworkEventsInTrail,\n", "",
  "TestNono_ANoNetworkEventsTrailSaysSoInsteadOfZero")
m("NONO a trail with no network events renders as a filtered, quiet session", "internal/report/text.go",
  "\tif n.NetworkEventsInTrail == 0 {", "\tif false {",
  "TestNono_ANoNetworkEventsTrailSaysSoInsteadOfZero|TestH31_ASilentTrailLeavesTheNamedHostInTheChain")

# NetworkEventsInTrail's own doc and commit history say
# it is a WHOLE-FILE count, in the window or out of it, whatever the target's
# shape -- but nothing pinned that against a mutant that moves the increment
# past the window and target checks, which still passes every pre-existing
# test here and in internal/report because every other fixture keeps its
# network records in-window with parseable targets.
m("NONO the whole-file network count only counts in-window parseable records",
  "internal/nono/nono.go",
  '\t\tcase "network":\n\t\t\tparsed++\n\t\t\tobs.NetworkEventsInTrail++\n\t\tdefault:\n\t\t\t// A type nono has and this reader has not learned. Counted,\n\t\t\t// because schema drift is a measured property of this dependency:\n\t\t\t// its documentation named event types the program does not emit.\n\t\t\tobs.Skipped++\n\t\t\tcontinue\n\t\t}\n\n\t\tn := head.Event\n\t\th, ok := host.Canonical(n.Target)\n\t\tif !ok {\n\t\t\t// The single most interesting record a sandbox can write -- a DENY\n\t\t\t// on a credential-bearing target -- was erased here without a\n\t\t\t// trace. Refusing to carry the string is right; refusing to carry\n\t\t\t// the count was not.\n\t\t\tobs.UnparseableTargets++\n\t\t\t// An unparseable target is dropped rather than guessed at. The\n\t\t\t// canonicaliser refuses anything carrying userinfo or a path, so\n\t\t\t// a credential in a target cannot reach a record from here.\n\t\t\tcontinue\n\t\t}\n\t\tat := time.UnixMilli(n.TimestampUnixMS)\n\t\tif at.Before(w.Start) || (!w.End.IsZero() && at.After(w.End)) {\n\t\t\tobs.Inherited++\n\t\t\tcontinue\n\t\t}\n', '\t\tcase "network":\n\t\t\tparsed++\n\t\tdefault:\n\t\t\t// A type nono has and this reader has not learned. Counted,\n\t\t\t// because schema drift is a measured property of this dependency:\n\t\t\t// its documentation named event types the program does not emit.\n\t\t\tobs.Skipped++\n\t\t\tcontinue\n\t\t}\n\n\t\tn := head.Event\n\t\th, ok := host.Canonical(n.Target)\n\t\tif !ok {\n\t\t\t// The single most interesting record a sandbox can write -- a DENY\n\t\t\t// on a credential-bearing target -- was erased here without a\n\t\t\t// trace. Refusing to carry the string is right; refusing to carry\n\t\t\t// the count was not.\n\t\t\tobs.UnparseableTargets++\n\t\t\t// An unparseable target is dropped rather than guessed at. The\n\t\t\t// canonicaliser refuses anything carrying userinfo or a path, so\n\t\t\t// a credential in a target cannot reach a record from here.\n\t\t\tcontinue\n\t\t}\n\t\tat := time.UnixMilli(n.TimestampUnixMS)\n\t\tif at.Before(w.Start) || (!w.End.IsZero() && at.After(w.End)) {\n\t\t\tobs.Inherited++\n\t\t\tcontinue\n\t\t}\n\t\tobs.NetworkEventsInTrail++\n',
  "TestRead_NetworkEventsInTrailCountsEventsOutsideTheWindowAndUnparseable")

# The no-network-events branch used to return right
# after printing its line, hiding the skipped/unparseable and sessions lines
# below on exactly the trails where schema drift or torn lines matter most.
m("NONO the no-network-events branch still returns before the shared lines",
  "internal/report/text.go",
  "\t\tfmt.Fprintf(b, \"  sandbox (nono): the trail holds no network events (%s; this is \"+\n\t\t\t\"not the same as a filtered session that saw nothing)\\n\", cause)\n\t} else {",
  "\t\tfmt.Fprintf(b, \"  sandbox (nono): the trail holds no network events (%s; this is \"+\n\t\t\t\"not the same as a filtered session that saw nothing)\\n\", cause)\n\t\treturn\n\t} else {",
  "TestNono_ZeroNetworkEventsStillReportsSkippedRecords")
