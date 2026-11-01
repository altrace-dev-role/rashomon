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
S = "internal/steer/render/slot/slot.go"

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
m("LRN-17 the lesson dropped when the preflight has a note", R,
  "\tif f, ok := lessonFinding(a, afterSeq, given, paths); ok && !rode {\n",
  "\tif f, ok := lessonFinding(a, afterSeq, given, paths); ok && !rode && len(found) == 0 {\n", "TestLRN17m_")

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
m("LRN-17 a lesson with no approval time delivered", V,
  "(ls.Tier == TierProven) != proven || ls.ApprovedUnixMS == 0 || delivered(ls.ID)",
  "(ls.Tier == TierProven) != proven || delivered(ls.ID)", "TestLRN17h_")
m("LRN-17 every lesson relevant at any count", V,
  "\tall := len(a.Lessons) <= pickAll\n", "\tall := true\n", "TestLRN17h_")
m("LRN-17 the flag keyed inside the id form", V,
  "const UncheckedKey = \"#unchecked\"\n", "const UncheckedKey = \"unchecked\"\n", "TestLRN17b_")

# Approval: delivery's check first, and its time.
m("LRN-17 approval without the render check", C,
  "\tif _, err := learn.Message(ls); err != nil {\n",
  "\tif _, err := learn.Message(ls); false && err != nil {\n", "TestLRN17g_")
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
m("LRN-17 the lesson id not host-screened", S,
  "\tif err := RefuseHosts(string(l)); err != nil {\n", "\tif err := RefuseHosts(string(l)); false && err != nil {\n", "TestLRN17i_")
m("LRN-17 the lesson id not URL-scheme screened", S,
  "\tif urlSchemeRE.get().MatchString(string(l)) {\n", "\tif false && urlSchemeRE.get().MatchString(string(l)) {\n", "TestLRN17i_")
m("LRN-17 the lesson id not banned-word screened", S,
  "\tif w, ok := firstWordOf(string(l), approvedBanned); ok {\n", "\tif w, ok := firstWordOf(string(l), approvedBanned); false && ok {\n", "TestLRN17i_")
m("LRN-17 import accepts a host-shaped lesson id", D,
  "\t\tif idForm(ls.ID) && slot.RefuseHosts(ls.ID) != nil {\n", "\t\tif false && idForm(ls.ID) && slot.RefuseHosts(ls.ID) != nil {\n", "TestLRN17i_")
m("LRN-17 the approval time not stamped", D,
  "\tl.file.Lessons[i].ApprovedUnixMS = now.UnixMilli()\n", "\t_ = now\n", "TestLRN17j_")

# The consent screen says what the build does: an approved lesson reaches the
# agent with a prompt, once a session.
m("LRN-17 the consent screen claims nothing unapproved is sent", "internal/disclose/paragraphs.go",
  "stops one the same way. A write to the lesson files that this program did not make is detected, and no lesson is sent, unless the writer can also read the store key: the same limit as `rashomon spec trust`.",
  "stops one the same way; nothing you have not approved is sent.", "TestLRN17k_")
m("LRN-17 state.sum's row drops the key limit", "internal/disclose/learn.go",
  "is detected and sends every lesson back to proposed, unless the writer can also read the store key (the same limit as spec trust)",
  "sends every lesson back to proposed", "TestLRN17k_")
m("LRN-17 the render says approved at a terminal", "internal/steer/render/render.go",
  "approved for this project: {text}.", "approved for this project at a terminal: {text}.", "TestLRN17h_")
RIDE = ("An approved lesson is sent with a prompt, at most one per prompt and each lesson once a session: "
        "as the last line of that prompt's preflight note when your prompt names a path the lesson names, "
        "and otherwise with the next prompt that carries no preflight note.")
m("LRN-17 the consent screen says no lesson reaches the agent", "internal/disclose/paragraphs.go",
  RIDE, "No lesson reaches your agent in this build; an approved one waits below the note.", "TestLRN17k_")
m("LRN-17 the consent screen says a lesson waits behind every preflight note", "internal/disclose/paragraphs.go",
  RIDE, "An approved lesson is sent with a prompt that carries no preflight note, at most one per prompt and each "
  "lesson once a session; it waits while a preflight note is sent.", "TestLRN17k_")
m("LRN-17 the consent screen drops the steering clamp", "internal/disclose/paragraphs.go",
  " If steering is off or you have not agreed to steering notes, it is shown to you and not sent.", "", "TestLRN17k_")
m("LRN-17 the import line says a lesson reaches the agent with every prompt", C,
  "\"an approved lesson is sent with a prompt, at most one per prompt and each lesson once a session: \"+\n"
  "\t\t\"as the last line of that prompt's preflight note when the prompt names a path the lesson names, \"+\n"
  "\t\t\"and otherwise with the next prompt that carries no preflight note; ",
  "\"an approved lesson reaches the agent with your prompt; ", "TestLRN17k_")

# LEARN-DX1 (#156 D2, D3): a stamp-0 approval approves again and the list
# says so; `learn withdraw ID` is typed at the terminal and clears the stamp.
LG = "internal/learn/ledger.go"
m("LRN-17 a stamp-0 approval cannot be approved again", LG,
  "\treturn ls.Tier == TierProposed || ls.ApprovedUnixMS == 0\n",
  "\treturn ls.Tier == TierProposed\n", "TestLRN17s_")
