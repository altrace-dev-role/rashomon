package slot

import (
	"strings"
	"testing"
)

// TestGS1_RelPathClimbsAtMostFourDirectories (card GRAPH-SLOT1): a graph
// line's path is relative to the hook's cwd, so it may open with "../" up
// to four times and then obeys every Path rule; a bare "..", a fifth hop,
// an absolute path and a secret-shaped segment are refused.
func TestGS1_RelPathClimbsAtMostFourDirectories(t *testing.T) {
	checkSlot(t,
		[]RelPath{"jobs/purge_sessions.py", "../acme-ops/jobs/purge_sessions.py", "../../../../a.py", "a"},
		[]RelPath{"", "..", "../", "../../../../../a.py", "/abs/a.py", "a/../b.py", "./a.py", "-x/a.py",
			RelPath("../x/" + strings.Repeat("deadbeef", 5) + ".py")})
	if RelPath("../acme-ops/jobs/purge_sessions.py").Text() != "'../acme-ops/jobs/purge_sessions.py'" {
		t.Error("a relative path renders outside a quote")
	}
}

// TestGS1_IdentIsDottedIdentifierSegments: a symbol, module, table or
// column name is one to eight identifier segments joined by dots, screened
// for secrets, and renders inside a quote.
func TestGS1_IdentIsDottedIdentifierSegments(t *testing.T) {
	checkSlot(t,
		[]Ident{"run", "public.sessions", "orgs.settings.get_org_settings", "_x", "SESSION_TTL_SECONDS", "a.b.c.d.e.f.g.h"},
		[]Ident{"", "1abc", "a-b", "a b", "a..b", ".a", "a.", "a.b.c.d.e.f.g.h.i", Ident(strings.Repeat("a", 65)),
			"AKIAABCDEFGHIJKLMNOP"})
	if Ident("run").Text() != "'run'" {
		t.Error("an identifier renders outside a quote")
	}
}

// TestGS1_KnowledgeIDKeepsLessonIDsScreensAndAdmitsUpperCase: a graph
// knowledge id is LessonID's shape with upper case, screened the same way,
// and renders after its kind prefix.
func TestGS1_KnowledgeIDKeepsLessonIDsScreensAndAdmitsUpperCase(t *testing.T) {
	checkSlot(t,
		[]KnowledgeID{"les:INC-212", "dec:ADR-0007", "inc-212", "L7"},
		[]KnowledgeID{"", "-x", "les:build.example.com", "https://x", "les:system", "a b", KnowledgeID(strings.Repeat("a", 65))})
	if KnowledgeID("les:INC-212").Text() != "INC-212" || KnowledgeID("inc-212").Text() != "inc-212" {
		t.Error("a knowledge id renders with its kind prefix")
	}
}

// TestGS1_RevisionShape: a graph revision is a short hash or an overlay
// revision; a 40-hex full hash is secret-shaped by the entropy rule.
func TestGS1_RevisionShape(t *testing.T) {
	checkSlot(t,
		[]Revision{"C1", "C1+ov16", "50942026abcd"},
		[]Revision{"", "+x", "a b", Revision(strings.Repeat("a", 42)), "5094202650942026509420265094202650942026"})
}

func inc212() KnowledgeFact {
	return KnowledgeFact{Kind: KnowledgeLesson, ID: "les:INC-212", Path: "../acme-ops/jobs/purge_sessions.py",
		Clause: "the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too",
		Source: "incidents/INC-212.md"}
}

func purgeLine() GraphLine {
	return NewCodeLine(CodeFact{Path: "../acme-ops/jobs/purge_sessions.py", Names: []Ident{"run"},
		Writes:  []TableRef{{Table: "public.sessions", Cols: []Ident{"created_at"}}},
		Imports: []Ident{"auth.sessions"}, Lessons: []KnowledgeID{"les:INC-212"}})
}

// TestGS1_LinesRenderAsOneSentenceEach: a code line is its path, then what
// it declares, calls, writes, reads and imports, then the records that
// apply; a table line names its writers and readers; a knowledge line is
// its kind, id, path, clause and source; an empty line lists nothing.
func TestGS1_LinesRenderAsOneSentenceEach(t *testing.T) {
	for _, c := range []struct {
		name string
		line GraphLine
		want string
	}{
		{"code", purgeLine(),
			"'../acme-ops/jobs/purge_sessions.py' declares 'run'; writes 'public.sessions' ('created_at'); imports 'auth.sessions'; INC-212 applies"},
		{"code with constants and calls", NewCodeLine(CodeFact{Path: "services/auth/auth/sessions.py",
			Names: []Ident{"create_session", "is_expired"}, Consts: []ConstFact{{Name: "SESSION_TTL_SECONDS", Value: 3600}},
			Calls: []Ident{"insert_session"}, Reads: []TableRef{{Table: "org_settings"}}, Lessons: []KnowledgeID{"les:INC-212", "dec:ADR-0007"}}),
			"'services/auth/auth/sessions.py' declares 'create_session', 'is_expired'; 'SESSION_TTL_SECONDS' is 3600; calls 'insert_session'; reads 'org_settings'; INC-212, ADR-0007 apply"},
		{"bare code", NewCodeLine(CodeFact{Path: "a.py"}), "'a.py'"},
		{"table", NewTableLine(TableFact{Table: "public.sessions", Writers: []Ident{"purge_sessions.run", "sessions.create_session"},
			Readers: []Ident{"sessions.is_expired"}, Lessons: []KnowledgeID{"les:INC-212"}}),
			"table 'public.sessions' is written by 'purge_sessions.run', 'sessions.create_session'; is read by 'sessions.is_expired'; INC-212 applies"},
		{"knowledge", NewKnowledgeLine(inc212()),
			"lesson INC-212 applies to '../acme-ops/jobs/purge_sessions.py': rule 'the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too'; source 'incidents/INC-212.md'"},
		{"knowledge without path or source", NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeDecision, ID: "dec:ADR-0007", Clause: "auth reads org values through get_org_settings"}),
			"decision ADR-0007: decided 'auth reads org values through get_org_settings'"},
		{"empty", GraphLine{}, "none"},
	} {
		v, err := Bind(c.line)
		if err != nil {
			t.Fatalf("%s: Bind: %v", c.name, err)
		}
		if got := v.Text(); got != c.want {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, got, c.want)
		}
		if lists := v.(GraphLine).Lists(); lists != (c.name != "empty") {
			t.Errorf("%s: Lists() = %v", c.name, lists)
		}
	}
}

