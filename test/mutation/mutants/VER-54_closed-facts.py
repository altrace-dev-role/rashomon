# VER-54: no fact field carries output or edit text. A fact field typed as a
# bare string is the whole regression; the test walks the four fact structs.
m("VER-54 a gate fact carries its id as free text", "internal/verify/finding.go",
  "\tGateID       SlotID\n", "\tGateID       string\n", "TestVER54b_")
# ToSteer, the verify-to-steer mapping (P07N-1 to -5, -13; verify F11, F12).
m("VER-54 the masked claim reaches the model at stop", "internal/verify/tosteer_card.go",
  '\t\tcase "masked":\n\t\t\treturn flag, true\n', '\t\tcase "masked-never":\n\t\t\treturn flag, true\n', "TestVER54e_")
m("VER-54 VERIFY's integrity check name crosses as the kind", "internal/verify/tosteer.go",
  '\tSkipAdded:          "skip-added",\n', '\tSkipAdded:          "skip_added",\n', "TestVER54e_")
m("VER-54 the spec rule kind crosses as the platform-rule kind", "internal/verify/tosteer.go",
  '\t\tcase f.Rule.Kind == spec.ForbidCode:\n\t\t\treturn "edit", ""\n', '\t\tcase f.Rule.Kind == spec.ForbidCode:\n\t\t\treturn steer.Kind(f.Rule.Kind), ""\n', "TestVER54e_")
m("VER-54 ordering-rule denies where its card asks", "internal/verify/tosteer_card.go",
  "\t\treturn ceiling{steer.LevelStop, steer.StopFormAsk}, true\n", "\t\treturn ceiling{steer.LevelStop, steer.StopFormDeny}, true\n", "TestVER54e_")
m("VER-54 platform-rule gets a stop form its card does not have", "internal/verify/tosteer_card.go",
  "\tcase id == pattern.PlatformRule && event == post:\n\t\treturn nudge, true\n",
  "\tcase id == pattern.PlatformRule && event == post:\n\t\treturn ceiling{steer.LevelStop, steer.StopFormDeny}, true\n", "TestVER54e_")
m("VER-54 after_seq is the fact's seq again", "internal/verify/tosteer.go",
  "\t\tAfterSeq: at.Seq,\n", "\t\tAfterSeq: slices.Max(append(factSeqs(f), 1)),\n", "TestVER54h_")
m("VER-54 the subtask-gate key is the gate, shared across tasks", "internal/verify/tosteer_card.go",
  '\t\treturn "task:" + *at.TaskRef, ""\n', '\t\treturn string(f.Gate.GateID), ""\n', "TestVER54i_")
m("VER-54 the platform-rule key drops the file", "internal/verify/tosteer_card.go",
  '\t\t\treturn string(f.Rule.RuleID) + "|" + string(*f.Rule.Path), ""\n', '\t\t\treturn string(f.Rule.RuleID), ""\n', "TestVER54i_")
m("VER-54 a claim finding may carry a rule beside the claim", "internal/verify/tosteer.go",
  "\t\treturn c && !r && !i\n", "\t\treturn c\n", "TestVER54g_")
m("VER-54 a fact seq of zero is evidence", "internal/verify/tosteer.go",
  "\t\tif seq < 1 || seq > at.Seq {\n", "\t\tif seq > at.Seq {\n", "TestVER54g_")
m("VER-54 a claim is raised at any event", "internal/verify/tosteer_card.go",
  "\tcase id == pattern.ClaimVsRecord && event == string(steer.EventStop):\n", "\tcase id == pattern.ClaimVsRecord:\n", "TestVER54g_")
m("VER-54 a claim is judged away from its stop point", "internal/verify/tosteer.go",
  "\t\tcase f.Pattern == pattern.ClaimVsRecord && at.Kind != PointStop:\n", "\t\tcase false:\n", "TestVER54g_")
m("VER-54 unrecovered failures never become silent-failure", "internal/verify/tosteer.go",
  "\t\tcase f.Pattern == pattern.ClaimVsRecord && f.Claim.UnrecoveredFailures > 0 && k != \"masked\":\n", "\t\tcase false:\n", "TestVER54e_")
m("VER-54 a done claim over absent is not done-unverified", "internal/verify/tosteer.go",
  '\t\tcase f.Claim.Class == ClaimDone && (k == "stale" || k == "absent"):\n', "\t\tcase false:\n", "TestVER54e_")
