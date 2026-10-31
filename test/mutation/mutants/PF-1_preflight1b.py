# PF-1 (PREFLIGHT1b): the preflight's steer line follows the record scope,
# requests resolve against the project, a delivered note prints before a
# stopView error returns, and attendance on the prompt path follows the
# surface. Each break below is one of those promises undone.
m("PF-1 the note names no stop form", "internal/intent/note.go",
  "\t\t\tStopForm: steer.StopFormNone, Message: m, Record: rec, AfterSeq: afterSeq}, true",
  "\t\t\tMessage: m, Record: rec, AfterSeq: afterSeq}, true",
  "TestPF1t_|TestPF1b_|TestPF1_")
m("PF-1 the v4 slot is the path, not its digest", "internal/intent/note.go",
  "\t\t\tds[i] = digest(string(p))\n", "\t\t\tds[i] = string(p)\n",
  "TestPF1w_")
m("PF-1 the slot digest is unkeyed", "internal/intent/digest.go",
  "\tm := hmac.New(sha256.New, key)\n", "\tm := hmac.New(sha256.New, nil)\n",
  "TestPF1w_DigestIsKeyed")
m("PF-1 the v4 record form is not applied to the path set", "internal/intent/note.go",
  "\tpf.Paths = keyed(pf.Paths)\n", "",
  "TestPF1w_")
m("PF-1 the delivered set at v4 compares plain paths", "internal/intent/note.go",
  "\t\t\treturn d.Paths[digest(p)]\n", "\t\t\treturn d.Paths[p]\n",
  "TestPF1w_V4DeliveredSetDedupes")
m("PF-1 the steer line records the printed message, not its record form", "internal/steer/decide.go",
  "\t\tslots, slotErr := slotsOf(f.recorded())\n", "\t\tslots, slotErr := slotsOf(f.Message)\n",
  "TestPF1w_")
m("PF-1 the steer line's text is the printed text, not its record form", "internal/steer/decide.go",
  "\t\t\tl.ModelText, l.UserText = p.recModel, p.recUser\n", "",
  "TestPF1w_")
m("PF-1 the line is recorded in the clear at v4", "cmd/rashomon/preflight.go",
  "\t\tif scope != store.RecordScopeV5 {\n", "\t\tif false {\n",
  "TestPF1w_")
m("PF-1 the line's project is filed at v4", "cmd/rashomon/preflight.go",
  "\tif scope == store.RecordScopeV5 {\n\t\tproject = ", "\tif true {\n\t\tproject = ",
  "TestPF1w_V4LineKeepsNoPath")
m("PF-1 the project resolves against the cwd", "cmd/rashomon/preflight.go",
  "intent.Project{Root: root, Cwd: cwd, Spec: sp,", "intent.Project{Root: p.CWD, Cwd: cwd, Spec: sp,",
  "TestPF1x_")
m("PF-1 a path named from the cwd is not tried from the cwd", "internal/intent/intent.go",
  "ok && cwdRel != \"\" {\n", "ok && cwdRel != \"\" && false {\n",
  "TestPF1x_")
m("PF-1 the note does not print before a stopView error returns", "cmd/rashomon/recap_turn.go",
  "\t\tif note != (steer.Output{}) {\n\t\t\t_ = printTurn(st, stdout, note, client, \"\")\n\t\t}\n\t\treturn serr\n",
  "\t\treturn serr\n",
  "TestPF1y_")
m("PF-1 the screen drops the v4 clause", "internal/disclose/paragraphs.go",
  ", or only their count and keyed digests at v4 (`rashomon status` shows the scope)", "",
  "TestPF1n_")
m("PF-1 the prompt path reads CI for attendance again", "internal/hook/promptcontext.go",
  "\t\tunattended = env.Surface == string(harness.SurfaceSDK) || env.Surface == string(harness.SurfaceCloud)\n",
  "\t\tunattended = env.CI || env.Surface == string(harness.SurfaceSDK) || env.Surface == string(harness.SurfaceCloud)\n",
  "TestPF1z_")
m("PF-1 the prompt path counts sdk-cli as attended", "internal/hook/promptcontext.go",
  "\t\tunattended = env.Surface == string(harness.SurfaceSDK) || env.Surface == string(harness.SurfaceCloud)\n",
  "\t\tunattended = env.Surface == string(harness.SurfaceCloud)\n",
  "TestPF1z_")
m("PF-1 the preflight reads the spec with the store root directly", "cmd/rashomon/preflight.go",
  "hook.TrustRoot(run, st.Root(), run.LastSeq())", "st.Root()",
  "TestST1x_")
m("PF-1 the stored paragraph claims every note word for word", "internal/disclose/paragraphs.go",
  "Each note it adds, word for word, except a request's preflight note at record scope v4, which keeps a count of the paths it named and keyed digests of them.", "Each note it adds, word for word.",
  "TestTRU_PF1b_")
m("PF-1 steer.ndjson claims the text shown at every scope", "internal/disclose/disclose.go",
  " (except a request's preflight note at record scope v4, which keeps a count of the paths it named and keyed digests of them)", "",
  "TestTRU_PF1b_")
