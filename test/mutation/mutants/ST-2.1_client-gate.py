# Card 2.1: the per-call context gate behind the probed client version (tech
# spec :2019, G3; read as a floor per event by card VERSION-GATE1). A
# model-visible field, additionalContext or a decision, prints only on a row
# whose floor a live probe set, for a client version at or above it by
# component and not excluded, with the mirror verified and on the main agent;
# the exempt row and a systemMessage are never gated; Marshal is the gate
# under the zero Client; the version is read only from the harness form of
# AI_AGENT. The kill row is the named test's.
C = "internal/steer/client.go"
F = "internal/steer/finding.go"

# The gate in the marshal, the table, and the rows it reads.
m("ST-2.1 PostToolUse prints additionalContext ungated", F,
  '\tif o.AdditionalContext != "" && !verified(keys, event, c, table) {\n', '\tif false {\n', "TestST4_")
m("ST-2.1 the decision gate dropped", F,
  '\tif o.Decision != nil && !verified(keys, event, c, table) {\n', '\tif false {\n', "TestST4_")
m("ST-2.1 PostToolUse's floor dropped", C,
  '\tstring(EventPostToolUse): {floor: "2.1.280"},\n', '\tstring(EventPostToolUse): {},\n', "TestST4_")
# PostToolUseFailure's floor is the version its own P-TRU-1 run passed on,
# 2.1.304. PostToolUse's 2.1.280, on the strength of the same payload shape,
# is a floor without a probe (D226).
m("ST-2.1 PostToolUseFailure's floor dropped", C,
  '\tstring(EventPostToolUseFailure): {floor: "2.1.304"},\n', '\tstring(EventPostToolUseFailure): {},\n', "TestST4_")
m("ST-2.1 PostToolUseFailure given PostToolUse's floor without its own probe", C,
  '\tstring(EventPostToolUseFailure): {floor: "2.1.304"},\n',
  '\tstring(EventPostToolUseFailure): {floor: "2.1.280"},\n', "TestST4_")
m("ST-2.1 PreToolUse given a floor without a probe", C,
  '\tstring(EventPreToolUse): {},\n', '\tstring(EventPreToolUse): {floor: "2.1.280"},\n', "TestST4_")
m("ST-2.1 SessionStart given a floor without a probe", C,
  '\tstring(EventSessionStart):  {},\n', '\tstring(EventSessionStart):  {floor: "2.1.280"},\n', "TestST4_")
m("ST-2.1 the exempt row gated", F,
  '\tstring(EventUserPromptSubmit):   {context: true, exempt: true},\n',
  '\tstring(EventUserPromptSubmit):   {context: true},\n', "TestST4_")
m("ST-2.1 the PostToolUse row's context flag not lifted", F,
  '\tstring(EventPostToolUse):        {context: true},\n', '\tstring(EventPostToolUse):        {},\n', "TestST4_")

# The predicate: shutBy, read as a floor per event.
m("ST-2.1 a row with no floor read as verified", C,
  '\tcase b.floor == "":\n\t\treturn shutEventClosed\n', '', "TestST4_")
m("ST-2.1 the floor read as the one verified version", C,
  '\tcase compareVersions(c.Version, b.floor) < 0:\n', '\tcase compareVersions(c.Version, b.floor) != 0:\n', "TestST4_")
m("ST-2.1 the floor read as a ceiling", C,
  '\tcase compareVersions(c.Version, b.floor) < 0:\n', '\tcase compareVersions(c.Version, b.floor) > 0:\n', "TestST4_")
m("ST-2.1 the floor compared as a string", C,
  '\tcase compareVersions(c.Version, b.floor) < 0:\n', '\tcase c.Version < b.floor:\n', "TestST4_")
m("ST-2.1 a malformed version compared", C,
  "\tcase !digitRuns(c.Version, '.'):\n\t\treturn shutUnknownVersion\n", '', "TestST4_")
m("ST-2.1 the exclusions ignored", C,
  '\tcase b.excludes(c.Version):\n', '\tcase false:\n', "TestST4_")
m("ST-2.1 an exclusion matched as text", C,
  '\t\tif compareVersions(v, x) == 0 {\n', '\t\tif v == x {\n', "TestVG1b_")
m("ST-2.1 MirrorInBand ignored", C,
  '\tcase !c.MirrorInBand:\n\t\treturn shutNoMirror\n', '', "TestST4_")
m("ST-2.1 Subagent ignored", C,
  '\tcase c.Subagent:\n\t\treturn shutSubagent\n', '', "TestST4_")
m("ST-2.1 Marshal calls MarshalFor with a verified Client", F,
  '\treturn o.MarshalFor(event, Client{})\n',
  '\treturn o.MarshalFor(event, Client{Version: "2.1.280", Source: VersionSourceEnv, MirrorInBand: true})\n',
  "TestST4b_")
m("ST-2.1 ContextAllowed reads another table than MarshalFor", C,
  '\treturn allowed(event, c, mirrorVerifiedOn())\n',
  '\treturn allowed(event, c, map[string]channelBasis{string(EventPostToolUse): {floor: "2.1.280"}, '
  'string(EventSessionStart): {floor: "2.1.280"}})\n', "TestTRU12_")

# The comparison (compareVersions).
m("ST-2.1 a component's leading zeros kept", C,
  '\t\tx, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")\n', '', "TestVG1a_")
m("ST-2.1 components compared as text, not by length first", C,
  '\t\tif len(x) != len(y) {\n', '\t\tif false {\n', "TestVG1a_")
m("ST-2.1 the comparison stops at the shorter version", C,
  '\tfor a != "" || b != "" {\n', '\tfor a != "" && b != "" {\n', "TestVG1a_")

# The version source.
m("ST-2.1 ClientVersion accepts letters", C,
  "\t\tcase b >= '0' && b <= '9':\n", "\t\tcase b != '-':\n", "TestST4c_")
m("ST-2.1 ClientVersion drops its length bound", C,
  '\tif len(aiAgent) > maxAIAgent {\n', '\tif false {\n', "TestST4c_")
m("ST-2.1 ClientVersion reads another client's value", C,
  '\tv, ok := strings.CutPrefix(aiAgent, aiAgentPrefix)\n', '\t_, v, ok := strings.Cut(aiAgent, "_")\n', "TestST4c_")
m("ST-2.1 ClientVersion reads any kind, the _agent form included", C,
  '\t\tv, ok = strings.CutSuffix(v, aiAgentSuffix)\n', '\t\tv, _, ok = strings.Cut(v, "_")\n', "TestST4c_")
m("ST-2.1 ClientVersion reads one component", C,
  '\treturn inRun && runs >= 2 && runs <= 4\n', '\treturn inRun && runs >= 1 && runs <= 4\n', "TestST4c_")
m("ST-2.1 ClientVersion reads five components", C,
  '\treturn inRun && runs >= 2 && runs <= 4\n', '\treturn inRun && runs >= 2\n', "TestST4c_")
