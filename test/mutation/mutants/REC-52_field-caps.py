# REC-52: every value a payload hands a record verbatim is capped at capture;
# at its cap it is stored, over it it is withheld and the call's coverage
# carries record_oversize. A session id over its cap is bucketed under
# unattributed.
for field in ["TranscriptPath", "PermissionMode", "ToolName"]:
    limit = "MaxPathBytes" if field == "TranscriptPath" else "MaxIDBytes"
    m("REC-52 no cap on the declaration's " + field, "internal/store/limits.go",
      "\td.%s = w.str(d.%s, %s)\n" % (field, field, limit), "", "TestREC52_")
m("REC-52 no cap on the declaration's ToolUseID", "internal/store/limits.go",
  "\td.ToolUseID = w.id(d.ToolUseID, MaxIDBytes)\n", "", "TestREC52_")
m("REC-52 no cap on the execution's ToolUseID", "internal/store/limits.go",
  "\tx.ToolUseID = w.id(x.ToolUseID, MaxIDBytes)\n", "", "TestREC52_")
for field in ["PromptID", "AgentID", "AgentType"]:
    m("REC-52 no cap on the declaration's " + field, "internal/store/limits.go",
      "\td.%s = w.ptr(d.%s, MaxIDBytes, &d.%sWithheld)\n" % (field, field, field), "", "TestREC52_")
m("REC-52 no cap on shape.program", "internal/store/limits.go",
  "\td.Shape.Program = w.ptr(d.Shape.Program, MaxProgramBytes, nil)\n", "", "TestREC52_")
m("REC-52 no cap on hosts", "internal/store/limits.go", "\td.Hosts = w.hosts(d.Hosts, &d.HostsWithheld)\n", "", "TestREC52_")
m("REC-52 no cap on ssh_hosts", "internal/store/limits.go", "\td.SSHHosts = w.hosts(d.SSHHosts, &d.SSHHostsWithheld)\n", "", "TestREC52_")
m("REC-52 no per-host cap", "internal/store/limits.go",
  "\t\tif len(h) > MaxHostBytes {\n", "\t\tif len(h) > MaxHostBytes+1 {\n", "TestREC52b_")
m("REC-52 a host list over its cap is shortened, not withheld", "internal/store/limits.go",
  "\tif over {\n\t\t*key = w.oversize()\n\t\treturn nil\n", "\tif over {\n\t\t*key = w.oversize()\n\t\treturn list[:min(len(list), MaxHosts)]\n",
  "TestREC52_")
m("REC-52 the hook skips the tool_use_id cap on its terminal", "internal/hook/handle.go",
  "\th.toolUseID = store.CapID(p.ToolUseID)\n", "\th.toolUseID = p.ToolUseID\n", "TestREC52_")
m("REC-52 a withheld field leaves the call verified", "internal/hook/handle.go",
  "\tif reason == \"\" && h.withheld {\n", "\tif false {\n", "TestREC52_")
m("REC-52 the execution is not capped", "internal/hook/post.go",
  "\tif store.CapExecution(&rec) {\n\t\tp.withheld = true\n\t}\n", "", "TestREC52_")
m("REC-52 a withheld execution field leaves the call verified", "internal/hook/post.go",
  "\tif reason == \"\" && p.withheld {\n", "\tif false {\n", "TestREC52_")
m("REC-52 an over-cap session id is kept raw instead of digested", "internal/hook/payload.go",
  "\treturn store.CapID(id)\n", "\treturn id\n", "TestREC52_")
m("REC-52 an over-long session id leaves the call verified", "internal/hook/handle.go",
  "\th.withheld = len(p.SessionID) > store.MaxIDBytes\n", "", "TestREC52_")
m("REC-52 no cap on the coverage record's cwd", "internal/hook/coverage.go",
  "\tif len(cwd) > store.MaxPathBytes {\n", "\tif false {\n", "TestREC52_")
m("REC-52 a refusal hands the head's session id to the record uncapped", "internal/hook/payload.go",
  "\treturn attribute(tl.SessionID), tl.CWD, true\n", "\treturn tl.SessionID, tl.CWD, true\n",
  "TestREC52_")
