package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// B3 -- the redaction digest is keyed.
//
// It used to be a bare sha256 truncated to 32 bits. A hostname is a tiny,
// guessable space, so a recipient holding a candidate simply hashed it and
// compared: `sha256("pypi.org")[:8]` is `a4aa2ac2`, which is what the report
// printed. The README promised a keyed digest and the code did not have one --
// the README was corrected first, because saying the true thing is not
// optional while the fix is in flight. This is the fix.
//
// Keyed with the per-install key, a recipient who does not have that key cannot
// confirm a guess at all. That is the property, and it is the whole difference.

var keyA = []byte("install-a-key-0123456789abcdef")
var keyB = []byte("install-b-key-0123456789abcdef")

// TestRedact_DigestIsNotTheUnkeyedHash is the regression, stated as the exact
// value that used to leak.
func TestRedact_DigestIsNotTheUnkeyedHash(t *testing.T) {
	const host = "pypi.org"
	sum := sha256.Sum256([]byte(host))
	unkeyed := hex.EncodeToString(sum[:])[:redactedHostLen]

	got := redactHost(host, keyA)

	if strings.HasPrefix(got, unkeyed) {
		t.Errorf("redactHost(%q) = %q, which still begins with the UNKEYED digest %q. "+
			"Anyone holding a candidate hostname recovers it with one line of Python.",
			host, got, unkeyed)
	}
	if !strings.HasSuffix(got, ".org") {
		t.Errorf("redactHost(%q) = %q; the last label is kept deliberately", host, got)
	}
}

// TestRedact_DifferentInstallsProduceDifferentDigests is what "keyed" buys.
// Two machines redacting the same host must not produce the same string, or a
// digest seen in one shared report identifies the host in another.
func TestRedact_DifferentInstallsProduceDifferentDigests(t *testing.T) {
	a := redactHost("internal.example", keyA)
	b := redactHost("internal.example", keyB)

	if a == b {
		t.Errorf("both installs rendered %q. A digest that is the same everywhere is a "+
			"dictionary entry: build the table once and read every report.", a)
	}
}

// TestRedact_StableWithinAndAcrossReportsOnOneInstall is the property that has
// to survive keying. A reader follows one host across the sections of a report,
// and compares two reports from the same machine.
func TestRedact_StableWithinAndAcrossReportsOnOneInstall(t *testing.T) {
	first := redactHost("files.pythonhosted.org", keyA)
	second := redactHost("files.pythonhosted.org", keyA)

	if first != second {
		t.Errorf("the same host on the same install rendered %q then %q; a reader cannot "+
			"follow a destination across sections", first, second)
	}
	if other := redactHost("pypi.org", keyA); other == first {
		t.Errorf("two different hosts collided on %q", other)
	}
}

// TestRedact_UsesItsOwnDomainSeparator keeps the two digests in this product
// apart.
//
// `forget --host` stores an HMAC of the host under the separator
// "forget-host\x00" so a gap record can be matched without naming the host.
// Redaction must NOT produce that same value: a shared report would then carry
// a token that can be tested directly against the gap records in a store.
func TestRedact_UsesItsOwnDomainSeparator(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const host = "acme-internal.example.com"

	forgetDigest := st.HostDigest(host)
	redactDigest := redactHost(host, st.Key())

	// Compare on the digest part only; redactHost appends the last label.
	red := strings.SplitN(redactDigest, ".", 2)[0]
	if strings.HasPrefix(forgetDigest, red) {
		t.Errorf("the redaction digest %q is a prefix of the forget-host digest %q. One key, "+
			"two purposes, no domain separation -- a shared report then carries a token that "+
			"can be tested against a store's gap records.", red, forgetDigest)
	}
}

// TestRedact_WithoutAKeyNothingIsDigested. A report built with no store has no
// key. It also has no hosts, so this cannot arise in practice -- but if it ever
// does, the answer must not be a silent fall back to the unkeyed hash, which is
// the property being removed.
func TestRedact_WithoutAKeyNothingIsDigested(t *testing.T) {
	got := redactHost("pypi.org", nil)

	sum := sha256.Sum256([]byte("pypi.org"))
	unkeyed := hex.EncodeToString(sum[:])[:redactedHostLen]
	if strings.HasPrefix(got, unkeyed) {
		t.Errorf("with no key it fell back to the unkeyed digest %q", unkeyed)
	}
	if !strings.Contains(got, "redacted") {
		t.Errorf("with no key it rendered %q; it should say it could not digest rather than "+
			"produce something that looks like a digest", got)
	}
}

// TestRedact_ReportCarriesTheLegend. The caveat belongs in the artifact that
// gets shared, not only in a README the recipient never sees.
func TestRedact_ReportCarriesTheLegend(t *testing.T) {
	rep := &Report{Sessions: []Session{{
		SessionID: "s1",
		Destinations: Destinations{
			Observed: true,
			Hosts:    []wire.Destination{{Host: "pypi.org", Attempts: 1}},
			WireOnly: []string{"pypi.org"},
		},
	}}}

	plain := renderText(t, rep)
	if strings.Contains(plain, "hostnames are redacted") {
		t.Error("the plain report carries the redaction legend")
	}

	out := renderText(t, Redact(rep, keyA))
	if !strings.Contains(out, "hostnames are redacted") {
		t.Errorf("the redacted report does not explain its own digests:\n%s", out)
	}
	if !strings.Contains(out, "this install") {
		t.Errorf("the legend does not say the digest is per-install, which is the one thing "+
			"it buys:\n%s", out)
	}
	if strings.Contains(out, "pypi.org") {
		t.Errorf("the plain hostname survived redaction:\n%s", out)
	}
}

// renderText renders a whole report to text, for the assertions above.
func renderText(t *testing.T, rep *Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := Text(&b, rep); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}
