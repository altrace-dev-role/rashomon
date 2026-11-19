# EDIT-MARKS the edit-marks producer (W-M3.7; issue 207): the post hook writes
# an Edit's and a MultiEdit's marks from a closed, versioned pattern table.
# Each break removes one rule of the ruling: an assertion pattern, the bare
# comparison that is no assertion, the comment that is no line, the word
# boundary, the sign of the delta, a MultiEdit's later edits, the path
# filters, the call itself, the failure event, the switch, the caps, and the
# one that matters for consent: a mark that carries bytes of the edit. Each
# names the test that holds it.
E = "internal/editmarks/editmarks.go"
T = "internal/editmarks/table.go"
H = "internal/hook/editmarks.go"
m("EDIT-MARKS testify's assert and require calls are no assertion", T,
  '"assert.", "require.",', '"assertx.", "requirex.",', "TestEM3_")
m("EDIT-MARKS a bare comparison is an assertion", T,
  '"assert.", "require.",', '"assert.", "require.", "if ",', "TestEM3_")
m("EDIT-MARKS a commented-out line is present", E,
  "		if c == ' ' || c == '\\t' {\n			return\n		}\n		s.lead = false",
  "		if c == ' ' || c == '\\t' || c == '/' || c == '#' {\n			return\n		}\n		s.lead = false", "TestEM4_|TestEM2_")
m("EDIT-MARKS SkipNow is no skip", T, '"t.Skip(", "t.Skipf(", "t.SkipNow(",', '"t.Skip(", "t.Skipf(",', "TestEM2_")
m("EDIT-MARKS a longer word is the mark", E,
  "		return !identByte(p[len(p)-1]) || len(rest) == 0 || !identByte(rest[0])\n", "		return true\n", "TestEM2_")
m("EDIT-MARKS TestMain is a test definition", E,
  '		return !(len(rest) >= len("Main(") && string(rest[:len("Main(")]) == "Main(")\n', "		return true\n", "TestEM1_")
m("EDIT-MARKS the old text counts as added", E,
  "	scan(body(in, from), l, -1, net)\n	scan(body(in, to), l, 1, net)\n",
  "	scan(body(in, from), l, 1, net)\n	scan(body(in, to), l, -1, net)\n", "TestEM1_|TestEM2_|TestACM36_")
m("EDIT-MARKS a net gain is a removal", E, "		if n > 0 {\n			m.Added[table[i].id] = n", "		if n < 0 {\n			m.Added[table[i].id] = n", "TestEM2_")
m("EDIT-MARKS a MultiEdit's later edits are not read", E, "				return readable\n			})", "				return false\n			})", "TestEM5_")
m("EDIT-MARKS Python is not read", E, '	case hasSuffixFold(p, ".py"):', "	case false:", "TestEM1_|TestEM2_")
m("EDIT-MARKS a source path is read", E,
  '	if !containsFold(p, "test") && !containsFold(p, "spec") && !containsFold(p, "e2e") {', "	if false {", "TestEM6_")
m("EDIT-MARKS a Write is read as an Edit", E,
  '	if tool != "Edit" && tool != "MultiEdit" {', '	if tool != "Edit" && tool != "MultiEdit" && tool != "Write" {', "TestEM6_")
m("EDIT-MARKS an escaped ASCII byte is not that byte", E, "			if r < utf8Self {", "			if r < 0 {", "TestEM7_")
m("EDIT-MARKS the marks allocate for an edit that moves nothing", E,
  "	root := jsonscan.ValueSpan(input)\n", "	input = append([]byte(nil), input...)\n	root := jsonscan.ValueSpan(input)\n", "TestEM10_")
# The consent pin: a mark carries bytes of the edit. Two leaks, one in the
# producer (the new text as an id) and one in the hook (the file path as the
# basis); the pin fails against each.
m("EDIT-MARKS a mark carries the new text", E,
  "	return marksOf(&net)\n}",
  "	mk := marksOf(&net)\n	if mk != nil {\n		mk.Added[string(body(input, top[keyNew]))] = 1\n	}\n	return mk\n}", "TestEM9_")
m("EDIT-MARKS a mark carries the old text in its basis", E,
  "	return marksOf(&net)\n}",
  "	mk := marksOf(&net)\n	if mk != nil {\n		mk.Basis += string(body(input, top[keyOld]))\n	}\n	return mk\n}", "TestEM9_")
m("EDIT-MARKS the post never fills the marks", "internal/hook/post.go",
  "	fillEditMarks(&rec, p.st, pl.HookEventName, pl.ToolName, pl.ToolInput)\n", "", "TestEM_PostWritesTheEditMarksOfAnEdit")
m("EDIT-MARKS a failure event has marks", H, "	if event == FailureEvent || len(input) == 0 {", "	if len(input) == 0 {", "TestEM_PostWritesNoMarksWhereThereAreNone")
m("EDIT-MARKS the switch is read the wrong way round", H, " || cfg.EditMarksOff()", " || !cfg.EditMarksOff()", "TestEM_TheSwitchTurnsTheMarksOff")
m("EDIT-MARKS a config that does not read leaves the marks on", H, "err != nil || cfg.EditMarksOff()", "err == nil && cfg.EditMarksOff()", "TestEM_TheSwitchTurnsTheMarksOff")
m("EDIT-MARKS the switch never reads off", "internal/store/config.go",
  "func (c Config) EditMarksOff() bool { return steerSwitchOff(c.EditMarks) }", "func (c Config) EditMarksOff() bool { return false }",
  "TestEM_ConfigEditMarksSwitch")
m("EDIT-MARKS marks over the caps are written", "internal/store/limits.go",
  "	if x.EditMarks != nil && !editMarksFit(x.EditMarks) {", "	if false {", "TestEM_ExecutionEditMarksCapAtNAndNPlusOne")
m("EDIT-MARKS the id cap is one too wide", "internal/store/limits.go",
  "			if len(id) > MaxEditMarkIDBytes {", "			if len(id) > MaxEditMarkIDBytes+1 {", "TestEM_ExecutionEditMarksCapAtNAndNPlusOne")
