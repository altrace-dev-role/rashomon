package report

import "testing"

// TestDetectorIDs: the ten ids are distinct, in the stated order, and spelled
// as the corpus's fixture directories spell them.
func TestDetectorIDs(t *testing.T) {
	want := []string{
		"digest_unknown", "coverage_unverified", "digest_truncated", "silent_failure",
		"masked_run", "tests_only_then_green", "flaky_pair", "declaration_without_execution",
		"novel_destination", "executed_differently_from_declared",
	}
	got := DetectorIDs()
	if len(got) != len(want) {
		t.Fatalf("%d ids, want %d", len(got), len(want))
	}
	for i, id := range got {
		if string(id) != want[i] {
			t.Errorf("id %d = %q, want %q", i, id, want[i])
		}
	}
}

// TestDestinationsFired: the two report-only detectors' fire conditions. A
// baseline just established, or unreadable, is not a novel destination, and
// an unknown comparison is not a difference.
func TestDestinationsFired(t *testing.T) {
	for name, tc := range map[string]struct {
		d                Destinations
		novel, different bool
	}{
		"nothing":              {Destinations{}, false, false},
		"new host":             {Destinations{Novelty: Novelty{Available: true, Hosts: []string{"example.org"}}}, true, false},
		"established baseline": {Destinations{Novelty: Novelty{Available: true, Established: true, Hosts: []string{"example.org"}}}, false, false},
		"unreadable baseline":  {Destinations{Novelty: Novelty{Hosts: []string{"example.org"}}}, false, false},
		"available, none new":  {Destinations{Novelty: Novelty{Available: true, KnownHosts: 3}}, false, false},
		"executed differently": {Destinations{ExecutedNotAsDeclared: 1}, false, true},
		"both":                 {Destinations{ExecutedNotAsDeclared: 2, Novelty: Novelty{Available: true, Hosts: []string{"a"}}}, true, true},
	} {
		if got := tc.d.Fired(DetectorNovelDestination); got != tc.novel {
			t.Errorf("%s: novel_destination = %v, want %v", name, got, tc.novel)
		}
		if got := tc.d.Fired(DetectorExecutedDifferentlyFromDeclared); got != tc.different {
			t.Errorf("%s: executed_differently_from_declared = %v, want %v", name, got, tc.different)
		}
		if tc.d.Fired(DetectorSilentFailure) {
			t.Errorf("%s: a digest-level id fired on destinations", name)
		}
	}
}
