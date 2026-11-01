# PF-1: the request's preflight (PREFLIGHT1). The prompt is read in memory
# and never kept; the note names only what the project holds, counts what an
# untrusted spec says, steps down to fit, and is new to the session or not
# given. Each break below is one of those promises undone.
m("PF-1 the intent keeps every named token as a path", "internal/intent/intent.go",
  "\tin.Paths = resolvePaths(named, p)\n", "\tin.Paths = named\n", "TestPF1d_")
m("PF-1 the delivered set is ignored", "internal/intent/note.go",
  "\tfirst := !d.BriefGiven && b.has()\n", "\td = Delivered{}\n\tfirst := !d.BriefGiven && b.has()\n", "TestPF1b_")
m("PF-1 a request naming nothing gets a note", "internal/intent/note.go",
  "\tif !first && (!in.Named || !fresh && !in.Truncated) {\n", "\tif !first && !fresh && !in.Truncated {\n",
  "TestPF1c_AmbiguousPromptGetsNothingNew")
m("PF-1 the note never steps down to fit", "internal/intent/note.go",
  "\tfor step := range 3 {\n", "\tfor step := range 1 {\n", "TestPF1g_")
m("PF-1 an untrusted spec's rule is named", "internal/intent/intent.go",
  "\t\t\tif !trusted(t) {\n\t\t\t\tin.Withheld++\n\t\t\t\tcontinue\n\t\t\t}\n", "", "TestPF1e_")
m("PF-1 an untrusted spec's rules are not counted", "internal/intent/intent.go",
  "\t\t\t\tin.Withheld++\n", "", "TestPF1e_")
m("PF-1 a test family no session made required is counted as learned", "internal/intent/intent.go",
  "e.Kind == \"test\" && p.Index.Required(e.Family, p.NowMS)", "e.Kind == \"test\"", "TestPF1_RequestNaming")
m("PF-1 a lookup leaves the project root", "internal/intent/intent.go",
  "\t\tlstat = func(rel string) (fs.FileInfo, error) { return r.Lstat(filepath.FromSlash(rel)) }\n",
  "\t\tlstat = func(rel string) (fs.FileInfo, error) { return os.Stat(filepath.Join(p.Root, filepath.FromSlash(rel))) }\n",
  "TestPF1j_PathTokensAreBoundedAndStayUnderTheRoot")
m("PF-1 a request the reader cut reads as whole", "internal/intent/intent.go",
  "\tin := Intent{Truncated: cut || len(prompt) > MaxPromptBytes}\n",
  "\tin := Intent{Truncated: len(prompt) > MaxPromptBytes}\n", "TestPF1j_Over16KiB")
m("PF-1 Names reads every request as naming nothing", "internal/intent/intent.go",
  "\tscan(prompt, cut, func(tok []byte) { named = named || pathLike(tok) || identifierLike(tok) })\n", "",
  "TestPF1c_NamesIsTheCatchUpsCheapGuard")
m("PF-1 the first prompt's brief is never given", "internal/intent/note.go",
  "\tfirst := !d.BriefGiven && b.has()\n", "\tfirst := false\n", "TestPF1i_")
m("PF-1 CheckOn admits after_seq 0 on every event", "internal/steer/finding.go",
  "\tif event != EventUserPromptSubmit || f.AfterSeq != 0 {\n", "\tif f.AfterSeq != 0 {\n",
  "TestPF1t_SeqZeroIsAdmittedOnAPromptOnly")
m("PF-1 the steer line cannot record a preflight slot", "internal/steer/slots.go",
  "\t\tcase slot.PathSet, slot.RuleRefs, slot.GateRefs, slot.EnumList:\n\t\t\terr = putJSON(put, s.Name, preflightJSON(v))\n",
  "", "TestPF1r_PreflightNoteIsMirroredAcrossItsLines|TestSlotsOf_EverySlotTypeHasARow")
m("PF-1 ** matches within one segment", "internal/spec/match.go",
  "\t\tif head != \"**\" {\n", "\t\tif true {\n", "TestPF1l_MatchPathClauses")
m("PF-1 a rule covers a path its ExcludePaths name", "internal/spec/match.go",
  "\tfor _, p := range r.ExcludePaths {\n\t\tif MatchPath(p, path) {\n\t\t\treturn false\n\t\t}\n\t}\n", "", "TestPF1l_CoversHonoursExcludePaths")
m("SR1a MatchPath splits the path into a slice again", "internal/spec/match.go",
  "\tif !cleanSegments(rel) {\n\t\treturn false\n\t}\n\tanchored",
  "\tif !cleanSegments(rel) {\n\t\treturn false\n\t}\n\t_ = strings.Split(rel, \"/\")\n\tanchored",
  "TestSR1a_MatchPathAllocatesNothing")
m("PF-1 the catch-up asks a nil run for its last seq", "cmd/rashomon/preflight.go",
  "\tif run == nil || p.CWD == \"\" {\n",
  "\tif p.CWD == \"\" {\n", "TestPF1u_")
m("PF-1 clearPrompt drops the prompt without zeroing it", "cmd/rashomon/preflight.go",
  "\tclear(p.Prompt)\n\tp.Prompt = nil\n", "\tp.Prompt = nil\n", "TestPF1k_")
m("PF-1 the screen still says a prompt's text is not examined", "internal/disclose/paragraphs.go",
  "Your prompts are never stored: rashomon reads each in memory to pick project facts for a note and keeps only, in the note's steer line, the project paths it names at record scope v5, or only their count and keyed digests at v4 (`rashomon status` shows the scope).",
  "Your prompts are never stored; rashomon records only each prompt's length and does not examine its text.",
  "TestPF1n_")
m("PF-1 task-preflight ranks below learned-fact", "internal/steer/pattern/pattern.go",
  "\t\t\tTaskPreflight:      72,\n", "\t\t\tTaskPreflight:      20,\n", "TestPF1p_|TestPriority_StepEightValues")
m("PF-1 the steer environment is built before the Names guard", "cmd/rashomon/preflight.go",
  "\tif !named && !proj.lessons.Any() && p.owed.Seq <= 0 {\n\t\treturn steer.Output{}, steer.Client{}\n\t}\n\tenv := steerEnv(id, now, st.Root())\n",
  "\tenv := steerEnv(id, now, st.Root())\n\tif !named && !proj.lessons.Any() && p.owed.Seq <= 0 {\n\t\treturn steer.Output{}, steer.Client{}\n\t}\n",
  "TestPF1v_")
