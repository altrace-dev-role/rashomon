# WM-1.9: the M1 guards that W-M1.7, W-M1.8 and W-M1.9 added (PLAN.md
# section 6.1). A finding's source and unit are checked before routing, a
# finding's unit is the unit reached from its first node, a worktree's git
# directories resolve through the pointer and the common directory, a
# packed ref reads through a worktree, the hook's evidence file holds no
# word of the prompt (D160), and an imported row keeps only a validating
# clause and only the paths the index holds. Each break below is one of
# those promises undone.
F = "internal/steer/finding.go"
m("WM-1.9 a finding's source is not checked", F,
  "\tif err := f.Source.check(); err != nil {\n\t\treturn err\n\t}\n", "\t_ = f.Source.check()\n",
  "TestFinding_Schema")
m("WM-1.9 a finding's unit is not checked", F,
  "\tif f.Unit != \"\" && !isGlobal(f.Unit, \"unit:\") {\n", "\tif false {\n",
  "TestFinding_Schema")
G = "cmd/rashomon/graph.go"
m("WM-1.9 the unit reached is not returned", G,
  "\t\tif n.Kind == \"unit\" {\n\t\t\treturn id\n\t\t}\n", "\t\tif n.Kind == \"unit\" {\n\t\t\treturn \"\"\n\t\t}\n",
  "TestUnitOf_WalksContainingDeclaringAndAppliesToEdges")
m("WM-1.9 the unit walk stops after two nodes", G,
  "\tfor step := 0; step < 4 && id != \"\"; step++ {\n", "\tfor step := 0; step < 2 && id != \"\"; step++ {\n",
  "TestUnitOf_WalksContainingDeclaringAndAppliesToEdges")
m("WM-1.9 a lesson's applies_to edge is not followed", G,
  "\t\t} else if out := s.Out(id, rev, graph.AppliesTo); len(out) > 0 {\n\t\t\tnext = out[0].Dst\n\t\t}\n", "\t\t}\n",
  "TestUnitOf_WalksContainingDeclaringAndAppliesToEdges")
m("WM-1.9 a packed ref is not read", G,
  "\t\t} else {\n\t\t\th = packedRef(filepath.Join(commonDir, \"packed-refs\"), ref)\n\t\t}\n", "\t\t}\n",
  "TestHeadCommit_ReadsThroughAWorktree")
m("WM-1.9 refs are read under the worktree's own directory", G,
  "\tgitDir, commonDir, ok := baseline.GitDirs(root)\n\tif !ok {\n\t\treturn \"C1\"\n\t}\n",
  "\tgitDir, commonDir, ok := baseline.GitDirs(root)\n\tif !ok {\n\t\treturn \"C1\"\n\t}\n\tcommonDir = gitDir\n",
  "TestHeadCommit_ReadsThroughAWorktree")
m("WM-1.9 the hook's evidence file holds the prompt", G,
  "\ttext := b.Evidence() + \"\\nbrief:\\n\" + b.Text() + \"\\n\"\n", "\ttext := b.Explain() + \"\\nbrief:\\n\" + b.Text() + \"\\n\"\n",
  "TestGraphBrief_EvidenceFileHoldsNoWordOfThePrompt")
m("WM-1.9 Evidence names the matched words", "internal/graph/engine/brief.go",
  "\treturn fmt.Sprintf(\"scorer: %s  tokens: %d\\n\", b.Scorer, len(b.Tokens)) + b.tables(false)\n",
  "\treturn fmt.Sprintf(\"scorer: %s  tokens: %d\\n\", b.Scorer, len(b.Tokens)) + b.tables(true)\n",
  "TestEvidence_HoldsNoWordOfThePrompt")
R = "internal/baseline/repoid.go"
m("WM-1.9 a .git file that is no pointer is taken as one", R,
  "\t\trest, ok := strings.CutPrefix(strings.TrimSpace(string(b)), \"gitdir: \")\n\t\tif !ok {\n\t\t\treturn \"\", \"\", false\n\t\t}\n",
  "\t\trest := strings.TrimSpace(string(b))\n",
  "TestGitDirs_ResolvesThePointerAndTheCommonDir")
m("WM-1.9 a relative gitdir pointer is not joined to the root", R,
  "\t\tif !filepath.IsAbs(dir) {\n\t\t\tdir = filepath.Join(root, dir)\n\t\t}\n", "",
  "TestGitDirs_ResolvesThePointerAndTheCommonDir")
m("WM-1.9 a relative commondir is not joined to the git directory", R,
  "\t\tif !filepath.IsAbs(common) {\n\t\t\tcommon = filepath.Join(dir, common)\n\t\t}\n", "",
  "TestGitDirs_ResolvesThePointerAndTheCommonDir")
K = "internal/graph/knowledge/import.go"
m("WM-1.9 a refused clause is written as the row's clause", K,
  "\t\tr.Clause = c\n", "\t\tif c == \"\" {\n\t\t\tc = r.Clause\n\t\t}\n\t\tr.Clause = c\n",
  "TestImport_ClauseValidates")
m("WM-1.9 a path the index lacks resolves anyway", K,
  "\t\tif id, ok := r.lookup(p); ok {\n\t\t\tids[id] = true\n\t\t} else {\n\t\t\tdropped[p] = true\n\t\t}\n",
  "\t\tif id, ok := r.lookup(p); ok {\n\t\t\tids[id] = true\n\t\t} else {\n\t\t\tids[graph.FileID(p)] = true\n\t\t\tdropped[p] = true\n\t\t}\n",
  "TestImport_AppliesToResolved")
