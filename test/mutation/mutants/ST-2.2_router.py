# Card 2.2: Route, one Output per event (tech 4.2.4, 4.2.5). The router ranks
# the admitted findings, clamps and gates each one, charges the prompt's
# budget counted from the steer log under its lock, writes one line per
# finding above off and prints one Output, or nothing when the append failed.
# The kill row is the named test's. No mutant sits on noteTexts' two
# refusals: neither is reachable through Route (card 2.2 A8), so a break
# there is equivalent through every caller and its test calls it directly.
D = "internal/steer/decide.go"
B = "internal/steer/budget.go"
R = "internal/steer/route.go"
V = "internal/steer/delivery.go"
S = "internal/steer/slots.go"

# Order (step 8; R5).
m("ST-2.2 the priority comparator reversed", D,
  '\t\t\treturn b.Priority - a.Priority\n', '\t\t\treturn a.Priority - b.Priority\n', "TestST1_")
m("ST-2.2 the after_seq tie dropped", D,
  '\t\tcase a.AfterSeq < b.AfterSeq:\n\t\t\treturn -1\n\t\tcase a.AfterSeq > b.AfterSeq:\n\t\t\treturn 1\n', '', "TestST1")

# Clamps, the event's channel, the arm and attendance (R3, R4, R12).
m("ST-2.2 a stop finding keeps stop without a project stop", D,
  '\t\t\tlevel = LevelNudge // stop needs a project stop setting, and no config exists (STR-20)\n', '', "TestR2_")
m("ST-2.2 the consent clamp dropped", D,
  '\tcase !ctx.Consent:\n\t\treturn reason(SuppressedByConsentMissing)\n', '', "TestST_ClampsRecordTheirReason")
m("ST-2.2 a note on Stop is not a flag", D,
  '\t\tif p.level == LevelNudge && rule == ruleFlagOnly {\n', '\t\tif false {\n', "TestR2_")
m("ST-2.2 PreToolUse prints", D,
  '\tcase !ok || keys.decision:\n', '\tcase !ok:\n', "TestST_PreToolUsePrintsNothing")
m("ST-2.2 arm off prints the Output", D,
  '\tif arm == ArmOff {\n\t\tnote, flag = armOff(plans)\n\t}\n', '', "TestST_ArmOffLogsAndPrintsNothing")
m("ST-2.2 an unverified channel delivers", D,
  '\t\tcase p.level == LevelNudge && !ContextAllowed(string(ctx.Event), c):\n\t\t\tp.suppress(SuppressedByUnverifiedChannel)\n',
  '', "TestTRU12b_")
m("ST-2.2 an unattended note delivers", D,
  '\t\tcase p.level == LevelNudge && attendance == AttendanceUnattended:\n', '\t\tcase false:\n', "TestED9_")
m("ST-2.2 Subagent read from the Client only", R,
  '\tc.Subagent = c.Subagent || ctx.AgentID != ""\n', '', "TestST_SubagentIsReadFromEitherSource")
m("ST-2.2 a LevelOff finding written", D,
  '\t\tif level == LevelOff {\n\t\t\tcontinue // dropped: no line, no charge (step 5)\n\t\t}\n', '', "TestST_LevelOffWritesNoLine")

# Dedupe and the budget (R5, R6).
m("ST-2.2 the same-key dedupe within a call dropped", D,
  '\t\tcase claimed[p.key]:\n\t\t\tp.suppress(SuppressedByBudgetPattern)\n', '', "TestST7b_")
m("ST-2.2 a key the prompt delivered is not read", D,
  '\t\tcase have.keys[p.key]:\n\t\t\tp.suppress(SuppressedByBudgetPattern)\n', '', "TestST7b_")
m("ST-2.2 the call's second note is not superseded", D,
  '\t\tcase note >= 0:\n\t\t\tp.state, p.by, p.decide = LineStateSuperseded, reason(SuppressedByBudgetBatch), false\n',
  '', "TestST1b_")
m("ST-2.2 the notes cap is 4", D, '\tmaxNotes = 3\n', '\tmaxNotes = 4\n', "TestST7_")
m("ST-2.2 the flags cap is 6", D, '\tmaxFlags = 5\n', '\tmaxFlags = 6\n', "TestST7c_")
m("ST-2.2 the flag printed beside a note", D,
  '\t\tif note >= 0 || flag >= 0 || have.flags >= maxFlags {\n', '\t\tif flag >= 0 || have.flags >= maxFlags {\n', "TestST_AFlagWaitsBesideANote")
m("ST-2.2 an unconfirmed line counts", B,
  '\tif !s.is(b, s.out[bkState], string(LineStateDelivered)) && !s.is(b, s.out[bkSuppressedBy], string(SuppressedByArmOff)) {\n\t\treturn\n\t}\n', '', "TestST7d_")
m("ST-2.2 another prompt's lines count", B,
  '\treturn s.is(b, p, *s.ctx.PromptID)\n', '\treturn true\n', "TestST7d_")
m("ST-2.2 the reverse scan stops one line early", B,
  '\t\tif start == 0 {\n\t\t\twhole = -1\n\t\t}\n', '', "TestST_CountsReadEveryLineOfThePrompt")
m("ST-2.2 the foreign-batch skip dropped", B,
  '\t\tcase !s.batch:\n', '\t\tcase false:\n', "TestST7e_")

# Texts and Output (R7).
m("ST-2.2 the mirror is not Mirror's", D,
  '\treturn text, user, nil\n', '\treturn text, text, nil\n', "TestST4d_")
m("ST-2.2 a flag given the mirror wrapper", D,
  '\treturn text, nil\n', '\treturn render.MirrorPrefix + text + render.MirrorSuffix, nil\n', "TestR2_")
m("ST-2.2 Stop sets StopContinue", D,
  '\t\tout = Output{SystemMessage: plans[flag].user}\n',
  '\t\tout = Output{SystemMessage: plans[flag].user, StopContinue: plans[flag].user}\n', "TestR2_")
m("ST-2.2 the Output returned when the append failed", R,
  '\t\treturn Output{}\n\t}\n\treturn out\n}\n', '\t\treturn out\n\t}\n\treturn out\n}\n', "TestSTR8_")
m("ST-2.2 a lock timeout is not counted", R,
  '\t\t\tlockTimeouts.Add(1)\n', '', "TestSTR8_")

# Delivery and slots (R10, R11).
m("ST-2.2 DeriveDelivery's budget branch returns shadow", V,
  '\t\tif by != nil && budgetReason(*by) {\n\t\t\td = DeliverySuppressedBudget\n\t\t}\n', '', "TestDeriveDelivery_")
m("ST-2.2 slotsOf writes a path list as a string", S,
  '\t\t\terr = putJSON(put, s.Name, names)\n', '\t\t\terr = putJSON(put, s.Name, v.Text())\n', "TestSlotsOf_")
m("ST-2.2 a string slot written from Text()", S,
  '\t\tcase slot.Gate:\n\t\t\terr = putJSON(put, s.Name, string(v))\n',
  '\t\tcase slot.Gate:\n\t\t\terr = putJSON(put, s.Name, v.Text())\n', "TestSlotsOf_")
