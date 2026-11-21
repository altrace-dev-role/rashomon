package steer

// ReqID is the id of the requirement pred(arg): the predicate, a colon and
// the argument. It is the id the session fold gives a requirement a delivered
// line states (session's require), so a Stop row's Req and a frontier's
// ReqState.ID match by equality, with no prefix to cut (W-M3.4).
// The argument is kept whole, colons included: gate_green of spec:unit is
// "gate_green:spec:unit". TestReqID_EqualsFoldKey pins the equality.
func ReqID(pred, arg string) string { return pred + ":" + arg }
