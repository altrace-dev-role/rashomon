# test-7: Claude Code payloads and a transcript captured from a live 2.1.280
# session drive hook, post, recap, report and spend (rubric fix list entry 37).
m("test-7  a settings deny rule's refusal reads as a result", "internal/report/transcript.go",
  "\tif strings.HasPrefix(text, \"Permission to use \") && strings.Contains(text, \" has been denied\") {\n\t\treturn true\n\t}",
  "\tif false && strings.HasPrefix(text, \"Permission to use \") {\n\t\treturn true\n\t}", "TestRubricTest7_")
m("test-7  every line of a response is priced again", "internal/spend/scan.go",
  "\t\tprev, seen := byID[cand.ID]\n\t\tif !seen {",
  "\t\tprev, seen := byID[cand.ID]\n\t\tseen = false\n\t\tif !seen {", "TestRubricTest7_")
m("test-7  the exit code is read past the first line", "internal/hook/post.go",
  "\tfirst, _, _ := strings.Cut(msg[len(exitCodePrefix):], \"\\n\")\n",
  "\tfirst := msg[len(exitCodePrefix):]\n", "TestRubricTest7_")
m("test-7  digest --stdin drops the Stop payload's final message", "cmd/rashomon/main.go",
  "\treturn fm.Message, ok && fm.Message != \"\"\n", "\treturn fm.Message, false\n", "TestRubricTest7_")
