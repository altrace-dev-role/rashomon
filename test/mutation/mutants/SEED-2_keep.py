# SEED-2 (card SEED1-KEEP, PF-HD8 and PF-HD17's typed rows): `rashomon audit
# --keep` counts the days before the install, per project, keeps the count in
# the store by stable ids, only ever grows it, and refuses in an agent session;
# the brief says it; purge takes it.
S = "internal/seed/seed.go"
P = "internal/seed/kept.go"
A = "cmd/rashomon/audit.go"
B = "internal/brief/brief.go"
G = "internal/purge/purge.go"

# The window.
m("SEED-2 a line after the install counts", S,
  "\t\tif !t.Until.IsZero() && !before(l.Timestamp, t.Until) {\n",
  "\t\tif false && !before(l.Timestamp, t.Until) {\n", "TestSEED2a_")
m("SEED-2 a line with no timestamp counts as before", S,
  "\treturn err == nil && at.Before(until)\n", "\treturn err != nil || at.Before(until)\n", "TestSEED2a_")
m("SEED-2 --keep counts after the install", A,
  "\tt.Until, t.Key = until, baseline.Key\n", "\tt.Key = baseline.Key\n", "TestSEED2c_")

# The projects.
m("SEED-2 every transcript is one project", S,
  "\t\t\tf.pt, f.keyed = t.project(t.Key(l.Cwd)), true\n",
  "\t\t\tf.pt, f.keyed = t.project(\"one\"), true\n", "TestSEED2a_")
m("SEED-2 a project's corrections are not counted", S,
  "\tif f.pt != nil {\n\t\tf.pt.add(k, c, session)\n\t}\n", "", "TestSEED2a_")
m("SEED-2 a project's sessions are not counted", S,
  "\tif f.pt != nil {\n\t\tf.pt.saw(l.SessionID, f.project)\n\t}\n", "", "TestSEED2a_")

# The record.
m("SEED-2 kept rows are not in the card's order", P,
  "\t\t\tstrings.Compare(rowText(a.k, a.c), rowText(b.k, b.c)))\n",
  "\t\t\tstrings.Compare(rowText(b.k, b.c), rowText(a.k, a.c)))\n", "TestSEED2a_")
m("SEED-2 a kind's id is not stable", P,
  "pushedBack: \"pushed-back\"", "pushedBack: \"pushed back\"", "TestSEED2a_")
m("SEED-2 a record over the cap is read", P,
  "\tif len(b) > KeptMaxBytes {\n\t\treturn Kept{}, false, fmt.Errorf(\"seed: %s is over",
  "\tif false {\n\t\treturn Kept{}, false, fmt.Errorf(\"seed: %s is over", "TestSEED2b_")
m("SEED-2 a record of another version is read", P,
  "\tif k.Version != keptVersion {\n", "\tif false {\n", "TestSEED2b_")

# Only grows; never in an agent session; never a count of nothing.
m("SEED-2 a smaller recount replaces the kept count", A,
  "\tkept := s.Sessions > 0 && (!had || s.Sessions >= old.All.Sessions)\n",
  "\tkept := s.Sessions > 0\n", "TestSEED2c_")
m("SEED-2 a count of no sessions is kept", A,
  "\tkept := s.Sessions > 0 && (!had || s.Sessions >= old.All.Sessions)\n",
  "\tkept := !had || s.Sessions >= old.All.Sessions\n", "TestSEED2c_")
m("SEED-2 --keep runs in an agent session", A,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := selfguard.AgentSignal(os.Getenv); false && s != \"\" {\n", "TestSEED2c_")

# The brief's words.
m("SEED-2 a rule's block counts as a correction", P,
  "\treturn c.Kinds[interrupted.id()] + c.Kinds[rejected.id()] + c.Kinds[pushedBack.id()]\n",
  "\treturn c.Kinds[interrupted.id()] + c.Kinds[rejected.id()] + c.Kinds[pushedBack.id()] + c.Kinds[blocked.id()]\n",
  "TestSEED2e_")
m("SEED-2 a row seen in one session is the most", P,
  "\t\tif okKind && okClass && r.Sessions >= cardRecurAt {\n",
  "\t\tif okKind && okClass {\n", "TestSEED2e_")
m("SEED-2 the project's row is not preferred", P,
  "\t\tif r, ok := recurring(p.Rows); ok {\n", "\t\tif r, ok := recurring(p.Rows); false && ok {\n", "TestSEED2e_")
m("SEED-2 the brief has no history line", B,
  "\tif storeRoot != \"\" {\n\t\tif f, ok := historyFact(root, storeRoot); ok {\n",
  "\tif false {\n\t\tif f, ok := historyFact(root, storeRoot); ok {\n", "TestSEED2e_")
m("SEED-2 the brief has a history line with nothing kept", B,
  "\tcase !ok:\n\t\treturn Fact{}, false\n", "", "TestSEED2e_")
m("SEED-2 an unreadable record is passed over in silence", B,
  "\tcase err != nil:\n\t\tf := newFact(KindHistory", "\tcase false:\n\t\tf := newFact(KindHistory", "TestSEED2e_")

# Purge.
m("SEED-2 purge leaves the kept count", G,
  "\tseed.KeptFile,\n", "", "TestSEED2d_")
m("SEED-2 purge leaves the kept count's temporary file", G,
  "\t\".\" + seed.KeptFile + \".tmp-\",\n", "", "TestSEED2d_")
