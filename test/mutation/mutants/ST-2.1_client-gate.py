# Card 2.1: the per-call context gate behind the probed client version (tech
# spec :2019, G3). A model-visible field, additionalContext or a decision,
# prints only on a row a live probe verified for the exact client version,
# with the mirror verified and on the main agent; the exempt row and a
# systemMessage are never gated; Marshal is the gate under the zero Client;
# the version is read only from the harness form of AI_AGENT. The kill row
# is the named test's.
C = "internal/steer/client.go"
F = "internal/steer/finding.go"

# The gate in the marshal, the table, and the rows it reads.
m("ST-2.1 PostToolUse prints additionalContext ungated", F,
  '\tif o.AdditionalContext != "" && !verified(keys, event, c, table) {\n', '\tif false {\n', "TestST4_")
m("ST-2.1 the decision gate dropped", F,
  '\tif o.Decision != nil && !verified(keys, event, c, table) {\n', '\tif false {\n', "TestST4_")
m("ST-2.1 PostToolUse's probed version dropped", C,
  '\tstring(EventPostToolUse): {"2.1.280"},\n', '\tstring(EventPostToolUse): {},\n', "TestST4_")
m("ST-2.1 PostToolUseFailure given 2.1.280 without a probe", C,
  '\tstring(EventPostToolUseFailure): {},\n', '\tstring(EventPostToolUseFailure): {"2.1.280"},\n', "TestST4_")
m("ST-2.1 PreToolUse given 2.1.280 without a probe", C,
  '\tstring(EventPreToolUse): {},\n', '\tstring(EventPreToolUse): {"2.1.280"},\n', "TestST4_")
m("ST-2.1 the exempt row gated", F,
  '\tstring(EventUserPromptSubmit):   {context: true, exempt: true},\n',
  '\tstring(EventUserPromptSubmit):   {context: true},\n', "TestST4_")
m("ST-2.1 the PostToolUse row's context flag not lifted", F,
  '\tstring(EventPostToolUse):        {context: true},\n', '\tstring(EventPostToolUse):        {},\n', "TestST4_")

# The predicate.
m("ST-2.1 a row absent from the table read as verified", C,
  '\tversions := table[event]\n', '\tversions, listed := table[event]\n\tif !listed {\n\t\treturn true\n\t}\n',
  "TestST4_")
m("ST-2.1 a listed version matched as a prefix of the client's", C,
  'slices.Contains(versions, c.Version)',
  'slices.ContainsFunc(versions, func(v string) bool { return strings.HasPrefix(c.Version, v) })', "TestST4_")
m("ST-2.1 the client's version matched as a prefix of a listed one", C,
  'slices.Contains(versions, c.Version)',
  'slices.ContainsFunc(versions, func(v string) bool { return strings.HasPrefix(v, c.Version) })', "TestST4_")
m("ST-2.1 MirrorInBand ignored", C,
  '\treturn c.MirrorInBand && !c.Subagent && ', '\treturn !c.Subagent && ', "TestST4_")
m("ST-2.1 Subagent ignored", C,
  '\treturn c.MirrorInBand && !c.Subagent && ', '\treturn c.MirrorInBand && ', "TestST4_")
m("ST-2.1 Marshal calls MarshalFor with a verified Client", F,
  '\treturn o.MarshalFor(event, Client{})\n',
  '\treturn o.MarshalFor(event, Client{Version: "2.1.280", Source: VersionSourceEnv, MirrorInBand: true})\n',
  "TestST4b_")
m("ST-2.1 ContextAllowed reads another table than MarshalFor", C,
  '\treturn allowed(event, c, mirrorVerifiedOn)\n',
  '\treturn allowed(event, c, map[string][]string{string(EventPostToolUse): {"2.1.280"}, '
  'string(EventSessionStart): {"2.1.280"}})\n', "TestTRU12_")

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
