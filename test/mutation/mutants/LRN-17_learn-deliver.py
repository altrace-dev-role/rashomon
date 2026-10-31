# LRN-17: approved lessons reach the agent with the prompt (LEARN-DELIVER1).
# recap pending reads active.json against its manifest and never writes it,
# gives one relevant lesson a prompt and each once a session, below the
# request's preflight note; a state that does not check gives one flag, under
# a key no lesson holds, worded for the signature or the size; approval runs
# delivery's own check and stamps its time. The kill row is the named test's.
A = "internal/learn/active.go"
V = "internal/learn/deliver.go"
D = "internal/learn/ledger.go"
R = "cmd/rashomon/recap_learn.go"
P = "cmd/rashomon/preflight.go"
C = "cmd/rashomon/learn.go"
F = "internal/steer/render/slot/fact.go"

# The read: verify, never write, a Save between reads not tamper, size apart.
m("LRN-17 the manifest not compared", A,
  "\tif !hmac.Equal(sum, []byte(signDigests(key, digests))) {\n",
  "\tif false && !hmac.Equal(sum, []byte(signDigests(key, digests))) {\n", "TestLRN17b_")
m("LRN-17 a mismatch saved on the prompt path", A,
  "\tif unchecked != \"\" {\n\t\treturn Active{Unchecked: unchecked}, nil\n",
  "\tif unchecked != \"\" {\n\t\t_ = (&Ledger{root: root, dir: dir, tallies: map[string]Tally{}}).Save()\n"
  "\t\treturn Active{Unchecked: unchecked}, nil\n", "TestLRN17b_")
m("LRN-17 a demoted row delivered", A,
  "\t\tif ls.Tier == TierProbation || ls.Tier == TierProven {\n",
  "\t\tif ls.Tier != TierProposed {\n", "TestLRN17d_")
m("LRN-17 the read not retried (a Save between reads is tamper)", A,
  "\tfor try := 0; try < 2 && unchecked == UncheckedSignature; try++ {\n",
  "\tfor try := 0; try < 1 && unchecked == UncheckedSignature; try++ {\n", "TestLRN17k_")
m("LRN-17 the size case reported as the signature case", A,
  "\t\tif over {\n\t\t\treturn nil, UncheckedSize, nil\n",
  "\t\tif over {\n\t\t\treturn nil, UncheckedSignature, nil\n", "TestLRN17l_")

# The call site: a lesson with a prompt that names nothing, and routed beside
# the preflight note so the note supersedes it.
m("LRN-17 a prompt naming nothing gives no lesson", P,
  "\tif !named && !proj.lessons.Any() {\n", "\tif !named {\n", "TestLRN17_")
m("LRN-17 the lesson dropped when the preflight has a note", P,
  "\tif f, ok := lessonFinding(proj.lessons, run.LastSeq(), lines, paths); ok {\n",
  "\tif f, ok := lessonFinding(proj.lessons, run.LastSeq(), lines, paths); ok && len(found) == 0 {\n", "TestLRN17m_")

# Which lesson, and once.
m("LRN-17 the delivered set ignored", R,
  "\tls, m, ok := learn.Pick(a, func(id string) bool { return given[id] }, paths)\n",
  "\tls, m, ok := learn.Pick(a, func(id string) bool { return false }, paths)\n", "TestLRN17c_")
m("LRN-17 the lesson finding carries no stop form", R,
  "Kind: \"local\", StopForm: steer.StopFormNone, AfterSeq: afterSeq}",
  "Kind: \"local\", AfterSeq: afterSeq}", "TestLRN17n_")
m("LRN-17 a superseded lesson counted as given", V,
  "spanText(b, sp[dkState]) != string(steer.LineStateDelivered)",
  "spanText(b, sp[dkState]) != string(steer.LineStateDelivered) && spanText(b, sp[dkState]) != string(steer.LineStateSuperseded)",
  "TestLRN17m_")
m("LRN-17 every lesson relevant at any count", V,
  "\tall := len(a.Lessons) <= pickAll\n", "\tall := true\n", "TestLRN17h_")
m("LRN-17 the flag keyed inside the id form", V,
  "const UncheckedKey = \"#unchecked\"\n", "const UncheckedKey = \"unchecked\"\n", "TestLRN17b_")

# Approval: delivery's check first, and its time.
m("LRN-17 approval without the render check", C,
  "\tif _, err := learn.Message(lessons[i]); err != nil {\n",
  "\tif _, err := learn.Message(lessons[i]); false && err != nil {\n", "TestLRN17g_")
m("LRN-17 the host screen dropped from Message", V,
  "\tif err := slot.RefuseHosts(ls.Text); err != nil {\n",
  "\tif err := slot.RefuseHosts(ls.Text); false && err != nil {\n", "TestLRN17g_")
m("LRN-17 the closed label list dropped (every dotted word refused)", F,
  "\t\tif slices.Contains(hostLabels, strings.ToLower(w[dot+1:])) {\n", "\t\tif true {\n", "TestLRN17i_")
m("LRN-17 the host screen reads whole white-space words only", F,
  "const hostSeps = \"/:=,;@()'+\"\n", "const hostSeps = \"\"\n", "TestLRN17i_")
m("LRN-17 the host screen refuses no IPv4 literal", F,
  "\t_, err := netip.ParseAddr(w)\n\treturn err == nil\n",
  "\ta, err := netip.ParseAddr(w)\n\treturn err == nil && !a.Is4()\n", "TestLRN17i_")
m("LRN-17 the host screen refuses no IPv6 literal", F,
  "\t_, err := netip.ParseAddr(w)\n\treturn err == nil\n",
  "\ta, err := netip.ParseAddr(w)\n\treturn err == nil && !a.Is6()\n", "TestLRN17i_")
m("LRN-17 the approval time not stamped", D,
  "\tl.file.Lessons[i].ApprovedUnixMS = now.UnixMilli()\n", "\t_ = now\n", "TestLRN17j_")

# The consent screen says what the build does: an approved lesson reaches the
# agent with a prompt, once a session.
m("LRN-17 the consent screen says no lesson reaches the agent", "internal/disclose/paragraphs.go",
  "An approved lesson reaches your agent with your prompt, once a session, below",
  "No lesson reaches your agent in this build; an approved one waits below", "TestLRN17k_")
