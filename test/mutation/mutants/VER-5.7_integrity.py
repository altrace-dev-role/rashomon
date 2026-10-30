# VER-5.7: integrity's one-way doors at the post (PF-IV3; card P5B, 5.7, D139).
# One break per rule: the raise set by kind, this call's entries only, a test
# removed only after a red run, the facts reaching the view and their lines,
# the card row at the failure event, and the trigger (D-B7). The ceiling is
# held three times over (the card row, UserOnly, the pattern's userOnly), so
# a break of one alone is equivalent and has no row. The kill row is the
# named test's.
R = "internal/verify/integrity_raise.go"
V = "internal/verify/steer.go"
K = "internal/verify/tosteer_card.go"
T = "internal/steer/render/render.go"
H = "internal/hook/steerstep.go"
P = "internal/hook/post.go"
DOORS = "\tcase TestRemoved, GateWeakened, GateConfigEdited, StoreTouched:\n"

# The raise set.
m("VER-5.7 test_removed is no door", R, DOORS, "\tcase GateWeakened, GateConfigEdited, StoreTouched:\n", "TestIV3a_")
m("VER-5.7 gate_weakened is no door", R, DOORS, "\tcase TestRemoved, GateConfigEdited, StoreTouched:\n", "TestIV3a_")
m("VER-5.7 gate_config_edited is no door", R, DOORS, "\tcase TestRemoved, GateWeakened, StoreTouched:\n", "TestIV3a_")
m("VER-5.7 store_touched is no door", R, DOORS, "\tcase TestRemoved, GateWeakened, GateConfigEdited:\n", "TestIV3a_")
m("VER-5.7 skip_added is a door", R, DOORS, "\tcase TestRemoved, GateWeakened, GateConfigEdited, StoreTouched, SkipAdded:\n", "TestIV3a_")

# This call's entries; a test removed after a red run.
m("VER-5.7 another call's entry raises", R, "\t\tif e.Seq != seq || !oneWayDoor(e.Check)", "\t\tif !oneWayDoor(e.Check)", "TestIV3c_")
m("VER-5.7 a test removed with no red before it raises", R,
  " || e.Check == TestRemoved && !failedBefore(runs, seq) {\n", " {\n", "TestIV3c_")
m("VER-5.7 a red after the edit counts", R, "\t\tif runs[i].Seq >= seq {\n\t\t\tbreak\n\t\t}\n", "", "TestIV3c_")

# The facts reach the view, and their lines.
m("VER-5.7 SteerAt drops the raises", V,
  "\tfacts = append(facts, integrityRaises(run, snap, c.runs, event, toolUseID)...)\n", "", "TestIV3e_")
m("VER-5.7 an integrity fact renders as a gate fact", V, "\t\tcase f.Integrity != nil:\n", "\t\tcase false:\n", "TestIV3b_")
m("VER-5.7 the card drops the failure event", K,
  "event == post || event == string(steer.EventPostToolUseFailure) ||\n", "event == post ||\n", "TestIV3b_")
m("VER-5.7 no store-touched row", T,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityStoreTouchedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ X int }{})}: {\n", "TestIV3b_")
m("VER-5.7 no unnamed row", T,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityStoreTouchedUnnamedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ Y int }{})}: {\n", "TestIV3d_")
m("VER-5.7 no test-deleted row", T,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityTestRemovedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ Z int }{})}: {\n", "TestIV3b_")
m("VER-5.7 no gate-weakened row", T,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityGateWeakenedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ W int }{})}: {\n", "TestIV3b_")
m("VER-5.7 no gate-config-edited row", T,
  "\t\t\t{pattern.Integrity, reflect.TypeOf(IntegrityGateConfigEditedBinding{})}: {\n",
  "\t\t\t{pattern.Integrity, reflect.TypeOf(struct{ V int }{})}: {\n", "TestIV3b_")

# The trigger.
m("VER-5.7 a store call does not step", H,
  " || inStore && (failure || event == string(steer.EventPostToolUse)))\n", ")\n", "TestSTR58_")
m("VER-5.7 a failed store call does not step", H,
  "inStore && (failure || event == string(steer.EventPostToolUse))", "inStore && event == string(steer.EventPostToolUse)", "TestSTR58_")
m("VER-5.7 the plain store name is not scanned", H,
  '\treturn bytes.Contains(input, []byte(".claude/rashomon")) || ', "\treturn ", "TestSTR58_")
m("VER-5.7 the escaped store name is not scanned", H,
  " || bytes.Contains(input, []byte(`.claude\\/rashomon`))\n", "\n", "TestIV1a_")
m("VER-5.7 the post never reads the input", P, ", storeNamed(pl.ToolInput, p.st.Root())\n", ", false\n", "TestSTR58_")
