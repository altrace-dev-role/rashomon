package acceptance

// H-24 — a report that can be shared.
//
// A hostname is the most sensitive field this product stores: it can carry a
// tenant, a bucket, a customer name or an internal topology. That is exactly
// the report someone wants to paste into an issue, and without redaction the
// only safe answer is "do not share it" -- which makes the tool useless in the
// conversation where it is most valuable.
//
// The assertion that matters is the LEAK HUNT: both renders are searched for
// the plaintext hostnames, rather than checking the fields the redactor happens
// to know about. A redactor that misses a newly added field would pass every
// field-by-field test and leak one host into an otherwise clean report.

import (
	"strings"
	"testing"
)

// secretHosts are the plaintext values that must not survive redaction. They
// are shaped like the things that actually leak: a customer name, an internal
// service, a bucket.
var secretHosts = []string{
	"acme-migration.internal",
	"billing-prod.eu-west-1.amazonaws.com",
	"customer-uploads.s3.amazonaws.com",
}

func TestH24_RedactedReportCarriesNoPlaintextHost(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// Declared hosts reach the store through the shape extractor, so the
	// commands name them the way a real session would.
	for i, h := range secretHosts {
		p := defaultPayload()
		p.ToolUseID = "toolu_redact_" + string(rune('a'+i))
		p.ToolInput = map[string]any{"command": "curl https://" + h + "/status"}
		e.mustHook(p.build(t))
	}
	e.probe("end", testSession)
	db := e.writeProxyStore(t, secretHosts...)

	plain := e.run("", nil, "report", "--session", testSession, "--proxy-store", db)
	redacted := e.run("", nil, "report", "--session", testSession, "--proxy-store", db, "--redact")
	redactedJSON := e.run("", nil, "report", "--json", "--session", testSession, "--proxy-store", db, "--redact")

	if redacted.exitCode != 0 || redactedJSON.exitCode != 0 {
		t.Fatalf("--redact exited %d/%d: %q %q",
			redacted.exitCode, redactedJSON.exitCode, redacted.stderr, redactedJSON.stderr)
	}

	// Premise: the plain render must actually contain them, or the leak hunt
	// below passes vacuously.
	for _, h := range secretHosts {
		if !strings.Contains(plain.stdout, h) {
			t.Fatalf("premise broken: the plain report does not contain %q, so the "+
				"redaction assertion would pass without redacting anything", h)
		}
	}

	for _, h := range secretHosts {
		if strings.Contains(redacted.stdout, h) {
			t.Errorf("the redacted text report contains the plaintext host %q", h)
		}
		if strings.Contains(redactedJSON.stdout, h) {
			t.Errorf("the redacted JSON report contains the plaintext host %q", h)
		}
	}
}

// TestH24_RedactionKeepsTheShapeOfTheFinding is the other half. A redaction that
// removed the findings would be safe and worthless: the point is to be able to
// discuss what happened without naming it.
func TestH24_RedactionKeepsTheShapeOfTheFinding(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://acme-migration.internal/x"}
	e.mustHook(p.build(t))
	e.probe("end", testSession)
	db := e.writeProxyStore(t, "acme-migration.internal")

	out := e.run("", nil, "report", "--session", testSession, "--proxy-store", db, "--redact").stdout

	// The suffix survives: it says something real about where traffic went
	// without naming who.
	if !strings.Contains(out, ".internal") {
		t.Errorf("the public suffix did not survive redaction:\n%s", out)
	}
	// And the section structure survives.
	for _, want := range []string{"declared but not observed", "executed differently from declared"} {
		if !strings.Contains(out, want) {
			t.Errorf("the redacted report is missing the %q line:\n%s", want, out)
		}
	}
}

// TestH24_RedactionIsStableAndDistinguishing: one host must render as one
// digest throughout a report so a reader can follow it across sections, and two
// hosts must not collapse into one, or the report would understate how many
// destinations were reached.
func TestH24_RedactionIsStableAndDistinguishing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	for i, h := range []string{"one.example", "two.example"} {
		p := defaultPayload()
		p.ToolUseID = "toolu_stable_" + string(rune('a'+i))
		p.ToolInput = map[string]any{"command": "curl https://" + h}
		e.mustHook(p.build(t))
	}
	e.probe("end", testSession)
	db := e.writeProxyStore(t, "one.example", "two.example")

	first := e.run("", nil, "report", "--session", testSession, "--proxy-store", db, "--redact").stdout
	second := e.run("", nil, "report", "--session", testSession, "--proxy-store", db, "--redact").stdout

	// Two renders of one store agree, apart from the generated-at line.
	stripStamp := func(s string) string {
		var keep []string
		for _, line := range strings.Split(s, "\n") {
			if !strings.HasPrefix(line, "rashomon report -- generated") {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "\n")
	}
	if stripStamp(first) != stripStamp(second) {
		t.Error("two redacted renders of one store differ; the digest is not stable")
	}

	// The two hosts produced two distinct digests.
	digests := map[string]bool{}
	for _, line := range strings.Split(first, "\n") {
		for _, f := range strings.Fields(line) {
			f = strings.Trim(f, ",")
			if strings.HasSuffix(f, ".example") && len(f) == len("00000000.example") {
				digests[f] = true
			}
		}
	}
	if len(digests) < 2 {
		t.Errorf("two distinct hosts produced %d digest(s) %v; a collision would "+
			"understate how many destinations were reached", len(digests), digests)
	}
}

// TestH24_AccountIsDroppedNotPartiallyCleaned. The agent's summary is prose and
// may name anything; a summary with some names removed reads as safe while
// being exactly as revealing as before.
func TestH24_AccountIsDroppedNotPartiallyCleaned(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	const secret = "acme-migration"
	transcript := e.writeTranscript(t, "Finished the "+secret+" work.")
	p := defaultPayload()
	p.TranscriptPath = transcript
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	plain := e.run("", nil, "report", "--session", testSession).stdout
	if !strings.Contains(plain, secret) {
		t.Skipf("premise: the plain report does not quote the summary here:\n%s", plain)
	}

	out := e.run("", nil, "report", "--session", testSession, "--redact").stdout
	if strings.Contains(out, secret) {
		t.Errorf("the redacted report still quotes the agent's summary:\n%s", out)
	}
	if !strings.Contains(out, "redacted") {
		t.Errorf("the redacted report does not say that a summary existed; \"the agent "+
			"said nothing\" and \"we are not showing you what it said\" are "+
			"different facts:\n%s", out)
	}
}
