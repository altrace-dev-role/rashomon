package report

// A trusted spec's gate id is the gate's name (STOP-NAMES-GATE1) and its
// charset admits a hostname, so the autosteer fields it reaches are
// host-bearing: the gate's name, the Stop's sentence, a finding's gate slot,
// and a check's gate key and its reason (which spells that key). Pattern ids,
// titles and kinds are closed words and stay false in the literal. The
// knowledge block's gate keys (spec:<id>) and a why view's gate are the same
// word, from REPORT1b. They are declared here, over the literal's false
// entries, so that the declaration sits with its test.
func init() {
	for _, p := range []string{
		"autosteer.gates[].name",
		"autosteer.stop.sentence",
		"autosteer.checks[].gate",
		"autosteer.checks[].reason",
		"autosteer.findings[].gate",
		"knowledge.gate_states[].gate",
	} {
		if _, ok := hostBearing[p]; !ok {
			panic("redact_gateid_test: " + p + " is not a declared field")
		}
		hostBearing[p] = true
	}
}
