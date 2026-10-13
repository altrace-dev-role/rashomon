package acceptance

// H-25 — forget one destination.
//
// The operation has three halves and is worth nothing without all of them: the
// records naming the host leave rashomon's own store, its project baseline
// entry goes, and the report keeps suppressing it from the wire view.
//
// That third half exists because of a boundary this tool does not cross. The
// proxy's causal.db is the closed product's hash-chained audit store, opened
// read-only, and deleting a row would break the chain it exists to provide. So
// a forgotten host would otherwise reappear in the very next report as a
// destination reached and never named -- the forget reading as undone, or as a
// finding. The gap record carries a KEYED DIGEST of the host -- not its name,
// which has to leave the store -- and the report recomputes that digest from
// each destination it observed.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

const forgettable = "acme-secret.internal"

// seedHostSession records a session that named two hosts, one to forget.
func seedHostSession(t *testing.T, e *env) {
	t.Helper()
	e.watched(testSession)
	for i, h := range []string{forgettable, "keep-me.example"} {
		p := defaultPayload()
		p.ToolUseID = "toolu_fh_" + string(rune('a'+i))
		p.ToolInput = map[string]any{"command": "curl https://" + h + "/x"}
		e.mustHook(p.build(t))
	}
	e.probe("end", testSession)
}

func TestH25_ForgetHostRemovesTheRecordsAndLeavesAGap(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	before := len(e.declarations(testSession))
	if before != 2 {
		t.Fatalf("premise: %d declarations, want 2", before)
	}

	res := e.run("", nil, "forget", "--host", forgettable)
	if res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// The call that named it is gone; the other call is untouched.
	after := e.declarations(testSession)
	if len(after) != 1 {
		t.Fatalf("got %d declarations after the forget, want 1", len(after))
	}

	// A gap record says that something left and identifies it well enough for
	// the report to keep suppressing it.
	gaps := e.gaps()
	if len(gaps) != 1 {
		t.Fatalf("got %d gap records, want 1", len(gaps))
	}
	g := gaps[0]
	if g.str("reason") != "forget_host" {
		t.Errorf("gap reason = %q, want forget_host", g.str("reason"))
	}
	// A KEYED DIGEST, not the hostname. The report has to recognise the host
	// again, but `forget --host` also has to make the name leave the store's
	// bytes -- writing the name here would satisfy the first by breaking the
	// second. The next test is what proves the name is gone.
	if d := g.str("host_digest"); d == "" {
		t.Error("gap carries no host_digest, so the report cannot keep the destination suppressed")
	} else if len(d) != 64 {
		t.Errorf("host_digest = %q, want 64 hex characters", d)
	}
	if strings.Contains(g.str("host_digest"), forgettable) {
		t.Error("the gap's digest field contains the plaintext host")
	}
	if g.fields["removed_records"] == float64(0) {
		t.Error("gap says 0 records removed")
	}
	// A gap is a session record: written at SessionSchemaVersion, so a binary
	// that reads only up to it still reads the gap.
	if got, want := g.fields["schema_version"], float64(store.SessionSchemaVersion); got != want {
		t.Errorf("gap schema_version = %v, want %v", got, want)
	}
}

// TestH25_HostIsGoneFromTheFileBytes is the assertion that survives an
// encoding. Checking that a reader no longer returns the host is weaker: the
// record could still be on disk and merely filtered.
func TestH25_HostIsGoneFromTheFileBytes(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	if res := e.run("", nil, "forget", "--host", forgettable); res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	for rel, f := range walkStore(t, e.home) {
		if strings.Contains(string(f.body), forgettable) {
			t.Errorf("%s still contains %q after forget --host; the records must leave "+
				"the file, not merely be filtered out of a reader", rel, forgettable)
		}
	}
	// The premise, so this cannot pass by the host never having been stored:
	// the host that was NOT forgotten is still on disk.
	var sawKeeper bool
	for _, f := range walkStore(t, e.home) {
		if strings.Contains(string(f.body), "keep-me.example") {
			sawKeeper = true
		}
	}
	if !sawKeeper {
		t.Error("premise broken: the host that was not forgotten is also absent, so the " +
			"assertion above proves nothing")
	}
}

