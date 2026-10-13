package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TRU-58b: after forget --host, the host returns nowhere in the report: not in
// declared_hosts, not in ssh_hosts, not on any chain link, not in novelty. And
// the project's baseline lets it go, so a session that names it again later
// is told it is new.
//
// declared_hosts and ssh_hosts are read from exactly the declaration fields
// ForgetHost matches, Hosts and SSHHosts. Reading any other field would let a
// forgotten host return under a different heading; the records are deleted,
// so nothing may read around them.
//
// THREE SESSIONS IN ONE PROJECT, because novelty needs them. The first
// establishes the project's baseline, and an establishing session reports no
// novel host: with it alone, novelty.hosts is empty before the forget as well
// as after, and the assertion could not fail. The second is the session under
// test, whose host is novel before the forget. The third names the host after
// the forget, and is told it is new only if the baseline forgot it.
func TestTRU58b_ForgottenHostNeverReturnsToTheReport(t *testing.T) {
	const host = "acme-secret.internal"
	const first, third = "sess-tru58-first", "sess-tru58-third"
	e := newEnv(t)

	e.watched(first)
	e.declare(first, "curl https://keep-me.example/y")
	e.probe("end", first)
	if got := valueAt(e.sessionJSON(t, first), "destinations", "novelty", "established"); got != true {
		t.Fatalf("premise: the first session did not establish the project's baseline (established = %v)", got)
	}

	e.probe("start", testSession)
	e.declare(testSession,
		"curl https://"+host+"/x",
		"ssh deploy@"+host+" uptime",
		"curl https://keep-me.example/y")
	e.probe("end", testSession)

	lists := [][]string{
		{"destinations", "declared_hosts"},
		{"destinations", "ssh_hosts"},
		{"destinations", "novelty", "hosts"},
	}
	linkKeys := []string{"declared_hosts", "ssh_hosts"}

	// Premise: before the forget, the host is everywhere the record puts it.
	before := e.sessionJSON(t, testSession)
	for _, path := range lists {
		if !listHas(before, host, path...) {
			t.Fatalf("premise: %q is not in %s before the forget:\n%v", host, strings.Join(path, "."), before["destinations"])
		}
	}
	for _, key := range linkKeys {
		if !chainsName(before, host, key) {
			t.Fatalf("premise: no chain link names %q under %s before the forget", host, key)
		}
	}

	res := e.run("", nil, "forget", "--host", host)
	if res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// One project, so one baseline, and the host was in it.
	if !strings.HasSuffix(res.stdout, "; 1 project baseline(s) cleared\n") {
		t.Errorf("forget --host does not say it cleared the project's baseline:\n%s", res.stdout)
	}

	after := e.sessionJSON(t, testSession)
	for _, path := range lists {
		if listHas(after, host, path...) {
			t.Errorf("%q is still in %s after the forget", host, strings.Join(path, "."))
		}
	}
	for _, key := range linkKeys {
		if chainsName(after, host, key) {
			t.Errorf("a chain link still names %q under %s after the forget", host, key)
		}
	}
	// The whole rendering, so a heading this test did not think of cannot
	// carry the name either.
	raw, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), host) {
		t.Errorf("the session's JSON still contains %q after the forget:\n%s", host, raw)
	}
	// And only that host went.
	if !listHas(after, "keep-me.example", "destinations", "declared_hosts") {
		t.Errorf("the forget removed more than it was asked to; keep-me.example is gone")
	}

	// The baseline forgot it too: a later session naming it again is told it
	// is new.
	e.probe("start", third)
	e.declare(third, "curl https://"+host+"/z")
	e.probe("end", third)
	if !listHas(e.sessionJSON(t, third), host, "destinations", "novelty", "hosts") {
		t.Errorf("after the forget, a later session naming %q is not told it is new: the "+
			"project's baseline still remembers it", host)
	}
}

