package acceptance

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var updateGoldens = flag.Bool("update", false, "rewrite the testdata goldens from this run")

// reportGolden is report --json's committed form, byte for byte, with what a
// run cannot hold still -- times, paths, the install id -- in placeholders.
var reportGolden = filepath.Join("testdata", "report.golden.json")

// normalizeReport replaces what a run cannot hold still with placeholders:
// the temporary directories (dirs), every *_unix_ms time and the install id.
// Every other byte is compared as written.
func normalizeReport(out []byte, dirs ...string) []byte {
	for _, dir := range dirs {
		out = bytes.ReplaceAll(out, []byte(dir), []byte("<tmp>"))
	}
	out = regexp.MustCompile(`("[a-z_]*_unix_ms": )\d+`).ReplaceAll(out, []byte(`${1}0`))
	return regexp.MustCompile(`("install_id": )"[^"]*"`).ReplaceAll(out, []byte(`${1}"<install>"`))
}

// TestRubricNAMING3_ReportJSONIsTheGolden compares report --json with a
// committed golden, byte for byte. The test named for the JSON form compared
// the binary's output with the e.report helper's decoding of the same
// binary's output, so renaming a key -- redacted, programs_unknown,
// by_verb_class -- passed everything while breaking every consumer. A change
// to the document now shows as a diff to this file, made on purpose with
// -update, and with a report_version bump when CurrentReportVersion's rule
// calls for one.
func TestRubricNAMING3_ReportJSONIsTheGolden(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	ids := []string{"toolu_a", "toolu_b"}
	transcript := writeTranscript(t, t.TempDir(), testSession, ids, ids[:1], nil)
	e.hookIDs(transcript, ids...)
	e.postIDs(transcript, ids[0])
	// A second, unreadable transcript group. The report lists transcripts in
	// raw path order, so this path lives in the readable transcript's own
	// directory under a name that sorts after it. The fixed /tmp path the
	// default payload names sorted before macOS's /var/folders temp dirs and
	// after Linux's /tmp/Test... ones, so the golden held on one platform only.
	unreadable := defaultPayload()
	unreadable.TranscriptPath = filepath.Join(filepath.Dir(transcript), "zz-unreadable", "sess-1.jsonl")
	e.mustHook(unreadable.build(t))
	e.probe("end", testSession)

	res := e.run("", nil, "report", "--json", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report --json: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	got := normalizeReport([]byte(res.stdout), e.home, e.configDir, e.cwd, filepath.Dir(transcript))
	if *updateGoldens {
		// A removed key with no version bump would otherwise just become
		// the new committed truth, with nothing left to diff it against.
		if before, err := os.ReadFile(reportGolden); err == nil {
			if removed, blocked := guardDropsAKey(docKeyPaths(t, before), docKeyPaths(t, got),
				versionOf(before, "report_version"), versionOf(got, "report_version")); blocked {
				t.Fatalf("-update would drop %v from %s with report_version still %d; bump report.CurrentReportVersion first",
					removed, reportGolden, versionOf(got, "report_version"))
			}
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(reportGolden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(reportGolden)
	if err != nil {
		t.Fatalf("%v: run with -update to write it", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("report --json differs from %s (rewrite it with -update only for a deliberate change, and bump report.CurrentReportVersion):\n--- got\n%s\n--- want\n%s",
			reportGolden, got, want)
	}
}
