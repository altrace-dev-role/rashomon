package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

const res7Child = "RASHOMON_RES7_CHILD"

// stopPayload is a Stop payload whose final message is n bytes of text.
func stopPayload(n int) []byte {
	var b bytes.Buffer
	b.WriteString(`{"session_id":"s-res7","hook_event_name":"Stop","last_assistant_message":"`)
	line := "All 412 tests pass; the build is green and nothing was skipped.\\n"
	for b.Len() < n {
		b.WriteString(line)
	}
	b.WriteString(`"}`)
	return b.Bytes()
}

// recapMaxRSS runs recap in a child process on payload and returns its peak
// RSS in MiB.
func recapMaxRSS(t *testing.T, payload []byte) float64 {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRES7_HugeFinalMessageIsConstant$")
	cmd.Env = append(os.Environ(), res7Child+"=1", "RASHOMON_HOME="+t.TempDir())
	cmd.Stdin = bytes.NewReader(payload)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("recap child: %v\n%s", err, out.String())
	}
	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Fatal("no rusage for the child")
	}
	rss := float64(ru.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return rss / (1 << 20)
}

// TestRES7_HugeFinalMessageIsConstant: recap streams a Stop payload, so a
// 7 MB final message costs what a 2 KiB one does plus the capture buffers,
// and peak RSS stays within 12 MiB. Reading it whole cost 56 MiB. Break: read
// the Stop payload through hookio.Intake, and the message is held whole.
func TestRES7_HugeFinalMessageIsConstant(t *testing.T) {
	if os.Getenv(res7Child) == "1" {
		run([]string{"recap"}, os.Stdin, io.Discard, io.Discard)
		os.Exit(0)
	}
	if testing.Short() {
		t.Skip("runs recap in child processes")
	}
	small := recapMaxRSS(t, stopPayload(2<<10))
	big := recapMaxRSS(t, stopPayload(7_000_000))
	t.Logf("recap peak RSS: %.2f MiB at a 2 KiB message, %.2f MiB at 7 MB", small, big)
	switch {
	case raceEnabled:
		t.Logf("race detector on: the 12 MiB bound is not applied to an instrumented child; the growth bound is")
	case big > 12:
		t.Errorf("recap at a 7 MB final message peaked at %.2f MiB, over 12 MiB", big)
	}
	if big-small > 4 {
		t.Errorf("a 7 MB message cost %.2f MiB more than a 2 KiB one: the payload is held", big-small)
	}
}

func TestReadFinalMessageKeepsTodaysAccountWindow(t *testing.T) {
	msg := strings.Repeat("é", 70_000) // 70,000 runes, 140,000 bytes: past 64 KiB, inside 64 Ki runes
	fm, ok := readFinalMessage(strings.NewReader(`{"session_id":"s","last_assistant_message":"`+msg+`"}`), true)
	if !ok || fm.SessionID != "s" || fm.Message != msg || fm.Tail != "" {
		t.Fatalf("ok=%v session %q head %d bytes tail %d bytes", ok, fm.SessionID, len(fm.Message), len(fm.Tail))
	}
	long := strings.Repeat("a", messageHeadBytes) + strings.Repeat("z", 100)
	fm, ok = readFinalMessage(strings.NewReader(`{"last_assistant_message":"`+long+`"}`), false)
	if !ok || len(fm.Message) != messageHeadBytes || !strings.HasSuffix(fm.Tail, strings.Repeat("z", 100)) || len(fm.Tail) != messageTailBytes {
		t.Fatalf("long message: head %d tail %d", len(fm.Message), len(fm.Tail))
	}
	for _, bad := range []string{`{"session_id":5,"last_assistant_message":"x"}`, `{"last_assistant_message":["x"]}`, `["x"]`, `{"last_assistant_message":"x"} {}`, ``} {
		if _, ok := readFinalMessage(strings.NewReader(bad), true); ok {
			t.Errorf("%q read as a payload", bad)
		}
	}
}