// TRU-58b, through the store-level record. A host that only the sandbox's
// trail holds is named by no record, so the forget removes nothing and writes
// no gap, and only forgotten-hosts.ndjson can keep it out (#45). The trail is
// nono's file and is not ours to edit, so the host must still return nowhere:
// not under the sandbox heading, not in the JSON, not in the text.
func TestTRU58b_AHostOnlyTheSandboxHeldNeverReturns(t *testing.T) {
	const host = "acme-secret.internal"
	e := newEnv(t)
	e.watched(testSession)
	time.Sleep(5 * time.Millisecond)
	at := time.Now().UnixMilli()
	e.declare(testSession, "curl https://keep-me.example/y")
	time.Sleep(5 * time.Millisecond)
	e.probe("end", testSession)

	// Outside the store: nono's file, which the forget must not need to edit.
	trail := filepath.Join(t.TempDir(), "audit-events.ndjson")
	body := `{"sequence":0,"event":{"type":"session_started"}}` + "\n"
	for i, ev := range [][2]string{{host, "deny"}, {"pypi.org", "allow"}} {
		body += fmt.Sprintf(`{"sequence":%d,"event":{"type":"network","event":{"timestamp_unix_ms":%d,`+
			`"mode":"connect","decision":%q,"target":%q,"port":443}}}`+"\n", i+1, at, ev[1], ev[0])
	}
	if err := os.WriteFile(trail, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	report := func() (map[string]any, string, string) {
		t.Helper()
		res := e.run("", nil, "report", "--json", "--session", testSession, "--nono-audit", trail)
		if res.exitCode != 0 {
			t.Fatalf("report --json: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		var rep struct {
			Sessions []map[string]any `json:"sessions"`
		}
		if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
			t.Fatalf("report --json: %v, %d sessions\n%s", err, len(rep.Sessions), res.stdout)
		}
		text := e.run("", nil, "report", "--session", testSession, "--nono-audit", trail, "--chain").stdout
		return rep.Sessions[0], res.stdout, text
	}

	before, _, text := report()
	if !listHas(before, host, "nono", "denied") || !strings.Contains(text, host) {
		t.Fatalf("premise: %q is not under the sandbox heading before the forget:\n%v\n%s",
			host, before["nono"], text)
	}

	res := e.run("", nil, "forget", "--host", host)
	if res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// Premise: the gap path played no part. Only the store-level record can
	// be what keeps the host out below.
	if n := len(e.gaps()); n != 0 {
		t.Fatalf("premise: the forget wrote %d gap(s), so a gap could be what suppresses the host", n)
	}
	files := walkStore(t, e.home)
	if _, ok := files[store.FileForgottenHosts]; !ok {
		t.Fatalf("forget --host wrote no %s", store.FileForgottenHosts)
	}
	for rel, f := range files {
		if strings.Contains(string(f.body), host) {
			t.Errorf("%s holds %q after the forget", rel, host)
		}
	}

	after, raw, text := report()
	if strings.Contains(raw, host) {
		t.Errorf("the report's JSON still contains %q after the forget", host)
	}
	if strings.Contains(text, host) {
		t.Errorf("the text report still names %q after the forget:\n%s", host, text)
	}
	// And only that host went.
	if !listHas(after, "pypi.org", "nono", "allowed") {
		t.Errorf("the forget took more than it was asked to: nono = %v", after["nono"])
	}
}

// declare records one Bash call per command, in the named session.
func (e *env) declare(sessionID string, commands ...string) {
	e.t.Helper()
	for i, command := range commands {
		p := defaultPayload()
		p.SessionID = sessionID
		p.ToolUseID = fmt.Sprintf("toolu_%s_%d", sessionID, i)
		p.ToolInput = map[string]any{"command": command}
		e.mustHook(p.build(e.t))
	}
}

// sessionJSON is one session of `report --json`, decoded generically so the
// test reads the contract as a consumer would, by key.
func (e *env) sessionJSON(t *testing.T, sessionID string) map[string]any {
	t.Helper()
	res := e.run("", nil, "report", "--json", "--session", sessionID)
	if res.exitCode != 0 {
		t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil {
		t.Fatalf("report output is not JSON: %v\n%s", err, res.stdout)
	}
	if len(rep.Sessions) != 1 {
		t.Fatalf("report returned %d sessions, want 1", len(rep.Sessions))
	}
	return rep.Sessions[0]
}

// valueAt walks the keys and returns what is there, or nil when a key is
// missing or a value on the way is not an object.
func valueAt(m map[string]any, path ...string) any {
	var cur any = m
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[key]
	}
	return cur
}

// listHas walks the keys to a list of strings and reports whether it holds
// want. A missing key or a value of another shape is false, never a panic.
func listHas(m map[string]any, want string, path ...string) bool {
	items, ok := valueAt(m, path...).([]any)
	if !ok {
		return false
	}
	for _, it := range items {
		if s, ok := it.(string); ok && s == want {
			return true
		}
	}
	return false
}

// chainsName reports whether any link anywhere in the chains section lists
// the host under key.
func chainsName(sess map[string]any, host, key string) bool {
	chains, _ := sess["chains"].(map[string]any)
	links := []any{}
	if prompts, ok := chains["prompts"].([]any); ok {
		for _, p := range prompts {
			if pm, ok := p.(map[string]any); ok {
				if ls, ok := pm["links"].([]any); ok {
					links = append(links, ls...)
				}
			}
		}
	}
	for _, key := range []string{"unattributed", "dropped"} {
		if ls, ok := chains[key].([]any); ok {
			links = append(links, ls...)
		}
	}
	for _, l := range links {
		if lm, ok := l.(map[string]any); ok && listHas(lm, host, key) {
			return true
		}
	}
	return false
}

// TRU-58c: a project baseline this build cannot read as a baseline -- bytes
// that do not parse, or a version it does not know -- cannot be rewritten
// without the host. When its bytes hold the host, forget --host deletes it
// and counts it as cleared, and the project's next session establishes the
// baseline again, as it does after a file without the basis marker.
func TestTRU58c_ForgetDeletesAnUnparseableBaselineHoldingTheHost(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(body string) string
	}{
		{"bytes that do not parse", func(body string) string {
			return body[:strings.Index(body, forgettable)+len(forgettable)+2]
		}},
		{"a version this build does not know", func(body string) string {
			return strings.Replace(body, "\"version\": 1,", "\"version\": 2,", 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			seedHostSession(t, e)
			if res := e.run("", nil, "report", "--session", testSession); res.exitCode != 0 {
				t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			path := baselineFile(t, e)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			broken := tc.damage(string(body))
			if broken == string(body) || !strings.Contains(broken, forgettable) {
				t.Fatalf("premise: the broken baseline must differ and still hold %q:\n%s", forgettable, broken)
			}
			if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
				t.Fatal(err)
			}

			res := e.run("", nil, "forget", "--host", forgettable)
			if res.exitCode != 0 {
				t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			if !strings.HasSuffix(res.stdout, "; 1 project baseline(s) cleared\n") {
				t.Errorf("forget --host does not count the baseline it deleted:\n%s", res.stdout)
			}
			if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the baseline is still on disk after the forget (stat: %v)", err)
			}
			for rel, f := range walkStore(t, e.home) {
				if strings.Contains(string(f.body), forgettable) {
					t.Errorf("%s still holds %q after forget --host", rel, forgettable)
				}
			}

			const next = "sess-tru58c-next"
			e.probe("start", next)
			e.declare(next, "curl https://keep-me.example/z")
			e.probe("end", next)
			if got := valueAt(e.sessionJSON(t, next), "destinations", "novelty", "established"); got != true {
				t.Errorf("the project's next session did not establish its baseline again (established = %v)", got)
			}
		})
	}
}

// baselineFile is the one project baseline a report wrote into the store.
func baselineFile(t *testing.T, e *env) string {
	t.Helper()
	dir := filepath.Join(e.home, "baseline")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The project files, not the lock file beside them.
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	if len(files) != 1 {
		t.Fatalf("premise: %d project files under baseline/, want the one project's", len(files))
	}
	return filepath.Join(dir, files[0])
}
