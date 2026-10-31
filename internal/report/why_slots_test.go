package report

import (
	"strings"
	"testing"
)

// TestRP2ze_WhySlotsAreAClosedAllowlist (card REPORT1b review, blocking): a
// view built by hand with free text in a slot outside the allowlist prints
// the text nowhere, in WhyText or after RedactWhy, whatever the slot is
// named; an allowlisted slot prints, and a path slot is digested by
// --redact. Break: print every slot's value, redact only "path", widen the
// allowlist to "text".
func TestRP2ze_WhySlotsAreAClosedAllowlist(t *testing.T) {
	v := &WhyView{Prefix: "abcdef", SessionID: "s1", LineID: "abcdef0123", State: "suppressed", SuppressedBy: "arm_off",
		Slots: []WhySlot{{Name: "gate", Value: "go"}, {Name: "message", Value: "Acme Corp secret"}, {Name: "path", Value: "/work/app/x.go"},
			{Name: "text", Value: "use Acme Corp staging cluster"}}, Source: WhySource{Kind: "none"}}
	for name, view := range map[string]*WhyView{"plain": v, "redacted": RedactWhy(v, []byte("k"))} {
		var b strings.Builder
		if err := WhyText(&b, view); err != nil {
			t.Fatal(err)
		}
		out := b.String()
		if strings.Contains(out, "Acme") || !strings.Contains(out, "gate=") || !strings.Contains(out, "text=<29 bytes>") || !strings.Contains(out, "message=<16 bytes>") {
			t.Errorf("%s:\n%s", name, out)
		}
		// The gate slot is a program word or a spec gate's id, which may be a
		// hostname, so --redact digests it (STEP-15): "go" survives the plain
		// view only.
		if (name == "redacted") == strings.Contains(out, "gate=go") {
			t.Errorf("%s: the gate slot's redaction is wrong:\n%s", name, out)
		}
		if name == "redacted" && strings.Contains(out, "/work/app") {
			t.Errorf("redacted view keeps the path:\n%s", out)
		}
		for _, s := range view.Slots {
			if name == "redacted" && !WhySlotShown(s.Name) && s.Name != "path" && s.Value != "" {
				t.Errorf("redacted view keeps slot %s = %q", s.Name, s.Value)
			}
		}
	}
	if v.Slots[1].Value == "" {
		t.Error("RedactWhy changed the original view")
	}
}
