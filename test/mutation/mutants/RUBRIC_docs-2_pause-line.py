# Rubric docs-2: `rashomon pause` said no tool call would be recorded, while
# each paused call still writes a recording_paused coverage note (H-80). The
# line now says so where a store exists, and claims no note where none does.
m("docs-2 pause says nothing is recorded again", "cmd/rashomon/main.go",
  "\tfmt.Fprintln(stdout, \"rashomon: paused -- tool calls are not recorded until `rashomon resume`\"+note)\n",
  "\tfmt.Fprintln(stdout, \"rashomon: paused -- no tool call will be recorded until `rashomon resume`\"+note[:0])\n",
  "TestRubricDOCS2_")
m("docs-2 pause claims a coverage note with no store", "cmd/rashomon/main.go",
  "\tnote := \"\"\n",
  "\tnote := pausedCoverageNote\n",
  "TestRubricDOCS2_")
