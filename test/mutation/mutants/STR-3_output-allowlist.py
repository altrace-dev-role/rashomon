# STR-3: Output.Marshal prints a key only on an event whose row allows it
# (tech 4.2.3's per-event keys; archive 40.3.2's "Output STEER may emit"),
# and permissionDecision carries only deny or ask. A per-call event or Stop
# printing additionalContext, PreToolUse output without a decision, or an
# "allow" is the whole regression.
m("STR-3 PostToolUse prints additionalContext", "internal/steer/finding.go",
  "\tstring(EventPostToolUse):        {},\n",
  "\tstring(EventPostToolUse):        {context: true},\n", "TestSTR3_")
m("STR-3 a Stop nudge continues the turn without a project stop", "internal/steer/finding.go",
  "\tstring(EventStop):               {stopContinue: true},\n",
  "\tstring(EventStop):               {context: true, stopContinue: true},\n", "TestSTR3_")
m("STR-3 PreToolUse prints additionalContext beside its decision", "internal/steer/finding.go",
  "\tstring(EventPreToolUse):         {decision: true},\n",
  "\tstring(EventPreToolUse):         {decision: true, context: true},\n", "TestSTR3_")
m("STR-3 Marshal stops checking where AdditionalContext may go", "internal/steer/finding.go",
  "\tif o.AdditionalContext != \"\" && !keys.context {\n",
  "\tif false {\n", "TestSTR3_")
m("STR-3 PreToolUse prints output without a decision", "internal/steer/finding.go",
  "\tif keys.decision && o.Decision == nil {\n",
  "\tif false {\n", "TestSTR3_")
m("STR-3 a deny prints permissionDecision allow", "internal/steer/finding.go",
  "\t\t\thso[\"permissionDecision\"] = \"deny\"\n",
  "\t\t\thso[\"permissionDecision\"] = \"allow\"\n", "TestSTR3_")