m("VER-54 an untrusted rule is not capped at flag", "internal/verify/tosteer_card.go",
  "\tcapped := f.Rule != nil && ((f.Rule.Trust != spec.Trusted && f.Rule.Trust != spec.Managed) || f.Rule.Rewritten)\n",
  "\tcapped := f.Rule != nil && ((f.Rule.Trust == spec.Managed) || f.Rule.Rewritten)\n", "TestVER54e_")
m("VER-54 a proposed fact field loses its mark", "internal/verify/finding.go",
  "\t// Added is how many matches the edit added (P-07 proposal: archive\n", "\t// Added is how many matches the edit added (archive\n", "TestVER54j_")
# Only the TaskUpdate form has the deny and the teammate flag names its task
# (P07N-3, archive 40.4.1); every fact id is checked, and a rule fact carries
# only its kind's fields (P07N-13).
m("VER-54 a TodoWrite completion is denied at PreToolUse", "internal/verify/tosteer_card.go",
  '\tcase id == pattern.SubtaskGate && kind == "" && task && event == pre:\n',
  '\tcase id == pattern.SubtaskGate && kind == "" && event == pre:\n', "TestVER54g_")
m("VER-54 a teammate close names no task", "internal/verify/tosteer_card.go",
  '\tcase id == pattern.SubtaskGate && kind == "" && task && event == eventTaskCompleted:\n',
  '\tcase id == pattern.SubtaskGate && kind == "" && event == eventTaskCompleted:\n', "TestVER54g_")
m("VER-54 a gate fact's paths go unchecked", "internal/verify/tosteer.go",
  "\t\tif !pathsValid(g.Paths, g.PathsOmitted) ||", "\t\tif false ||", "TestVER54k_")
m("VER-54 a claim's gate ids go unchecked", "internal/verify/tosteer.go",
  "\t\t\tif !id.Valid() {\n", "\t\t\tif id == \"\" {\n", "TestVER54k_")
m("VER-54 a claim's quote digest goes unchecked", "internal/verify/tosteer.go",
  ' || c.QuoteHMAC != "" && !c.QuoteHMAC.Valid() {\n', " {\n", "TestVER54k_")
m("VER-54 a rule's program goes unchecked", "internal/verify/tosteer.go",
  " || r.Program != nil && !r.Program.Valid() ||", " ||", "TestVER54k_")
m("VER-54 an integrity fact's paths go unchecked", "internal/verify/tosteer.go",
  " || i.Count < 0 || !pathsValid(i.Paths, i.PathsOmitted) {\n", " || i.Count < 0 {\n", "TestVER54k_")
m("VER-54 a command rule may carry a path", "internal/verify/tosteer.go",
  "\t\treturn r.Path == nil && r.Gate == nil\n", "\t\treturn r.Gate == nil\n", "TestVER54k_")
m("VER-54 a code rule may name no file", "internal/verify/tosteer.go",
  "\t\treturn r.Path != nil && r.Program == nil && r.Gate == nil\n", "\t\treturn r.Program == nil && r.Gate == nil\n", "TestVER54k_")
# Round 2 (P07N-13 partial; P07N-1/-3 readings): the closed vocabularies
# that decide visibility, kind and key are refused outside their lists; the
# absence kinds of claim-vs-record never continue the turn (tech 2.2.9); a
# masked verdict never crosses as silent-failure (ED-3); a rewritten input
# caps a rule at flag (40.4.6, 40.4.10).
m("VER-54 a rule's unknown trust maps as trusted", "internal/verify/tosteer_vocab.go",
  "\tif r := f.Rule; r != nil && !slices.Contains(spec.Trusts, r.Trust) {\n",
  "\tif r := f.Rule; r != nil && !slices.Contains(append(spec.Trusts, r.Trust), r.Trust) {\n", "TestVER54l_")
m("VER-54 a claim class outside the lexicon becomes the key", "internal/verify/tosteer_vocab.go",
  "\t\tcase !slices.Contains(ClaimClasses, c.Class):\n", "\t\tcase false:\n", "TestVER54l_")
m("VER-54 a claim source goes unchecked", "internal/verify/tosteer_vocab.go",
  "\t\tcase !sourceFits(f.Pattern, c.Source):\n", "\t\tcase false:\n", "TestVER54l_")
m("VER-54 a subagent's account passes as the final message", "internal/verify/tosteer_vocab.go",
  "\t\treturn s == SourceFinalMessage\n", "\t\treturn s != \"\"\n", "TestVER54l_")
m("VER-54 an unknown verdict beside failures is silent-failure", "internal/verify/tosteer_vocab.go",
  "\t\tcase c.Verdict != NoGateKnown && !slices.Contains(States, State(c.Verdict)):\n", "\t\tcase false:\n", "TestVER54l_")
