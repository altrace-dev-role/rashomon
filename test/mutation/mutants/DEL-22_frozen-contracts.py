# DEL-22: P-07's contracts are frozen by test. The steer.PatternID set and the
# gate Reason vocabulary are the two lists the row names; a dropped id, a
# dropped reason or two reasons sharing a string is the whole regression.
m("DEL-22 the frozen pattern set loses masked-status", "internal/steer/pattern/pattern.go",
  "\t\tClaimVsRecord, MaskedStatus, SequenceRisk, TaskPreflight, StaleEvidence,\n",
  "\t\tClaimVsRecord, SequenceRisk, TaskPreflight, StaleEvidence,\n", "TestDEL22_")
m("DEL-22 a gate reason is dropped from the vocabulary", "internal/verify/state.go",
  "\tBudget, OK,\n}", "\tOK,\n}", "TestDEL22_")
# Two gate reasons sharing a string is no longer a mutant: reasonState in
# internal/verify/state.go is a map literal keyed by every Reason, so a
# collision is a duplicate key and the package does not compile. The
# compiler now refuses the regression this mutant named.
m("DEL-22 the session record's resume key is respelled", "internal/store/record_v5.go",
  "`json:\"resume\"`", "`json:\"resumed\"`", "TestDEL22b_")
# The target's dropped keys becoming a count again is no longer a mutant:
# rec31_test.go builds a target with Dropped as a slice and appends to it, so
# the type change stops the store's tests compiling, the package
# TestDEL22b_SchemaFiveKeyListsAreFrozen lives in, and the sweep can only
# report it as a broken build. The compiler now refuses the regression.
# The pattern interface proposal, the nullable steer-line prompt_id and
# Finding.Check (cross F7, F8(d); P07N-2, P07N-14).
m("DEL-22 the pattern interface's Revalidate is renamed", "internal/steer/pattern_iface.go",
  "\tRevalidate(ctx *Context, f Finding) bool\n", "\tRecheck(ctx *Context, f Finding) bool\n", "TestDEL22c_")
m("DEL-22 a steer line drops prompt_id when it has none", "internal/steer/line.go",
  "\tPromptID *string `json:\"prompt_id\"`\n", "\tPromptID *string `json:\"prompt_id,omitempty\"`\n", "TestDEL22d_")
m("DEL-22 any kind passes as a frozen kind", "internal/steer/pattern/pattern.go",
  "\treturn slices.Contains(kinds()[id], kind)\n}\n\n// classForm", "\treturn true\n}\n\n// classForm", "TestDEL22e_")
m("DEL-22 the masked claim may be model-visible", "internal/steer/finding.go",
  '\treturn f.Pattern == pattern.ClaimVsRecord && f.Kind == "masked"\n', "\treturn false\n", "TestDEL22e_")
m("DEL-22 a message rides another pattern's finding", "internal/steer/finding.go",
  "\tcase f.Message.Text() != \"\" && f.Message.Pattern() != f.Pattern:\n", "\tcase false:\n", "TestDEL22e_")
m("DEL-22 evidence may repeat or follow after_seq", "internal/steer/finding.go",
  "\t\tif seq < 1 || seq > f.AfterSeq || i > 0 && seq <= f.Evidence[i-1] {\n", "\t\tif seq < 1 || i > 0 && seq < f.Evidence[i-1] {\n", "TestDEL22e_")
m("DEL-22 a stop form rides below the stop level", "internal/steer/finding.go",
  "\tcase f.StopForm != StopFormNone && f.MaxLevel != LevelStop:\n", "\tcase false:\n", "TestDEL22e_")
# The schema-5 key list against every route around it (P07N-6), BREADTH's
# call-record keys (cross F4) and the withheld keys (STORE-9).
m("DEL-22 omitzero drops a v5 key when it is null", "internal/store/record_v5.go",
  "`json:\"written_paths_complete\"`", "`json:\"written_paths_complete,omitzero\"`", "TestDEL22b_")
m("DEL-22 a tag option changes a v5 key's wire type", "internal/store/record_v5.go",
  "\tSize       int      `json:\"size\"`\n", "\tSize       int      `json:\"size,string\"`\n", "TestDEL22b_")
m("DEL-22 a pointer-receiver MarshalJSON rewrites the session record", "internal/store/record_v5.go",
  "type Session struct {\n", "func (s *Session) MarshalJSON() ([]byte, error) { return []byte(`{\"x\":1}`), nil }\n\ntype Session struct {\n", "TestDEL22b_")
