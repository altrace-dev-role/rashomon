# BGO-2: a done claim answers to the evidence since the last edit in the
# gates' projects (card BG-OUTCOME phase 2a, the manager's ruling of 11-03):
# the done flag reads the window from the turn holding that edit through
# the Stop, never its turn alone; an edit no gate covers opens no window; and
# a wrapper's call launched in the background reads unknown, naming the call
# with no clause about its output. Each break removes one rule and names the
# tests that hold it.
RD = "internal/recap/done.go"
VS = "internal/verify/stop.go"
VD = "internal/verify/declared.go"
m("BGO-2 the done flag reads its turn alone", RD,
  "\tif v.Done != nil {\n", "\tif false {\n", "TestBGO2a_|TestBGO2c_|TestBGO2d_")
m("BGO-2 any edit opens the window", VS,
  "w.Seq <= at && w.Seq > last && coveredByAny(roots, w) {", "w.Seq <= at && w.Seq > last {", "TestBGO2c_")
m("BGO-2 a line run forgets its background launch", VD,
  "wrapper: wrapper, background: e.Backgrounded})", "wrapper: wrapper})", "TestBGO2a_|TestBGO2c_")
m("BGO-2 a background wrapper reads unread", VD,
  "\t\tif unread.background {\n", "\t\tif false {\n", "TestBGO2a_|TestBGO2c_|TestWR1_AnUnreadWrapperAfterTheLastEditIsUnknown")