// TestH25_ForgottenHostStaysSuppressedInTheReport is the third half, and the
// one the proxy boundary forces.
func TestH25_ForgottenHostStaysSuppressedInTheReport(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)
	db := e.writeProxyStore(t, forgettable, "keep-me.example")

	// Premise: before the forget, the wire shows it.
	plain := e.run("", nil, "report", "--session", testSession, "--proxy-store", db).stdout
	if !strings.Contains(plain, forgettable) {
		t.Fatalf("premise: the report does not mention %q before the forget:\n%s",
			forgettable, plain)
	}

	res := e.run("", nil, "forget", "--host", forgettable)
	if res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// A proxy store WAS read here -- the report above wrote a project baseline
	// -- so forget's output is exactly what it always was: the baseline count,
	// and the sentence saying the proxy's rows are not ours to delete.
	for _, want := range []string{
		"project baseline(s) cleared\n",
		"rashomon: the proxy's own records are not ours to delete",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("forget --host after a proxy store was read does not say %q:\n%s",
				want, res.stdout)
		}
	}

	after := e.run("", nil, "report", "--session", testSession, "--proxy-store", db)
	if strings.Contains(after.stdout, forgettable) {
		t.Errorf("the forgotten host is back in the report. The proxy's row still exists "+
			"-- it is not ours to delete -- so the report must suppress it from the "+
			"view or the forget reads as undone:\n%s", after.stdout)
	}
	// And it says something was withheld, rather than silently omitting rows.
	if !strings.Contains(after.stdout, "suppressed") {
		t.Errorf("the report does not say a destination was suppressed; a view that "+
			"silently omits rows is the same failure as one that prints nothing when "+
			"it was not watching:\n%s", after.stdout)
	}
	// The host that was not forgotten is still reported.
	if !strings.Contains(after.stdout, "keep-me.example") {
		t.Errorf("the forget removed more than it was asked to:\n%s", after.stdout)
	}
}

// TestH25_ForgetHostSaysNothingOfAProxyNeverRead: on a machine where no proxy
// store was ever read -- every machine in this alpha -- forget --host says
// what it removed in one line. The baseline count and the sentence about the
// proxy's own records explain state that exists only after a report has read
// a proxy store, and printed here they describe a product the reader does not
// have. The baseline directory is the evidence that one was read, because a
// report writes it only then.
func TestH25_ForgetHostSaysNothingOfAProxyNeverRead(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	res := e.run("", nil, "forget", "--host", forgettable)
	if res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// The record count is the store's business (a call leaves more than its
	// declaration behind); the shape of the line is this item's, and so is its
	// grammar: one run is "1 run", never "1 runs".
	want := regexp.MustCompile(`^rashomon: forgot ([1-9][0-9]*) (records?) naming ` +
		regexp.QuoteMeta(forgettable) + ` across 1 run; 1 gap record written\n$`)
	m := want.FindStringSubmatch(res.stdout)
	if m == nil {
		t.Fatalf("forget --host with no proxy store ever read printed\n%q\nwant one line "+
			"matching\n%s", res.stdout, want)
	}
	if (m[1] == "1") != (m[2] == "record") {
		t.Errorf("forget --host says %q %q: the noun does not agree with the count", m[1], m[2])
	}
}

// TestH25_ForgetHostRefusesToMixWithAWindow: --host removes by destination and
// --since/--before remove by time. Accepting both would make the scope of a
// deletion ambiguous, which is the one thing a deletion must never be.
func TestH25_ForgetHostRefusesToMixWithAWindow(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	res := e.run("", nil, "forget", "--host", forgettable, "--since", "1h")
	if res.exitCode == 0 {
		t.Error("forget --host --since was accepted; the scope of the deletion would be " +
			"ambiguous")
	}
	if len(e.declarations(testSession)) != 2 {
		t.Error("the refused command still removed records")
	}
}

// TestH25_ForgetUnknownHostIsNotAnError: asking to forget something that was
// never there is a no-op, not a failure. A user cleaning up cannot be expected
// to know which hosts a store holds.
func TestH25_ForgetUnknownHostIsNotAnError(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	res := e.run("", nil, "forget", "--host", "never-seen.example")
	if res.exitCode != 0 {
		t.Errorf("forget --host on an unknown host: exit %d, stderr %q",
			res.exitCode, res.stderr)
	}
	if len(e.declarations(testSession)) != 2 {
		t.Error("forgetting an unknown host removed records")
	}
	if len(e.gaps()) != 0 {
		t.Error("forgetting an unknown host wrote a gap record; a gap must mean records " +
			"actually left")
	}
}

// TestH25_AForgetThatRemovedNothingSaysTheHostStaysOut is #45 at the command
// line. "forgot 0 records" read as "nothing happened", and before the
// store-level record it was true: a host only the sandbox's trail held came
// back in the next report. The command now records the forget whatever it
// removed, and says so when it removed nothing.
func TestH25_AForgetThatRemovedNothingSaysTheHostStaysOut(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	res := e.run("", nil, "forget", "--host", "never-named.example")
	if res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	want := "rashomon: forgot 0 records naming never-named.example across 0 runs; " +
		"0 gap records written; later reports still leave it out wherever they find it, " +
		"the sandbox trail included\n"
	if res.stdout != want {
		t.Errorf("forget --host that removed nothing printed\n%q\nwant\n%q", res.stdout, want)
	}
}