m("DEL-22 a MarshalJSON promoted from the header rewrites every record", "internal/store/record_v5.go",
  "type EventHeader struct {\n", "func (h *EventHeader) MarshalJSON() ([]byte, error) { return []byte(`{\"only\":\"header\"}`), nil }\n\ntype EventHeader struct {\n", "TestDEL22b_")
m("DEL-22 a named embed nests the header", "internal/store/record_v5.go",
  "type Prompt struct {\n\tEventHeader\n", "type Prompt struct {\n\tEventHeader `json:\"header\"`\n", "TestDEL22b_")
m("DEL-22 a MarshalText writes the git state as one string", "internal/store/record_v5.go",
  "type GitState struct {\n", "func (g GitState) MarshalText() ([]byte, error) { return []byte(\"opaque\"), nil }\n\ntype GitState struct {\n", "TestDEL22b_")
m("DEL-22 a list key becomes a scalar", "internal/store/record_v5.go",
  "\tToolUseIDs []string `json:\"tool_use_ids\"`\n", "\tToolUseIDs string   `json:\"tool_use_ids\"`\n", "TestDEL22b_")
m("DEL-22 written_paths withheld as oversize has no key", "internal/store/record_v5.go",
  "\tWrittenPathsWithheld    *string `json:\"written_paths_withheld\"`", "\tWrittenPathsWithheld    *string `json:\"-\"`", "TestDEL22b_")
m("DEL-22 BREADTH's mcp_server_source key is absent", "internal/store/record_v5.go",
  "\tMCPServerSource *string `json:\"mcp_server_source\"`\n", "", "TestDEL22b_")
# Route admits only findings that pass Check (P07N-14).
m("DEL-22 Route routes findings that skipped Check", "internal/steer/route.go",
  "\treturn route(ctx, admit(ctx, findings))\n", "\treturn route(ctx, findings)\n", "TestDEL22f_")
m("DEL-22 admit keeps a finding that fails Check", "internal/steer/route.go",
  "\t\tif f.CheckOn(event) == nil {\n", "\t\tif f.CheckOn(event) == nil || true {\n", "TestDEL22f_")
# A withheld key beside every field a cap or a switch can null (STORE-9).
m("DEL-22 a switched-off task subject reads as not applicable", "internal/store/record_v5.go",
  "\tSubjectWithheld  *string `json:\"subject_withheld\"`   // switched_off (task_subjects) | masker_error\n", "", "TestDEL22g_")
m("DEL-22 prompt_class, off by default, has no withheld key", "internal/store/record_v5.go",
  "`json:\"prompt_class_withheld\"`", "`json:\"-\"`", "TestDEL22g_")
m("DEL-22 the session's git switched off has no key", "internal/store/record_v5.go",
  "\tGitWithheld       *string `json:\"git_withheld\"`        // switched_off (git)\n", "", "TestDEL22g_")
m("DEL-22 an oversize instructions path cannot be null", "internal/store/record_v5.go",
  "\tFilePath        *string `json:\"file_path\"`\n", "\tFilePath        string  `json:\"file_path\"`\n", "TestDEL22g_")
# The withheld root is set by the cap (limits.go), so deleting the field no
# longer compiles; the key goes missing from the wire instead, as
# written_paths_withheld's does above.
m("DEL-22 an oversize git root has no key", "internal/store/record_v5.go",
  "\tRootWithheld *string `json:\"root_withheld\"`\n", "\tRootWithheld *string `json:\"-\"`\n", "TestDEL22g_")
m("DEL-22 a finding may name an after_seq below 1", "internal/steer/finding.go",
  "\tif f.AfterSeq < floor {\n\t\treturn errAfterSeq\n\t}\n",
  "", "TestDEL22e_")
m("DEL-22 an evidence seq of zero or less is accepted", "internal/steer/finding.go",
  "\t\tif seq < 1 || seq > f.AfterSeq ", "\t\tif seq > f.AfterSeq ", "TestDEL22e_")
m("DEL-22 a finding's priority is never checked against its pattern", "internal/steer/finding.go",
  "\tif p, ok := pattern.Priority(f.Pattern); !ok || f.Priority != p {\n\t\treturn fmt.Errorf(\"%w: %s does not carry its own-channel or table priority\", ErrFinding, f.Pattern)\n\t}\n",
  "", "TestDEL22e_")
m("DEL-22 write_observed's path cannot be withheld", "internal/store/record_v5.go",
  "\tPath         *string `json:\"path\"`\n\tEditCount    int     `json:\"edit_count\"`\n\tPathWithheld *string `json:\"path_withheld\"` // oversize | switched_off (paths)\n",
  "\tPath         string `json:\"path\"`\n\tEditCount    int    `json:\"edit_count\"`\n", "TestDEL22g_")
