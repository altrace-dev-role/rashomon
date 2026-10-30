# Rubric conc-7: the goroutine readRecapPayload abandons at its timeout ends
# when its Read returns, because its result has room in the channel. Without
# that room it would block on the send for the life of the process.
m("RUBRIC conc-7 the abandoned stdin reader blocks on its send", "cmd/rashomon/recap_turn.go",
  "\tch := make(chan pendingPayload, 1)\n\tsafe.Go(func() {\n\t\tfm, ok := readFinalMessage(in, true, true, withPrompt)",
  "\tch := make(chan pendingPayload)\n\tsafe.Go(func() {\n\t\tfm, ok := readFinalMessage(in, true, true, withPrompt)",
  "TestRubricCONC7_AbandonedStdinReadEndsWhenTheReadReturns")