// TestH25_TheStoreLevelForgetRecordIsClosed: forgotten-hosts.ndjson is a
// record file like the others, so it gets what they get -- a closed key set
// (H-13) and the store's file mode (H-15). The only check it had was that the
// hostname's bytes were absent, and a field carrying the name in any encoding,
// base64 included, passed it.
func TestH25_TheStoreLevelForgetRecordIsClosed(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	if res := e.run("", nil, "forget", "--host", forgettable); res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	f, ok := walkStore(t, e.home)[store.FileForgottenHosts]
	if !ok {
		t.Fatalf("forget --host wrote no %s", store.FileForgottenHosts)
	}
	if f.mode.Perm() != 0o600 {
		t.Errorf("%s has mode %v, want 0600", store.FileForgottenHosts, f.mode.Perm())
	}
	recs := readNDJSON(t, filepath.Join(e.home, store.FileForgottenHosts))
	if len(recs) != 1 {
		t.Fatalf("%s holds %d records after one forget, want 1", store.FileForgottenHosts, len(recs))
	}
	assertKeySet(t, recs[0], forgottenHostKeys)
	if recs[0].typ() != store.TypeForgottenHost || recs[0].fields["schema_version"] != float64(store.SessionSchemaVersion) {
		t.Errorf("the record is %v, want type %s at schema_version %d", recs[0].fields, store.TypeForgottenHost, store.SessionSchemaVersion)
	}
	if d, _ := recs[0].fields["host_digest"].(string); !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(d) {
		t.Errorf("host_digest = %q, want 64 hex characters", d)
	}
}

// TestH25_ForgetHostTakesAnySpellingOfTheHost: every host rashomon holds -- in
// its records, its baselines, the proxy's rows and the sandbox's trail -- has
// been through host.Canonical. `forget --host` digested and matched the value
// as typed, so any other spelling removed nothing, cleared no baseline,
// suppressed nothing, and the line said later reports would leave it out.
func TestH25_ForgetHostTakesAnySpellingOfTheHost(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)
	db := e.writeProxyStore(t, forgettable, "keep-me.example")
	if plain := e.run("", nil, "report", "--session", testSession, "--proxy-store", db).stdout; !strings.Contains(plain, forgettable) {
		t.Fatalf("premise: the report does not mention %q before the forget:\n%s", forgettable, plain)
	}

	const spelled = "https://ACME-Secret.Internal:443/"
	res := e.run("", nil, "forget", "--host", spelled)
	if res.exitCode != 0 {
		t.Fatalf("forget --host %q: exit %d, stderr %q", spelled, res.exitCode, res.stderr)
	}
	if want := " naming " + forgettable + " across 1 run;"; !strings.Contains(res.stdout, want) {
		t.Errorf("forget --host %q does not name what it forgot (%q):\n%s", spelled, want, res.stdout)
	}
	if got := len(e.declarations(testSession)); got != 1 {
		t.Errorf("forget --host %q left %d declarations, want 1", spelled, got)
	}
	// The proxy's own store is not ours to delete from; everything else under
	// the store root, the project baseline included, must have let the name go.
	for rel, f := range walkStore(t, e.home) {
		if rel != filepath.Base(db) && strings.Contains(string(f.body), forgettable) {
			t.Errorf("%s still holds %q after forget --host %q", rel, forgettable, spelled)
		}
	}
	if after := e.run("", nil, "report", "--session", testSession, "--proxy-store", db).stdout; strings.Contains(after, forgettable) {
		t.Errorf("forget --host %q: the host is back in the report:\n%s", spelled, after)
	}
}

// TestH25_ForgetHostRefusesWhatIsNotAHostname: a value with a path, a
// credential or whitespace names no destination, so forgetting it can only be
// a no-op that reads as a success. Refused, without echoing the value -- a
// credential in it would otherwise land in whatever captures stderr, a
// session transcript included -- and without touching the store.
func TestH25_ForgetHostRefusesWhatIsNotAHostname(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	res := e.run("", nil, "forget", "--host", "deploy:s3cret@"+forgettable)
	if res.exitCode == 0 {
		t.Errorf("forget --host with a credential in it was accepted:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "is not a hostname") {
		t.Errorf("the refusal does not say why: stderr %q", res.stderr)
	}
	if strings.Contains(res.stderr, "s3cret") {
		t.Errorf("the refusal echoes the credential it refused: stderr %q", res.stderr)
	}
	if got := len(e.declarations(testSession)); got != 2 {
		t.Errorf("the refused command removed records: %d declarations left, want 2", got)
	}
	if _, ok := walkStore(t, e.home)[store.FileForgottenHosts]; ok {
		t.Error("the refused command wrote a store-level forget record")
	}

	// Refused as an argument, not as a store operation: with no store here it
	// is still an error, rather than "nothing recorded here" and exit 0.
	if res := newEnv(t).run("", nil, "forget", "--host", "py pi.example"); res.exitCode == 0 {
		t.Errorf("forget --host with a space in it was accepted on a machine with no store:\n%s", res.stdout)
	}
}
