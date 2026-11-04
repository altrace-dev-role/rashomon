# BGO-2: a done claim answers to the evidence since the last edit in the
# gates' projects (card BG-OUTCOME phase 2a, the manager's ruling of 11-03):
# the done flag reads the window from the turn holding that edit through
# the Stop, never its turn alone; an edit no gate of the turn covers opens no
# window (card F2's offTurn, the ruling of 11-03 09:3x); and a wrapper's call
# launched in the background reads unknown, naming the call as one that may
# have run the gate, with no clause about its output. Each break removes one
# rule and names the tests that hold it.
RD = "internal/recap/done.go"
VS = "internal/verify/stop.go"
VD = "internal/verify/declared.go"
VC = "internal/verify/cover.go"
VN = "internal/verify/snapshot.go"
m("BGO-2 the done flag reads its turn alone", RD,
  "\tif v.Done != nil {\n", "\tif false {\n", "TestBGO2a_|TestBGO2c_|TestBGO2d_")
m("BGO-2 any edit opens the window", VS,
  "w.Seq <= at && w.Seq > last && coveredByAny(roots, w) {", "w.Seq <= at && w.Seq > last {", "TestBGO2c_")
m("BGO-2 a line run forgets its background launch", VD,
  "wrapper: wrapper, background: e.Backgrounded})", "wrapper: wrapper})", "TestBGO2a_|TestBGO2c_")
m("BGO-2 a background wrapper reads unread", VD,
  "\t\tif unread.background {\n", "\t\tif false {\n", "TestBGO2a_|TestBGO2c_|TestWR1_AnUnreadWrapperAfterTheLastEditIsUnknown")
m("BGO-2 the window opens at an off-turn project's edit", VC,
  "if g := &snap.Gates[i]; !g.offTurn && !slices.Contains(roots, g.root) {",
  "if g := &snap.Gates[i]; !slices.Contains(roots, g.root) {", "TestBGO2e_")
m("BGO-2 the background wrap word reads back none", VN,
  "\t\t*w = WrapBackground\n", "\t\t*w = WrapNone\n", "TestWR1_TheWrapWordKeepsItsJSON")
