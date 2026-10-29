# VER-6.3: `rashomon backtest --sensor ID` (internal/backtest). One break per
# promise: a transcript's calls replay as the hooks saw them -- a denied or
# subagent call does not run -- each steer line counts once, a fire is
# matched to the live log's on its pattern and call, at most once, and the
# rows are the named sensor's. The kill row is the named test's.
T = "internal/backtest/transcript.go"
R = "internal/backtest/replay.go"
C = "cmd/rashomon/backtest.go"

m("VER-6.3 a denied call is replayed as run", T,
  "c.Done, c.IsError, c.Response, c.DoneAt = !c.Denied && !c.Unhooked,",
  "c.Done, c.IsError, c.Response, c.DoneAt = !c.Unhooked,", "TestP6_CallsReadATranscript|TestP6_BacktestReproducesTheLiveFires")
m("VER-6.3 a subagent's call is replayed", T,
  "\t\tif json.Unmarshal(sc.Bytes(), &l) != nil || l.IsSidechain {\n", "\t\tif json.Unmarshal(sc.Bytes(), &l) != nil {\n",
  "TestP6_CallsReadATranscript|TestP6_BacktestReproducesTheLiveFires")
m("VER-6.3 the replay's coverage is not verified", R,
  "\t\tRecordedAtMS: calls[0].At.UnixMilli(), SessionID: session, Phase: store.PhasePost, State: store.StateVerified,\n",
  "\t\tRecordedAtMS: calls[0].At.UnixMilli(), SessionID: session, Phase: store.PhasePost, State: store.StateUnverified,\n",
  "TestP6_BacktestReproducesTheLiveFires")
m("VER-6.3 no post is replayed", R,
  "\t\tif err := h.Flush(); err != nil || !c.Done {\n", "\t\tif err := h.Flush(); err != nil || true {\n",
  "TestP6_BacktestReproducesTheLiveFires")
m("VER-6.3 a fire counts twice", R,
  "\t\t\tout = append(out, Fire{Pattern: line.Pattern, Call: calls[line.AfterSeq]})\n",
  "\t\t\tout = append(out, Fire{Pattern: line.Pattern, Call: calls[line.AfterSeq]}, Fire{Pattern: line.Pattern})\n",
  "TestP6_BacktestReproducesTheLiveFires")
m("VER-6.3 a fire is matched on its whole line, store ids and keys included", R,
  "\t\t\tout = append(out, Fire{Pattern: line.Pattern, Call: calls[line.AfterSeq]})\n",
  "\t\t\tout = append(out, Fire{Pattern: line.Pattern, Call: string(l)})\n",
  "TestP6_BacktestReproducesTheLiveFires")
m("VER-6.3 a live fire answers twice", R, "\t\t\tleft[f]--\n", "", "TestP6_MatchedIsAMultisetIntersection")
m("VER-6.3 another pattern's fires are counted", C, "\t\tif f.Pattern == id {\n", "\t\tif true {\n",
  "TestP6_MatchedIsAMultisetIntersection")
m("VER-6.3 an unknown sensor is taken", C, "\tif !pattern.Valid(pattern.ID(sensor)) {\n", "\tif false {\n",
  "TestP6_BacktestArguments")
