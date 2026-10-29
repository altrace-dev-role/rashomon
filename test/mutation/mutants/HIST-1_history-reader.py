# B1 (docs/autosteer-prescan.md): the history reader joins each tool_use to
# its tool_result by id, emits calls in tool_use order, reads an outcome from
# an anchored exit code with 124, 126 and 127 as classes of their own, never
# reads a denial as a failure, and grades what it cannot trust below Verified.
F = "internal/history/history.go"
m("hist-1  a denial reads as a failure", F,
  "\tcase report.IsDenial(isError, text):\n\t\treturn Denied, nil\n", "\tcase false:\n\t\treturn Denied, nil\n", "TestHIST1_|TestHIST2_")
m("hist-1  exit 124 is a plain failure", F,
  "\tcase 124:\n\t\treturn Timeout, &code\n", "\tcase -124:\n\t\treturn Timeout, &code\n", "TestHIST2_")
m("hist-1  126 and 127 are plain failures", F,
  "\tcase 126, 127:\n", "\tcase -126:\n", "TestHIST2_")
m("hist-1  the exit code is read anywhere in the text", F,
  "\trest, ok := strings.CutPrefix(text, exitPrefix)\n\tif !ok {\n\t\treturn 0, false\n\t}\n",
  "\ti := strings.Index(text, exitPrefix)\n\tif i < 0 {\n\t\treturn 0, false\n\t}\n\trest := text[i+len(exitPrefix):]\n", "TestHIST2_")
m("hist-1  calls are emitted as their results arrive", F,
  "\tfor len(rd.queue) > 0 && rd.queue[0].answered {\n\t\trd.pop(ReasonNone)\n\t}\n",
  "\tfor i := 0; i < len(rd.queue); i++ {\n\t\tif rd.queue[i].answered {\n\t\t\trd.queue[0], rd.queue[i] = rd.queue[i], rd.queue[0]\n\t\t\trd.pop(ReasonNone)\n\t\t\ti = -1\n\t\t}\n\t}\n", "TestHIST3_|FuzzHistCalls")
m("hist-1  an unanswered call keeps an outcome", F,
  "\t\tp.Outcome, p.ExitCode = OutcomeNone, nil\n\t\tp.Reason = why\n", "\t\tp.Outcome = OK\n\t\tp.Reason = why\n", "TestHIST3_|FuzzHistCalls")
m("hist-1  an unsupported version is trusted", F,
  "\tcase !Supported(p.Version):\n", "\tcase false:\n", "TestHIST4_")
m("hist-1  a line skipped between a call and its result goes unnoticed", F,
  "\tif rd.unread > p.unread && p.Reason == ReasonNone {\n", "\tif false && rd.unread > p.unread && p.Reason == ReasonNone {\n", "TestHIST4_|FuzzHistCalls")
m("hist-1  disagreeing results go unnoticed", F,
  "\t\tif out != p.Outcome || !sameCode(code, p.ExitCode) {\n", "\t\tif false {\n", "TestHIST4_")
m("hist-1  a repeated tool_use is a second call", F,
  "\tif rd.seen[b.ID] || rd.byID[b.ID] != nil {\n", "\tif false {\n", "TestHIST5_|FuzzHistCalls")
m("hist-1  the queue is unbounded", F,
  "\tfor len(rd.queue) > rd.lim.MaxOutstanding || rd.queued > rd.lim.MaxQueuedBytes && len(rd.queue) > 1 {\n",
  "\tfor false {\n", "TestHIST7_")
m("hist-1  an oversized input is kept", F,
  "\tif len(b.Input) <= rd.lim.MaxInputBytes {\n", "\tif true {\n", "TestHIST7_")
m("hist-1  a Bash error with no exit code is Verified", F,
  "\tcase p.ToolName == \"Bash\" && p.Outcome == ToolError:\n", "\tcase false:\n", "TestHIST2_")
