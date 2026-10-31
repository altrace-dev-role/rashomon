package acceptance

import (
	"strings"
	"testing"
)

// H-155 -- a run under a cd the hook does not read is reported as not
// credited (5.14b).
//
// `cd sub; make test 2>&1 | tail -40` starts in a directory the hook does not
// read (a `;` does not guard the cd), so its record holds only a keyed
// digest of an unread fold, which the report cannot open. The declaration
// carries one more key, location_unread, true, on that call alone; the report
// then names the run as not credited for that reason beside the masked
// count, never as a pass and never as zero. A call whose cd is read carries
// no such key.
//
// Break: never set the bit, or set it on every cd, and the line is missing or
// names the read run.
func TestH155_AnUnreadLocationIsRecordedAndReported(t *testing.T) {
	s := newTBSession(t)
	s.shell("cd sub; make test 2>&1 | tail -40", true, "")
	s.shell("cd sub && make lint 2>&1 | tail -40", true, "")

	decls := s.e.declarations(testSession)
	if len(decls) != 2 {
		t.Fatalf("%d declarations, want 2", len(decls))
	}
	if got := decls[0].fields["location_unread"]; got != true {
		t.Errorf("a run under `cd DIR;` carries location_unread %v, want true", got)
	}
	if _, ok := decls[1].fields["location_unread"]; ok {
		t.Errorf("a run under a read cd carries location_unread: %s", decls[1].raw)
	}
	for _, r := range s.e.records(testSession) {
		if r.typ() != "declaration" && strings.Contains(r.raw, "location_unread") {
			t.Errorf("a %s carries location_unread: %s", r.typ(), r.raw)
		}
	}

	out := s.e.run("", nil, "report", "--session", testSession).stdout
	for _, want := range []string{
		"masked exit status: 2 build or test call(s) recorded ok",
		"1 of these ran after a leading `cd` step the hook does not read (`cd DIR;`, a variable, quoting or `-`), so the directory they ran in is not known and they are not credited",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}
	js := s.e.run("", nil, "report", "--json", "--session", testSession).stdout
	if !strings.Contains(js, `"unread_location": 1`) {
		t.Errorf("report --json carries no unread_location of 1:\n%s", js)
	}
}
