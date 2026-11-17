# RV-1 item 3: a prompt the host framed (a background job's notification, a
# harness notice, a message from another session) is read by its envelope
# alone (harness.Cut). A message from another session reaches the hook as the
# bare element, so it is framed only when the whole prompt, outer white space
# trimmed, is one element: the opening tag alone with the from= key on the
# first line, the closing tag alone on the last, no other line the closing
# tag alone; a head the reader cut is framed when no line of it is the
# closing tag alone. Cut holds it, so the graph brief, which reads Cut alone,
# builds nothing from it. A prompt that is frames only names no path in the
# preflight, and it takes a flagged Stop's done verdict without telling it.
# Each break below reads a frame the host did not write or misses one it did
# (TestRV1_3a_), loses the cut on its way to Framed or to the graph brief
# (TestRV1_3b_, TestBuild_ARequestWithNoTaskIsNotSent), reads the element in
# Framed only (TestBuild_ARequestWithNoTaskIsNotSent), or tells the verdict
# at a framed prompt or holds it to a typed one, where it names the wrong turn
# (TestRV1_3c_).
F = "internal/harness/frame.go"
P = "cmd/rashomon/preflight.go"
R = "cmd/rashomon/recap_turn.go"
GUARD = "\treturn strings.Contains(prompt, \"[\") || strings.Contains(prompt, \"-notification>\")\n"
FIRST = "\tif !ok || !peerOpen(first) || !strings.Contains(first, peerFrom) {\n"
TAKE = "\tpp.unverified = flagged && recap.TakeUnverified(root, d.SessionID) && !pp.framed\n"
m("RV-1 item 3 a notice is read without its bracket", F,
  GUARD, GUARD.replace("strings.Contains(prompt, \"[\") || ", ""),
  "TestRV1_3a_")
m("RV-1 item 3 a notification is read without its tag end", F,
  GUARD, GUARD.replace(" || strings.Contains(prompt, \"-notification>\")", ""),
  "TestRV1_3a_")
m("RV-1 item 3 a peer message needs no from= key", F,
  FIRST, FIRST.replace(" || !strings.Contains(first, peerFrom)", ""),
  "TestRV1_3a_")
m("RV-1 item 3 a closing tag alone inside the body is accepted", F,
  "\treturn closes == 1 && last == peerClose\n", "\treturn closes >= 1 && last == peerClose\n",
  "TestRV1_3a_")
m("RV-1 item 3 the cut is ignored", F,
  "\tif cut {\n", "\tif false {\n",
  "TestRV1_3a_")
m("RV-1 item 3 a cut head may hold a closing tag", F,
  "\t\treturn closes == 0\n", "\t\treturn true\n",
  "TestRV1_3a_")
m("RV-1 item 3 the prompt is read untrimmed", F,
  "strings.Cut(strings.TrimSpace(prompt), \"\\n\")", "strings.Cut(prompt, \"\\n\")",
  "TestRV1_3a_")
m("RV-1 item 3 the bare element is read in Framed only", "internal/graph/engine/notask.go",
  "\treturn harness.Cut(prompt, cut)\n",
  "\ttyped, held := harness.Cut(prompt, cut)\n\tif held && typed == \"\" {\n\t\tif _, h := harness.Cut(prompt+\"\\nx\", cut); !h {\n\t\t\treturn prompt, false\n\t\t}\n\t}\n\treturn typed, held\n",
  "TestBuild_ARequestWithNoTaskIsNotSent")
m("RV-1 item 3 the graph brief reads a cut prompt as whole", "internal/graph/engine/brief.go",
  "notice(prompt, opt.PromptCut)", "notice(prompt, false)",
  "TestBuild_ARequestWithNoTaskIsNotSent")
m("RV-1 item 3 the cut does not reach the brief's options", "cmd/rashomon/graph.go",
  "\topt.PromptCut = cut\n", "",
  "TestRV1_3b_")
m("RV-1 item 3 the preflight drops the cut on its way to the graph brief", P,
  "p.PromptID, p.Prompt, p.PromptCut, specOf", "p.PromptID, p.Prompt, false, specOf",
  "TestRV1_3b_")
m("RV-1 item 3 Framed reads a cut prompt as whole", R,
  "\tpp.framed, _ = harness.Framed(string(pp.Prompt), pp.PromptCut)\n", "\tpp.framed, _ = harness.Framed(string(pp.Prompt), false)\n",
  "TestRV1_3b_")
m("RV-1 item 3 a framed prompt names paths", P,
  "\tnamed := !p.framed && intent.Names(p.Prompt, p.PromptCut)\n",
  "\tnamed := intent.Names(p.Prompt, p.PromptCut)\n",
  "TestRV1_3b_")
m("RV-1 item 3 a framed prompt is told the done verdict", R,
  TAKE, TAKE.replace(" && !pp.framed", ""),
  "TestRV1_3c_")
m("RV-1 item 3 a framed prompt holds the done verdict for a typed one", R,
  TAKE, "\tpp.unverified = flagged && !pp.framed && recap.TakeUnverified(root, d.SessionID)\n",
  "TestRV1_3c_")