// TestGS1_BindLeavesOutAndCountsWhatAChecksRefuses: an identifier, a table
// or a record id a check refuses is left out and counted into "and k more";
// a list past its cap is cut and counted; the path a code line stands on,
// the table a table line stands on and a knowledge line's id refuse the
// line; a knowledge line's path, clause and source are each left out alone.
func TestGS1_BindLeavesOutAndCountsWhatAChecksRefuses(t *testing.T) {
	v, err := Bind(NewCodeLine(CodeFact{Path: "a.py", Names: []Ident{"a", "b", "c", "d", "bad name"},
		Calls: []Ident{"AKIAABCDEFGHIJKLMNOP"}, Lessons: []KnowledgeID{"les:ok", "les:system"}}))
	if err != nil {
		t.Fatal(err)
	}
	want := "'a.py' declares 'a', 'b', 'c' and 2 more; calls 1 callees; ok and 1 more apply"
	if got := v.Text(); got != want {
		t.Errorf("left out and cut:\n got %q\nwant %q", got, want)
	}
	if m := v.(GraphLine).More(); m.Names != 2 || m.Calls != 1 || m.Lessons != 1 {
		t.Errorf("More() = %+v", m)
	}
	for _, c := range []struct {
		name string
		line GraphLine
	}{
		{"secret path", NewCodeLine(CodeFact{Path: "x/AKIAABCDEFGHIJKLMNOP.py"})},
		{"absolute path", NewCodeLine(CodeFact{Path: "/etc/a.py"})},
		{"bad table", NewTableLine(TableFact{Table: "a b"})},
		{"bad id", NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeLesson, ID: "les:system"})},
		{"bad kind", NewKnowledgeLine(KnowledgeFact{Kind: "rumor", ID: "les:x"})},
		{"two facts", GraphLine{code: &CodeFact{Path: "a.py"}, table: &TableFact{Table: "t"}}},
	} {
		if _, err := Bind(c.line); err == nil {
			t.Errorf("%s: bound", c.name)
		}
	}
	k := inc212()
	k.Path, k.Source = "/abs.py", "docs/AKIAABCDEFGHIJKLMNOP.md"
	k.Clause = Approved(strings.Repeat("x", 161))
	v, err = Bind(NewKnowledgeLine(k))
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Text(); got != "lesson INC-212" {
		t.Errorf("a knowledge line with every optional field refused = %q", got)
	}
}

// TestGS1_RecordFormHoldsDigestsCountsAndIds: the record form holds the
// path and the source as keyed digests and every list as its count, keeps
// the knowledge ids and the clause, and renders as counts; a record form
// that lists a name or a path is refused.
func TestGS1_RecordFormHoldsDigestsCountsAndIds(t *testing.T) {
	digest := func(s string) string { return strings.Repeat("ab", 32) }
	v, err := Bind(purgeLine())
	if err != nil {
		t.Fatal(err)
	}
	r := v.(GraphLine).Record(digest)
	if !r.Keyed() || r.Digest() != strings.Repeat("ab", 32) || r.Kind() != "code" {
		t.Fatalf("record = keyed %v digest %q kind %q", r.Keyed(), r.Digest(), r.Kind())
	}
	if m := r.More(); m != (Counts{Names: 1, Writes: 1, Imports: 1}) {
		t.Errorf("record counts = %+v", m)
	}
	if c, _ := r.Code(); c.Path != "" || len(c.Names) != 0 || len(c.Lessons) != 1 {
		t.Errorf("record code fact = %+v", c)
	}
	rv, err := Bind(r)
	if err != nil {
		t.Fatalf("Bind(record): %v", err)
	}
	if want := "a file on record declares 1 names; writes 1 tables; imports 1 modules; INC-212 applies"; rv.Text() != want {
		t.Errorf("record text\n got %q\nwant %q", rv.Text(), want)
	}
	kv, _ := Bind(NewKnowledgeLine(inc212()))
	kr := kv.(GraphLine).Record(digest)
	if kr.Digest() == "" || kr.SourceDigest() == "" {
		t.Errorf("knowledge record digests = %q %q", kr.Digest(), kr.SourceDigest())
	}
	if got := kr.Text(); !strings.HasPrefix(got, "lesson INC-212 applies to a file on record: rule 'the nightly") || !strings.HasSuffix(got, "; source on record") {
		t.Errorf("knowledge record text = %q", got)
	}
	bad := GraphLine{keyed: true, code: &CodeFact{Path: "a.py"}}
	if _, err := Bind(bad); err == nil {
		t.Error("a record form holding a path was bound")
	}
	if _, err := Bind(GraphLine{keyed: true, digest: "zz", code: &CodeFact{}}); err == nil {
		t.Error("a record form with a digest that is not 64 hex was bound")
	}
}
