package gather

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestBaselineErrorReason_DistinctCausesDistinctMessages: a locked baseline,
// a session naming more hosts than a baseline can hold, and a corrupt or
// unreadable file are three different situations, and a reader who sees "the
// project baseline could not be read or written" for all three cannot tell a
// transient contention from a reason to go look at the file. The first two
// have their own named errors (baseline.ErrTooManyHosts, store.ErrLockTimeout)
// precisely so a caller can tell them apart; only the generic message is
// deliberately silent about which file.
func TestBaselineErrorReason_DistinctCausesDistinctMessages(t *testing.T) {
	tooMany := baselineErrorReason(baseline.ErrTooManyHosts)
	locked := baselineErrorReason(store.ErrLockTimeout)
	generic := baselineErrorReason(errors.New("baseline: unrecognised file version"))
	for _, pair := range [][2]string{{tooMany, locked}, {tooMany, generic}, {locked, generic}} {
		if pair[0] == pair[1] {
			t.Errorf("two distinct baseline errors produced the same reason %q", pair[0])
		}
	}
	if generic == "" || strings.Contains(generic, "unrecognised file version") {
		t.Errorf("generic reason %q: must say something, and must not repeat the error text, "+
			"which can carry the baseline's path", generic)
	}
}

// TestNovelty_TooManyHostsSaysSoNotTheGenericMessage: the integration path,
// not just the mapping function above -- a session that names more hosts
// than a baseline holds must reach the reader as something more specific
// than the generic "could not be read or written", which a lock timeout and
// a corrupt file also produce.
func TestNovelty_TooManyHostsSaysSoNotTheGenericMessage(t *testing.T) {
	hosts := make([]string, 5000)
	for i := range hosts {
		hosts[i] = fmt.Sprintf("h%d.example", i)
	}
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash", SessionID: "sess-1", Hosts: hosts,
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: t.TempDir(), SessionID: "sess-1",
		}},
	}
	n := novelty(run, t.TempDir(), nil)
	if n.Available {
		t.Fatal("novelty reports itself available for a session past the baseline's host cap")
	}
	if n.Reason == "the project baseline could not be read or written" {
		t.Errorf("reason %q: a session naming too many hosts reads identically to a locked "+
			"or corrupt baseline", n.Reason)
	}
}

// TestRubricHLD4_TheBaselinesAnswerCarriesNoErrorText: the baseline's error
// reaches the read step as it came, path and all, and what the read step
// hands Assemble is a reason, never the error's text.
func TestRubricHLD4_TheBaselinesAnswerCarriesNoErrorText(t *testing.T) {
	const text = "open /home/someone/baseline.json: permission denied"
	n := report.AnsweredNovelty(false, 0, 0, nil, baselineErrorReason(errors.New(text)))
	if n.Available || n.Reason == "" || strings.Contains(n.Reason, "someone") || strings.Contains(n.Reason, "denied") {
		t.Errorf("novelty = %+v, want unavailable with a reason that is not %q", n, text)
	}
}

// TestRubricLLD11_NoveltyLegacyIPLiteralDeclarationIsNotNovelAgain: a run recorded
// before host.Canonical folded an IP literal to its one spelling holds the
// uncanonicalised string. Read today, under the one spelling a current
// build now writes for the same host, it must not read as a new
// destination.
func TestRubricLLD11_NoveltyLegacyIPLiteralDeclarationIsNotNovelAgain(t *testing.T) {
	project := t.TempDir()
	root := t.TempDir()
	mk := func(id string, at int64, hosts ...string) *store.Run {
		return &store.Run{
			Declarations: []store.Declaration{{
				ToolUseID: "t", ToolName: "Bash", SessionID: id, Hosts: hosts,
			}},
			Coverage: []store.Coverage{{
				Phase: store.PhaseStart, RecordedAtMS: at, CWD: project, SessionID: id,
			}},
		}
	}
	// A pre-upgrade session recorded the host in its raw, uncanonicalised
	// form.
	novelty(mk("sess-1", t0ms, "167838211"), root, nil)

	// A current session names the same host in its canonical spelling.
	n := novelty(mk("sess-2", t0ms+3600000, "10.1.2.3"), root, nil)

	if len(n.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty: 167838211 and 10.1.2.3 are one host", n.Hosts)
	}
}

// TestNovelty_RerenderingAPreUpgradeRunIsNotNovel: the reverse order from
// above -- a CURRENT session establishes the baseline under a host's
// canonical spelling, and an OLDER run, stored before host.Canonical
// existed, is rendered afterwards holding the host's raw, uncanonicalised
// form. declaredHosts has to canonicalise what it reads out of that old
// record, or the same host reads as a second, unknown one every time that
// old run is rendered again.
func TestNovelty_RerenderingAPreUpgradeRunIsNotNovel(t *testing.T) {
	project := t.TempDir()
	root := t.TempDir()
	mk := func(id string, at int64, hosts ...string) *store.Run {
		return &store.Run{
			Declarations: []store.Declaration{{
				ToolUseID: "t", ToolName: "Bash", SessionID: id, Hosts: hosts,
			}},
			Coverage: []store.Coverage{{
				Phase: store.PhaseStart, RecordedAtMS: at, CWD: project, SessionID: id,
			}},
		}
	}
	novelty(mk("sess-new", t0ms, "10.1.2.3"), root, nil)

	n := novelty(mk("sess-old", t0ms+3600000, "167838211"), root, nil)

	if len(n.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty: 167838211 is the same host as 10.1.2.3", n.Hosts)
	}
}
