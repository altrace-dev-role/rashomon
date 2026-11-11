package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The fixture's words and places.
const (
	goalWords     = "make session expiry respect the org-level timeout override."
	r1Words       = "respect the org-level timeout override"
	decisionWords = "read the org value through get_org_settings(org_id); do not import orgs.models into auth."
	goalLine      = 3
	decisionLine  = 7
	sessionsPy    = "services/auth/auth/sessions.py"
	cachePy       = "services/auth/auth/cache.py"
	purgePy       = "jobs/purge_sessions.py"
	line1         = "dlv:S:5/1"
	line2         = "dlv:S:5/2"
)

// retention is a fixture's retention setting: bodies on copies the words at
// write time; an unreadable transcript is one the references cannot be
// read from.
type retention struct {
	bodies     bool
	unreadable bool
}

// fixture is the session over the specimen with its transcript.
type fixture struct {
	*Session
	sp         *graphtest.Specimen
	transcript string
}

// digest is the test's keyed digest stand-in.
func digest(words string) string {
	sum := sha256.Sum256([]byte("test\x00" + words))
	return hex.EncodeToString(sum[:])
}

// extract is the test's transcript line reader: message.content.
func extract(line []byte) (string, bool) {
	var rec struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &rec); err != nil {
		return "", false
	}
	return rec.Message.Content, true
}

