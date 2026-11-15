# RV-1 item 3: a prompt the host framed (a background job's notification, a
# harness notice, a message from another session) is read by its envelope
# alone (harness.Cut), by position: the peer frame is its lead line, the
# opening tag on the next line, and the first closing tag alone on a line
# followed by an empty line and the closing paragraph. A prompt that is
# frames only names no path in the preflight, and it takes a flagged Stop's
# done verdict without telling it. Each break below reads a frame the host
# did not write (TestRV1_3a_), reads a peer's text as the user's request
# (TestRV1_3b_), or tells the verdict at a framed prompt or holds it to a
# typed one, where it names the wrong turn (TestRV1_3c_).
F = "internal/harness/frame.go"
P = "cmd/rashomon/preflight.go"
R = "cmd/rashomon/recap_turn.go"
LEAD = "\tif strings.TrimSpace(lines[i]) != peerLead || i+1 >= len(lines) || !peerOpen(strings.TrimSpace(lines[i+1])) {\n"
EXIT = "\t\treturn 0, false\n\t}\n"
LOOP = ("\tfor j := i + 2; j+2 < len(lines); j++ {\n"
        "\t\tif lines[j] == peerClose && lines[j+1] == \"\" && strings.HasPrefix(lines[j+2], peerTrailer) {\n"
        "\t\t\treturn j + 2, true\n\t\t}\n\t}\n")
GUARD = "\treturn strings.Contains(prompt, \"[\") || strings.Contains(prompt, \"-notification>\") || strings.Contains(prompt, peerLead)\n"
TAKE = "\tpp.unverified = flagged && recap.TakeUnverified(root, d.SessionID) && !pp.framed\n"
m("RV-1 item 3 a peer frame needs no lead line", F,
  LEAD, LEAD.replace("strings.TrimSpace(lines[i]) != peerLead || ", ""),
  "TestRV1_3a_")
m("RV-1 item 3 a peer frame is its lead line and opening tag", F,
  LOOP, "\treturn i + 1, true\n",
  "TestRV1_3a_")
m("RV-1 item 3 a peer frame ends at its closing tag", F,
  LOOP, "\tfor j := i + 2; j < len(lines); j++ {\n\t\tif lines[j] == peerClose {\n\t\t\treturn j, true\n\t\t}\n\t}\n",
  "TestRV1_3a_")
m("RV-1 item 3 the closing paragraph counts anywhere in the prompt", F,
  LOOP, LOOP.replace("j+2 < len(lines)", "j < len(lines)").replace(
      "lines[j+1] == \"\" && strings.HasPrefix(lines[j+2], peerTrailer)",
      "strings.Contains(strings.Join(lines, \"\\n\"), peerTrailer)").replace("return j + 2, true", "return j, true"),
  "TestRV1_3a_")
m("RV-1 item 3 the lead line counts anywhere in the prompt", F,
  LEAD + EXIT + "\tfor j := i + 2;",
  "\tif !strings.Contains(strings.Join(lines, \"\\n\"), peerLead) || !peerOpen(strings.TrimSpace(lines[i])) {\n" + EXIT + "\tfor j := i + 1;",
  "TestRV1_3a_")
m("RV-1 item 3 a notice is read without its bracket", F,
  GUARD, GUARD.replace("strings.Contains(prompt, \"[\") || ", ""),
  "TestRV1_3a_")
m("RV-1 item 3 a notification is read without its tag end", F,
  GUARD, GUARD.replace(" || strings.Contains(prompt, \"-notification>\")", ""),
  "TestRV1_3a_")
m("RV-1 item 3 a peer frame is read without its lead line", F,
  GUARD, GUARD.replace(" || strings.Contains(prompt, peerLead)", ""),
  "TestRV1_3a_")
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