m("VER-54 a gate reason goes unchecked", "internal/verify/tosteer_vocab.go",
  "\t\tif g.Reason != \"\" && !slices.Contains(Reasons, g.Reason) {\n", "\t\tif false {\n", "TestVER54m_")
m("VER-54 subtask-gate denies over a green gate", "internal/verify/tosteer_vocab.go",
  "var subtaskStates = []State{Red, MaskedInconclusive, Stale, Absent}\n",
  "var subtaskStates = []State{Red, MaskedInconclusive, Stale, Absent, Green}\n", "TestVER54l_")
m("VER-54 subtask-gate fires over a backgrounded gate", "internal/verify/tosteer_vocab.go",
  "var subtaskStates = []State{Red, MaskedInconclusive, Stale, Absent}\n",
  "var subtaskStates = []State{Red, MaskedInconclusive, Stale, Absent, Backgrounded}\n", "TestVER54l_")
m("VER-54 regression-location fires over any state", "internal/verify/tosteer_vocab.go",
  "\t\treturn g.State == Red && g.LastGreenSeq != nil &&", "\t\treturn g.State != Green && g.LastGreenSeq != nil &&", "TestVER54l_")
m("VER-54 regression-location fires with no earlier green", "internal/verify/tosteer_vocab.go",
  " && g.LastGreenSeq != nil && g.LastRunSeq != nil && *g.LastGreenSeq < *g.LastRunSeq\n",
  " && (g.LastGreenSeq == nil || g.LastRunSeq != nil && *g.LastGreenSeq < *g.LastRunSeq)\n", "TestVER54l_")
m("VER-54 regression-location's green may follow its red", "internal/verify/tosteer_vocab.go",
  "*g.LastGreenSeq < *g.LastRunSeq\n", "*g.LastGreenSeq <= *g.LastRunSeq+1\n", "TestVER54l_")
m("VER-54 the gate beside a claim may be in any state", "internal/verify/tosteer_vocab.go",
  "\treturn f.Claim != nil && g.State == State(f.Claim.Verdict)\n", "\treturn f.Claim != nil\n", "TestVER54l_")
m("VER-54 a masked verdict crosses as silent-failure", "internal/verify/tosteer.go",
  " && f.Claim.UnrecoveredFailures > 0 && k != \"masked\":\n", " && f.Claim.UnrecoveredFailures > 0:\n", "TestVER54e_")
m("VER-54 the absence kinds continue the turn", "internal/verify/tosteer_card.go",
  "\t\tcase \"red\", \"backgrounded\", \"silent-failure\":\n",
  "\t\tcase \"red\", \"backgrounded\", \"silent-failure\", \"stale\", \"absent\", \"done-unverified\":\n", "TestVER54e_")
m("VER-54 a rewritten input is not capped at flag", "internal/verify/tosteer_card.go",
  " || f.Rule.Rewritten)\n", " || false)\n", "TestVER54e_")
# Round 3 (P07N-13 partial): regression-location needs the red run it cites
# (40.4.2 "this red"), not only a green; and a gate's reason explains its
# state (2.2.4, archive 20.2.4), so a red gate explained as ok or a red
# subtask gate carrying a stale reason is refused.
m("VER-54 regression-location fires with no red run", "internal/verify/tosteer_vocab.go",
  " && g.LastRunSeq != nil && *g.LastGreenSeq < *g.LastRunSeq\n",
  " && (g.LastRunSeq == nil || *g.LastGreenSeq < *g.LastRunSeq)\n", "TestVER54l_")
m("VER-54 a gate reason may explain another state", "internal/verify/tosteer_vocab.go",
  "\t\tif g.Reason != \"\" && reasonState[g.Reason] != g.State {\n", "\t\tif false {\n", "TestVER54l_")
m("VER-54 writes_after_green explains a red gate", "internal/verify/tosteer_vocab.go",
  "\tWritesAfterGreen:      Stale,\n", "\tWritesAfterGreen:      Red,\n", "TestVER54m_")
m("VER-54 ok explains a red gate", "internal/verify/tosteer_vocab.go",
  "\tOK:                    Green,\n", "\tOK:                    Red,\n", "TestVER54l_")
m("VER-54 a masked pass explains a red gate", "internal/verify/tosteer_vocab.go",
  "\tMaskedWithSuccessLine: Green,\n", "\tMaskedWithSuccessLine: Red,\n", "TestVER54m_")
m("VER-54 a closed reason has no state", "internal/verify/tosteer_vocab.go",
  "\tBudget:                Unknown,\n", "", "TestVER54m_")
