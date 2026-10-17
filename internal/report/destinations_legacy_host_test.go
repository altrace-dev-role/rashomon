package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestNovelty_LegacyIPLiteralDeclarationIsNotNovelAgain: a run recorded
// before host.Canonical folded an IP literal to its one spelling holds the
// uncanonicalised string. Read today, under the one spelling a current
// build now writes for the same host, it must not read as a new
// destination.
func TestNovelty_LegacyIPLiteralDeclarationIsNotNovelAgain(t *testing.T) {
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
	buildDestinations(mk("sess-1", t0ms, "167838211"), root, nil)

	// A current session names the same host in its canonical spelling.
	d := buildDestinations(mk("sess-2", t0ms+3600000, "10.1.2.3"), root, nil)

	if len(d.Novelty.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty: 167838211 and 10.1.2.3 are one host", d.Novelty.Hosts)
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
	buildDestinations(mk("sess-new", t0ms, "10.1.2.3"), root, nil)

	d := buildDestinations(mk("sess-old", t0ms+3600000, "167838211"), root, nil)

	if len(d.Novelty.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty: 167838211 is the same host as 10.1.2.3", d.Novelty.Hosts)
	}
}
