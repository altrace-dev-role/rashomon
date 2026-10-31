package report

import "strings"

// redactAutosteer digests the places a trusted spec's gate id reaches the
// autosteer block: each gate's name, the Stop's state sentence (which spells
// the names of the gates it speaks of), a finding's gate slot, and a check's
// gate key and reason. A gate id's charset admits a hostname
// (deploy.corp.example), so under --redact none survives; the sentence is
// digested whole, since it is prose built from those names. Row, kind, state,
// pattern ids, titles and every seq stay. The unredacted report keeps the owner's own word. The
// gates and the stop are rebuilt, for the reason redactLinks gives.
func redactAutosteer(a *Autosteer, key []byte) *Autosteer {
	if a == nil {
		return nil
	}
	out := *a
	out.Gates = make([]AutosteerGate, 0, len(a.Gates))
	for _, g := range a.Gates {
		g.Name = redactWord(g.Name, key)
		out.Gates = append(out.Gates, g)
	}
	// A finding's gate is the steer line's gate slot: the program word, or a
	// spec gate's bare id. Digested whole, since the slot cannot say which.
	out.Findings = make([]AutosteerFinding, 0, len(a.Findings))
	for _, f := range a.Findings {
		f.Gate = redactWord(f.Gate, key)
		out.Findings = append(out.Findings, f)
	}
	// A check's gate is a declared gate's key (spec:<id>), and its reason
	// spells that key ("no green run of spec:<id> since the last edit"); the
	// reason keeps its words and loses only the key.
	out.Checks = make([]AutosteerCheck, 0, len(a.Checks))
	for _, c := range a.Checks {
		if c.Gate != "" {
			c.Reason = strings.ReplaceAll(c.Reason, c.Gate, redactGateKey(c.Gate, key))
			c.Gate = redactGateKey(c.Gate, key)
		}
		out.Checks = append(out.Checks, c)
	}
	if a.Stop != nil {
		st := *a.Stop
		st.Sentence = redactWord(st.Sentence, key)
		out.Stop = &st
	}
	return &out
}

// redactGateKey digests the owner's word in a gate key. "spec:<id>" keeps its
// prefix and digests the id; "learned:..." and "ci:..." keys name a program
// family and a command's index, never an owner's word, and stay; any other
// key is not one the program mints, so it is digested whole.
func redactGateKey(k string, key []byte) string {
	if id, ok := strings.CutPrefix(k, "spec:"); ok {
		return "spec:" + redactWord(id, key)
	}
	if strings.HasPrefix(k, "learned:") || strings.HasPrefix(k, "ci:") {
		return k
	}
	return redactWord(k, key)
}

// redactWord is redactHost without the kept suffix: a gate id or a sentence is
// not a host, so the tail after its last dot is part of the name, not a TLD.
func redactWord(w string, key []byte) string {
	d := redactHost(w, key)
	if i := strings.Index(d, "."); i >= 0 && !strings.HasPrefix(d, "[") {
		return d[:i]
	}
	return d
}