m("LRN-17 the list says nothing of a stamp-0 approval", C,
  "\tif ls.AwaitsApproval() {\n", "\tif false {\n", "TestLRN17s_")
# LEARN-DX1 F1: a text the check refuses is not offered approval again.
m("LRN-17 the list offers approval for a text that cannot arrive", C,
  "\t\treturn fmt.Sprintf(\"    cannot reach the agent: %v\\n\", refused)\n",
  "\t\tif ls.AwaitsApproval() {\n"
  "\t\t\treturn fmt.Sprintf(\"    cannot reach the agent: %v\\n\", refused) + "
  "fmt.Sprintf(\"    approved before this build; approve again to send it: `rashomon learn approve %s`\\n\", ls.ID)\n"
  "\t\t}\n\t\treturn fmt.Sprintf(\"    cannot reach the agent: %v\\n\", refused)\n", "TestLRN17s_")
m("LRN-17 a withdrawal keeps the stamp", LG,
  "\tl.file.Lessons[i].Tier = TierProposed\n\tl.file.Lessons[i].ApprovedUnixMS = 0\n",
  "\tl.file.Lessons[i].Tier = TierProposed\n", "TestLRN17w_")
m("LRN-17 withdraw without the code", C,
  "\tif err := typedAtTerminal(ls, \"to stop it reaching the agent\", \"withdrawn\", \"withdraws\"); err != nil {\n\t\treturn err\n\t}\n",
  "", "TestLRN17w_")

# LEARN-DX1 F2: lessons and the Stop recap read a stamp-0 approval as learn
# does, not sent and awaiting approval.
LV = "cmd/rashomon/verbs_lessons.go"
m("LRN-17 lessons says nothing of a stamp-0 approval", LV,
  "\t\t\t\trows.WriteString(notSentLine(learn.Lesson{ID: ls.ID, Tier: ls.Tier, Text: ls.Text, ApprovedUnixMS: ls.ApprovedUnixMS}))\n",
  "", "TestLRN17s_")
m("LRN-17 the lessons row drops the approval time", LV,
  "SinceUnixMS: ls.SinceUnixMS, ApprovedUnixMS: ls.ApprovedUnixMS, Tally:", "SinceUnixMS: ls.SinceUnixMS, Tally:",
  "TestLRN17s_")
m("LRN-17 the recap counts a stamp-0 approval approved", "internal/recap/impact.go",
  "\t\t\tif l.AwaitsApproval() {\n", "\t\t\tif false {\n", "TestLRN17s_")

# LEARN-RIDE1 (R13 D1): a lesson rides the preflight note of the prompt that
# names its path, as the note's last line inside its cap, the unknowns line
# dropped first; the ride is given, once, and nothing goes beside it.
N = "internal/intent/note.go"
m("LRN-17 a lesson never rides the note", R,
  "intent.ReadDelivered(lines), nil, afterSeq, digest, rideFor(a, given, paths))",
  "intent.ReadDelivered(lines), nil, afterSeq, digest, nil)", "TestLRN17r_")
m("LRN-17 a lesson rides a note whose prompt does not name it", V,
  "\tls, _, ok := pick(a, delivered, paths, false)\n", "\tls, _, ok := pick(a, delivered, paths, true)\n",
  "TestLRN17r_")
m("LRN-17 a lesson that rode goes again beside the note", R,
  "; ok && !rode {\n", "; ok && (!rode || true) {\n", "TestLRN17r_")
m("LRN-17 a lesson that rode the note is not given", V,
  "\t\tcase string(pattern.TaskPreflight):\n", "\t\tcase \"\":\n", "TestLRN17r_")
m("LRN-17 the unknowns line kept while the note steps down", N,
  "\tcase !ride:\n", "\tcase true:\n", "TestLRN17r_TheUnknownsLineGoesFirst")
m("LRN-17 the first note says a lesson rode it", N,
  "rides(lesson != nil && !first)", "rides(lesson != nil)", "TestLRN17r_ALessonRidesInsideTheNotesCap")
# #197 F2: a lesson rides only a prompt that names its path whole. The
# request's paths are root-relative, so a substring or a /-separated suffix
# is another file; a full stop or a leading "./" is the same path.
WHOLE = "TestLRN17r_ALessonRidesOnlyAPathItNames"
m("LRN-17 a lesson rides on a substring of its path", V,
  "tok != \"\" && slices.Contains(paths, tok) {\n",
  "tok != \"\" && slices.ContainsFunc(paths, func(p string) bool { return p != \"\" && strings.Contains(text, p) }) {\n",
  "TestLRN17r_ALessonRidesNoPromptAboutAnotherFile")
m("LRN-17 a lesson rides a nested file of the same name", V,
  "tok != \"\" && slices.Contains(paths, tok) {\n",
  "tok != \"\" && slices.ContainsFunc(paths, func(p string) bool { return tok == p || strings.HasSuffix(tok, \"/\"+p) }) {\n",
  WHOLE)
m("LRN-17 a full stop hides the path", V,
  "strings.TrimRight(text[i:j], \".\")", "text[i:j]", WHOLE)
m("LRN-17 a leading ./ hides the path", V,
  "strings.TrimPrefix(strings.TrimRight(text[i:j], \".\"), \"./\")", "strings.TrimRight(text[i:j], \".\")", WHOLE)
