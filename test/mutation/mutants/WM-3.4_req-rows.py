# WM-3.4 (W-M3.4, the Stop's requirement rows): a requirement the Stop's
# caller checked is one expected_evidence row of the ledger, keyed by its
# family, its id in Req, the id the session fold gives it (steer.ReqID).
# gate_green reads the Stop's own gate row and never the caller's word, takes
# the state gateAfterWriteTable gives that row, and reads unknown where the
# ledger's own rows for that gate could not say. No
# requirement row enters the done flag's clause, and with no requirements
# the ledger is byte for byte what it was. Each break below undoes one of
# those promises; each row names the pin that holds it.
REQS = "internal/verify/reqs.go"
m("WM-3.4 ReqID drops the predicate", "internal/steer/reqid.go",
  "func ReqID(pred, arg string) string { return pred + \":\" + arg }",
  "func ReqID(pred, arg string) string { return arg }",
  "TestReqID_EqualsFoldKey")
m("WM-3.4 every ledger row carries an empty req key", "internal/verify/obligations.go",
  "`json:\"req,omitempty\"`",
  "`json:\"req\"`",
  "TestReq_ReqsNilIsByteEqual")
m("WM-3.4 two requirements of one id are two rows", REQS,
  "\t\t\tif out[i].Req == q.ID {\n",
  "\t\t\tif out[i].Req == q.ID && i < 0 {\n",
  "TestReq_OneRowPerID")
m("WM-3.4 a check that could not be made reads as made", REQS,
  "\t\tcase !q.Known:\n",
  "\t\tcase false:\n",
  "TestReq_RowPerReqCarriesItsID")
m("WM-3.4 gate_green takes the caller's word", REQS,
  "\tif q.Pred != PredGateGreen {\n",
  "\tif true {\n",
  "TestReq_GateGreenIgnoresCallerHolds")
m("WM-3.4 gate_green is met on an incomplete record", REQS,
  "\tif snap.Recording != Complete {\n\t\treturn ObligationUnknown\n\t}\n",
  "",
  "TestReq_GateGreenIgnoresCallerHolds")
m("WM-3.4 gate_green is met from a source outside the four", REQS,
  "\t\tif _, known := obligationSource(g.Source); !known {\n",
  "\t\tif _, known := obligationSource(g.Source); !known && i < 0 {\n",
  "TestReq_GateGreenIgnoresCallerHolds")
m("WM-3.4 gate_green reads a gate never run as unmet, off the ledger's table", REQS,
  "\t\t\tif r.in == g.State {\n\t\t\t\treturn r.out\n",
  "\t\t\tif r.in == g.State && r.in == Absent {\n\t\t\t\treturn ObligationUnmet\n\t\t\t}\n"
  "\t\t\tif r.in == g.State {\n\t\t\t\treturn r.out\n",
  "TestReq_GateGreenIsTheTableRow")
m("WM-3.4 an unmet requirement enters the done flag's clause", "internal/verify/evidence.go",
  "func UnmetExpected(ex []Expected, obs []Obligation) []Expected {\n\tvar out []Expected\n",
  "func UnmetExpected(ex []Expected, obs []Obligation) []Expected {\n\tvar out []Expected\n"
  "\tfor _, o := range obs {\n\t\tif o.Source == SourceRequirement && o.State == ObligationUnmet {\n"
  "\t\t\tout = append(out, Expected{Kind: o.Req, State: o.State})\n\t\t}\n\t}\n",
  "TestReq_UnmetExpectedNamesNoRequirementRow|TestReq_DoneFlagIgnoresReqs")
m("WM-3.4 intent states the spec gate's own id", "internal/intent/intent.go",
  "Gate{ID: d.ID, Words: w, Trust: t}",
  "Gate{ID: d.Gate.ID, Words: w, Trust: t}",
  "TestIntent_GateIDIsIdFamily")
