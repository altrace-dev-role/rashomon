# FD-5.4: the call path (PRD PF-FD2; card P5B, 5.4, D-B1 to D-B3). 5.4 adds no
# production code, so each break removes a property the tree already has:
# one goroutine per call, the barrier that keeps a panic at exit 0, and the
# cap before the parse. The kill row is the named test's.
C = "cmd/rashomon/post.go"
MAIN = "cmd/rashomon/main.go"
RD = "internal/hookio/read.go"

m("FD-5.4 the steer step runs on a second goroutine", C,
  "\t\tif b := p.Steer(steerEnv(id, time.Now(), st.Root())); len(b) > 0 {\n",
  "\t\tvar b []byte\n\t\t<-safe.Go(func() { b = p.Steer(steerEnv(id, time.Now(), st.Root())) }, nil)\n\t\tif len(b) > 0 {\n",
  "TestFD2a_")
m("FD-5.4 no barrier over the entry", MAIN,
  "func guardedHook(stderr io.Writer, fn func() error) int {\n\tif err := safe.Guard(fn); err != nil {\n",
  "func guardedHook(stderr io.Writer, fn func() error) int {\n\tif err := fn(); err != nil {\n", "TestFD2b_")
m("FD-5.4 the read is not clamped to the cap", RD,
  "\t\tif room := MaxPayloadBytes + 1 - total; size > room {\n", "\t\tif room := MaxPayloadBytes + 1 - total; size > room && false {\n",
  "TestFD2c_")