// transcriptLines is the seven-line transcript: the goal on line 3 and the
// plan on line 7.
func transcriptLines() []string {
	user := func(s string) string { return `{"type":"user","message":{"role":"user","content":` + quote(s) + `}}` }
	asst := func(s string) string {
		return `{"type":"assistant","message":{"role":"assistant","content":` + quote(s) + `}}`
	}
	return []string{
		`{"type":"system","subtype":"init"}`,
		user("hi"),
		user(goalWords),
		asst("Reading services/auth first."),
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"tool result"}}`,
		asst("Three files are involved."),
		asst(decisionWords),
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// newFixture builds the session at C1 with the brief of trace line 5
// delivered: six lines, three of which have predicates here.
func newFixture(t *testing.T, r retention) *fixture {
	t.Helper()
	sp := graphtest.New(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(transcriptLines(), "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refPath := path
	if r.unreadable {
		refPath = filepath.Join(t.TempDir(), "gone.jsonl")
	}
	ret := func(id string, class Class, what, origin, words string, line, off int) *Ret {
		rt := &Ret{ID: id, Class: class, What: what, Origin: origin, Digest: digest(words), Bytes: len(words),
			Ref: &Ref{Path: refPath, Line: line, Off: off, Len: len(words)}}
		if r.bodies {
			rt.Copy = words
		}
		return rt
	}
	s := New("S", sp.Store, graphtest.Commit)
	s.Digest, s.Extract, s.CapBytes = digest, extract, 1<<20
	s.Goal = ret("ret:S:1", ClassUser, "goal", "prompt 1", goalWords, goalLine, 0)
	create := graphtest.Sym("services/auth", "auth.sessions.create_session")
	expired := graphtest.Sym("services/auth", "auth.sessions.is_expired")
	getOrg := graphtest.Sym("services/orgs", "orgs.settings.get_org_settings")
	ttl := graphtest.Const("services/auth", "auth.sessions.SESSION_TTL_SECONDS")
	s.Reqs = []*Req{
		{ID: "R1", Words: ret("ret:S:2", ClassUser, "req", "prompt 1", r1Words, goalLine, strings.Index(goalWords, r1Words)),
			Tier: graph.Declared, Conf: 1.0,
			Text: "Necessary condition: create_session or is_expired calls orgs.settings.get_org_settings",
			Pred: func(st *graph.Store, rev graph.Rev) bool { return callsAny(st, rev, getOrg, create, expired) }},
		{ID: "R2", Tier: graph.Inferred, Conf: 0.5,
			Text: "SESSION_TTL_SECONDS in services/auth/auth/sessions.py unchanged from C1",
			Pred: func(st *graph.Store, rev graph.Rev) bool {
				n, ok := st.Node(ttl, rev)
				return ok && n.Attrs["value"] == "3600"
			}},
	}
	for _, q := range s.Reqs {
		q.Holds = q.Pred(s.Store, s.Rev())
	}
	s.Decisions = []Decision{{Words: ret("ret:S:3", ClassModel, "dsn", "your plan, call 7", decisionWords, decisionLine, 0),
		Names: []string{"services/orgs/orgs/settings.py", sessionsPy}}}
	s.Gates = []*Gate{{Cmd: "pytest services/auth", Unit: "services/auth"}, {Cmd: "pytest jobs", Unit: "jobs"},
		{Cmd: "pytest tests/integration -k session"}}
	s.Delivered = []*Delivered{
		{ID: line1, Line: 1, Hidden: []string{"redis_ttl_matches"},
			Pred: func(s *Session) FollowedLabel { return wroteAll(s, sessionsPy, cachePy, purgePy) },
			Unmet: func(*Session) string {
				return "auth/cache.py put_session ex=3600 unchanged; no gate covers it; not owed"
			}},
		{ID: line2, Line: 2,
			Pred:  func(s *Session) FollowedLabel { return wroteAll(s, purgePy) },
			Unmet: func(*Session) string { return "jobs/purge_sessions.py unchanged" }},
		{ID: "dlv:S:5/3", Line: 3}, {ID: "dlv:S:5/4", Line: 4}, {ID: "dlv:S:5/5", Line: 5}, {ID: "dlv:S:5/6", Line: 6},
	}
	s.Deliver(5, line1, line2, "dlv:S:5/3", "dlv:S:5/4", "dlv:S:5/5", "dlv:S:5/6")
	return &fixture{Session: s, sp: sp, transcript: path}
}

// callsAny reports whether any of srcs calls dst at rev.
func callsAny(st *graph.Store, rev graph.Rev, dst graph.ID, srcs ...graph.ID) bool {
	for _, src := range srcs {
		for _, e := range st.Out(src, rev, graph.Calls) {
			if e.Dst == dst {
				return true
			}
		}
	}
	return false
}

// wroteAll is yes when every path was written, partial when some, no when
// none.
func wroteAll(s *Session, paths ...string) FollowedLabel {
	n := 0
	for _, p := range paths {
		for _, t := range s.Touched {
			if t.Path == p {
				n++
				break
			}
		}
	}
	switch {
	case n == len(paths):
		return FollowedYes
	case n == 0:
		return FollowedNo
	}
	return FollowedPartial
}

// edit applies one edit as an overlay at seq and records the write.
func (f *fixture) edit(t *testing.T, seq int64, rel, old, new string) {
	t.Helper()
	if _, err := f.sp.Overlay(rel, old, new, seq); err != nil {
		t.Fatalf("edit %d %s: %v", seq, rel, err)
	}
	f.Wrote(seq, rel)
}

// Trace line 6: three reads.
func (f *fixture) reads() {
	f.Opened(6, sessionsPy)
	f.Opened(6, cachePy)
	f.Opened(6, purgePy)
}

// The three edits of the trace, acts 12, 14 and 16.
const (
	oldCreate = "from auth.db import insert_session\n\nSESSION_TTL_SECONDS = 3600\n\n\ndef create_session(user, org):\n    expires_at = datetime.utcnow() + timedelta(seconds=SESSION_TTL_SECONDS)"
	newCreate = "from auth.db import insert_session\nfrom orgs.settings import get_org_settings\n\nSESSION_TTL_SECONDS = 3600\n\n\ndef create_session(user, org):\n    ttl = get_org_settings(org.id).session_timeout_s or SESSION_TTL_SECONDS\n    expires_at = datetime.utcnow() + timedelta(seconds=ttl)"
	oldPurge  = `    conn.execute("DELETE FROM public.sessions WHERE created_at < now() - interval '%s seconds'", SESSION_TTL_SECONDS)`
	newPurge  = `    conn.execute("DELETE FROM public.sessions WHERE expires_at < now()")`
)

func (f *fixture) editsThroughPurge(t *testing.T) {
	t.Helper()
	f.edit(t, 12, sessionsPy, oldCreate, newCreate)
	f.edit(t, 14, cachePy, "ex=3600)", "ex=ttl)")
	f.edit(t, 16, purgePy, oldPurge, newPurge)
}

func (f *fixture) editsSkippingCache(t *testing.T) {
	t.Helper()
	f.edit(t, 12, sessionsPy, oldCreate, newCreate)
	f.edit(t, 16, purgePy, oldPurge, newPurge)
}

// corruptTranscriptLine rewrites line n so its words no longer match.
func (f *fixture) corruptTranscriptLine(t *testing.T, n int) {
	t.Helper()
	lines := transcriptLines()
	lines[n-1] = `{"type":"user","message":{"role":"user","content":"make session expiry respect the org-level timeout OVERRIDE."}}`
	if err := os.WriteFile(f.transcript, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
