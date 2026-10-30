# Card 4.1b: the body store's CLI (PF-RC14). bodies on lists this project
# and off removes it alone; status counts copies, never a temporary file;
# forget takes one kind of three; the end probe says when a transcript was
# over the whole cap. Each guard is broken once (R2, R2b).
CB = "cmd/rashomon/bodies.go"
MG = "cmd/rashomon/main.go"
CF = "cmd/rashomon/forget.go"
HP = "internal/hook/probe.go"
SB = "internal/store/bodies.go"

m("ST-4.1b off removes every project", CB,
  '\t\tset.Projects = slices.DeleteFunc(set.Projects, func(p string) bool { return p == key })\n',
  '\t\tset.Projects = slices.DeleteFunc(set.Projects, func(p string) bool { return true })\n',
  "TestRC14i_BodiesOnOffStatus")
m("ST-4.1b on lists the project twice", CB,
  '\t\tif !slices.Contains(set.Projects, key) {\n\t\t\tset.Projects = append(set.Projects, key)\n\t\t}\n',
  '\t\tset.Projects = append(set.Projects, key)\n',
  "TestRC14i_BodiesOnOffStatus")
m("ST-4.1b --max-bytes is ignored", CB,
  '\t\tset.MaxBytes = cmp.Or(maxBytes, set.MaxBytes, store.DefaultBodiesMaxBytes)\n',
  '\t\tset.MaxBytes = cmp.Or(set.MaxBytes, store.DefaultBodiesMaxBytes)\n',
  "TestRC14i_BodiesOnOffStatus")
m("ST-4.1b status counts the temporary files", SB,
  ' || path == except || !strings.HasSuffix(e.Name(), ".jsonl") {\n',
  ' || path == except || !strings.HasSuffix(e.Name(), ".jsonl") && !strings.HasPrefix(e.Name(), BodiesTempPrefix) {\n',
  "TestRC14i_BodiesOnOffStatus")
m("ST-4.1b the kind count accepts two", CF,
  '\tcase forgetKinds(hostArg != "", from != nil || to != nil, bodies) > 1:\n',
  '\tcase forgetKinds(hostArg != "", from != nil || to != nil, bodies) > 2:\n',
  "TestRC14j_ForgetOneKindOnly")
m("ST-4.1b forget --bodies takes another project's copies", CB,
  '\tn, err := st.ForgetBodies(dir)\n',
  '\tn, err := st.ForgetBodies("other-project")\n',
  "TestRC14j_ForgetOneKindOnly")
m("ST-4.1b status has no bodies line", "cmd/rashomon/verbs_status.go",
  '\t\tfmt.Fprintf(stdout, "  %s\\n", bodiesLine(st))\n',
  '',
  "TestRC14k_StatusLineBesideStoreSize")
m("ST-4.1b the over-cap line is dropped", MG,
  '\t\tif errors.Is(runErr, store.ErrBodyOverCap) {\n\t\t\tfmt.Fprintln(stderr, bodyOverCapLine)\n\t\t}\n',
  '',
  "TestRC14l_OverCapPrintsOneLine")
m("ST-4.1b the probe drops the over-cap refusal", HP,
  '\t\tif errors.Is(err, store.ErrBodyOverCap) {\n\t\t\toverCap = err\n\t\t}\n',
  '',
  "TestRC14l_OverCapPrintsOneLine")
