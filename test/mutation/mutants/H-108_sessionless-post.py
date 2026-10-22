# H-108 -- a tool payload that names no session writes no execution record,
# and the report counts the calls that arrived that way. The first break is
# the defect as it shipped; the rest are the other ways to lose the fix -- a
# count that is never taken, an unknown count read as zero, a line that stops
# rendering, the failure event exempted, a failure count over no outcomes.
m("H-108 a post that names no session records its execution as ok", "internal/hook/post.go",
  "\tif pl.SessionID == \"\" {\n\t\treturn nil\n\t}\n", "",
  "TestH108_ASessionlessCall")
m("H-108 a failure event that names no session is exempt from the guard", "internal/hook/post.go",
  "\tif pl.SessionID == \"\" {\n\t\treturn nil", "\tif pl.SessionID == \"\" && pl.HookEventName != FailureEvent {\n\t\treturn nil",
  "TestH108_ASessionlessFailureEvent")
m("H-108 the report never counts the unattributed run", "internal/report/report.go",
  "\tn := len(run.Declarations)\n\treturn &n", "\tn := 0 * len(run.Declarations)\n\treturn &n",
  "TestH108_ASessionlessCall|TestCallsWithoutSessionID_")
m("H-108 an unreadable unattributed run is counted as zero", "internal/report/report.go",
  "\tif err != nil || run.Skipped > 0 {\n\t\treturn nil\n\t}\n\tn := len(run.Declarations)",
  "\tif err != nil || run.Skipped > 0 {\n\t\tzero := 0\n\t\treturn &zero\n\t}\n\tn := len(run.Declarations)",
  "TestCallsWithoutSessionID_IsUnknown")
m("H-108 a run with lines that did not parse is counted as if it were whole", "internal/report/report.go",
  "\tif err != nil || run.Skipped > 0 {\n\t\treturn nil", "\tif err != nil {\n\t\treturn nil",
  "TestCallsWithoutSessionID_IsUnknownOverLines")
m("H-108 the count's healthy twin vanishes", "internal/report/text.go",
  "\tcase *n == 0:\n\t\tfmt.Fprintf(b, \"calls without a session id: %s\\n\", none)", "\tcase *n == 0:",
  "TestH108_ACodexShaped|TestText_CallsWithoutSessionID")
m("H-108 the count is built and never rendered", "internal/report/text.go",
  "\twriteSessionless(&b, rep.CallsWithoutSessionID)\n", "",
  "TestH108_ASessionlessCall")
m("H-108 the without-execution line joins every id again", "internal/report/text.go",
  "\t\tout[i] = u.ToolUseID + \" (\" + orUnknown(u.PermissionMode) + \")\"\n\t}\n\treturn list(out)",
  "\t\tout[i] = u.ToolUseID + \" (\" + orUnknown(u.PermissionMode) + \")\"\n\t}\n\treturn strings.Join(out, \", \")",
  "TestUnexecutedIsBounded")
m("H-108 the unattributed block reports a failure count over no outcomes", "internal/report/text.go",
  "\tif sess.SessionID == store.UnattributedSession {", "\tif false {",
  "TestText_TheUnattributedRunHasNoFailureCount|TestH108_ASessionlessCall")
