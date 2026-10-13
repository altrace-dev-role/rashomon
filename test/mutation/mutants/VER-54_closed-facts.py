# VER-54: no fact field carries output or edit text. A fact field typed as a
# bare string is the whole regression; the test walks the four fact structs.
m("VER-54 a gate fact carries its id as free text", "internal/verify/finding.go",
  "\tGateID       SlotID\n", "\tGateID       string\n", "TestVER54b_")
# ToSteer, the verify-to-steer mapping (P07N-1 to -5, -13; verify F11, F12).
m("VER-54 the masked claim reaches the model at stop", "internal/verify/tosteer_card.go",
  '\t\tif kind == "masked" {\n\t\t\treturn flag, true\n', '\t\tif kind == "masked-never" {\n\t\t\treturn flag, true\n', "TestVER54e_")
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
  "\t\tcase f.Pattern == pattern.ClaimVsRecord && f.Claim.UnrecoveredFailures > 0:\n", "\t\tcase false:\n", "TestVER54e_")
m("VER-54 a done claim over absent is not done-unverified", "internal/verify/tosteer.go",
  '\t\tcase f.Claim.Class == ClaimDone && (k == "stale" || k == "absent"):\n', "\t\tcase false:\n", "TestVER54e_")
m("VER-54 an untrusted rule is not capped at flag", "internal/verify/tosteer_card.go",
  "\tuntrusted := f.Rule != nil && f.Rule.Trust == spec.Untrusted\n", "\tuntrusted := f.Rule != nil && f.Rule.Trust == spec.Managed\n", "TestVER54e_")
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
