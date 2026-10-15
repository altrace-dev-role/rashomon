// Peak RSS is read from the child's Unix rusage (Maxrss), which Windows'
// syscall.Rusage does not have; GOOS=windows go vet ./... failed here.

//go:build !windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/peakrss"
)

const res8Child = "RASHOMON_RES8_CHILD"

// buildReportStore creates a store at root with n sessions, each one
// declaration naming a transcript whose final assistant message is
// finalBytes long -- enough data that holding every session's transcript at
// once, rather than one at a time, is measurable in peak RSS.
func buildReportStore(t *testing.T, root string, n, finalBytes int) {
	t.Helper()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	final := strings.Repeat("a", finalBytes)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("res8-%d", i)
		path := filepath.Join(root, "transcripts", id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		lines := []string{
			`{"message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_` + id + `"}]}}`,
			`{"message":{"role":"assistant","content":[{"type":"text","text":"` + final + `"}]}}`,
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := st.AppendDeclaration(store.Declaration{
			Type: "declaration", SchemaVersion: 2, SessionID: id, ToolUseID: "toolu_" + id,
			ToolName: "Bash", TranscriptPath: path, RecordedAtMS: time.Now().UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// reportMaxRSS runs `rashomon report --json` over the store at home in a
// child process through reaper, and returns its peak RSS in MiB.
func reportMaxRSS(t *testing.T, reaper *peakrss.Reaper, home string) float64 {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRES8_ReportPeakHeapStaysFlatAsSessionCountGrows$")
	cmd.Env = append(os.Environ(), res8Child+"=1", "RASHOMON_HOME="+home)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.Discard, &out
	rss, err := reaper.Run(cmd)
	if err != nil {
		t.Fatalf("report child: %v\n%s", err, out.String())
	}
	return rss
}

// TestRES8_ReportPeakHeapStaysFlatAsSessionCountGrows: a report over 60
// sessions costs what one over 3 does plus the transcript buffers for the
// session Assemble is rendering, and peak RSS does not scale with the store's
// history. Read loading every session's transcript, ID set and final text
// before Assemble renders any of them breaks this: peak RSS there scales with
// the number of sessions.
func TestRES8_ReportPeakHeapStaysFlatAsSessionCountGrows(t *testing.T) {
	if os.Getenv(res8Child) == "1" {
		run([]string{"report", "--json"}, strings.NewReader(""), io.Discard, io.Discard)
		os.Exit(0)
	}
	if testing.Short() {
		t.Skip("runs report in child processes")
	}
	reaper, err := peakrss.NewReaper(t.TempDir())
	if errors.Is(err, peakrss.ErrUnavailable) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	const finalBytes = 500_000 // 500 KB per session's final assistant message

	fewHome := t.TempDir()
	buildReportStore(t, fewHome, 3, finalBytes)
	few := reportMaxRSS(t, reaper, fewHome)

	manyHome := t.TempDir()
	buildReportStore(t, manyHome, 60, finalBytes)
	many := reportMaxRSS(t, reaper, manyHome)

	t.Logf("report peak RSS: %.2f MiB at 3 sessions, %.2f MiB at 60 sessions (500 KB each)", few, many)
	// 60 sessions hold 30 MB of final text against 3 sessions' 1.5 MB if every
	// session is read before any is assembled. Streaming one session at a
	// time costs only the one transcript currently being read and rendered,
	// so the growth bound is far under that 28.5 MB difference; 12 MiB leaves
	// headroom for GC and runner noise without hiding the defect, which this
	// test failed against by over 20 MiB before gather.Build read one session
	// at a time.
	growthBound := 12.0
	if many-few > growthBound {
		t.Errorf("60 sessions cost %.2f MiB more than 3 did (500 KB final text each); want under %.1f MiB -- Read is holding every session at once", many-few, growthBound)
	}
}
