package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// H-100 -- a torn read is surfaced, never silently absorbed.
//
// eachLine (internal/store/read.go) took no lock at all, which was safe for
// `report`: it runs after a session is effectively over, so there is nothing
// left mid-write to race. `digest` runs at Stop, concurrent with an in-flight
// PostToolUse from a backgrounded Bash or a subagent, so it can read a torn
// tail line and bump run.Skipped where report never could.
//
// The fix is two things, both exercised here: store.ReadRunConsistent takes
// a short, best-effort lock against the same flock a writer holds
// (store.go's lockFile) before reading each file, sized to digest's own 50ms
// budget rather than borrowed from the writer's 2s one; and Run.Skipped --
// which already counts a line that failed to parse -- is surfaced on the
// digest and forces Coverage unverified, rather than a torn line being
// silently absorbed into a count that still looks clean.
//
// Break, deterministically: a torn tail line -- what a write caught
// mid-flight leaves behind, whatever the cause -- is constructed directly.
// Before the fix, this line is simply skipped and nothing on the digest says
// so: Coverage reads verified and the two real records look like the whole
// story. After the fix, skipped_records is 1 and Coverage carries
// records_skipped.
func TestH100_ATornTailIsSurfacedNotSilentlyAbsorbed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))

	// A second, healthy call in the same turn, so the test can show the torn
	// line costs exactly what it should and nothing more: the other two real
	// records still land.
	p2 := defaultPayload()
	p2.ToolUseID = "toolu_2"
	e.mustHook(p2.build(t))
	post2 := defaultPost()
	post2.ToolUseID = "toolu_2"
	e.mustPost(post2.build(t))

	// Appended directly: what a write caught mid-flight leaves on disk,
	// regardless of how it got there. Deliberately cut inside a field name
	// so it cannot possibly parse as anything.
	recordsPath := filepath.Join(e.home, "runs", testSession, "records.ndjson")
	f, err := os.OpenFile(recordsPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"declaration","schema_version":2,"seq":99,"tool_use_`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	d := e.digest("--session", testSession, "--prompt", p.PromptID)
	if d.SkippedRecords != 1 {
		t.Fatalf("skipped_records = %d, want 1: the torn line must be counted, not silently "+
			"dropped", d.SkippedRecords)
	}
	if !e.hasCoverageReason(d, "records_skipped") {
		t.Errorf("reasons = %v, want records_skipped", d.Coverage.Reasons)
	}
	if d.Coverage.State == "verified" {
		t.Error("state = verified with a torn line sitting in this session's own read -- " +
			"exactly the silent absorption this item exists to prevent")
	}
	if d.Declarations.Recorded != 2 {
		t.Errorf("recorded = %d, want 2: the two real, complete records must still both be "+
			"counted despite the torn line", d.Declarations.Recorded)
	}
}

// TestH100_DigestUnderConcurrentWriters is the "run a concurrent appender"
// case in the more literal sense: real hook/post invocations racing real
// digest reads on one store. Which of them, if any, actually catches a
// line mid-write is a timing accident this test does not try to force or
// assert on -- what it asserts is the property that must hold regardless:
// none of them errors, panics, or deadlocks while a real store is under real
// concurrent load, and every digest call that returns is internally
// consistent (a non-zero skipped_records always also marks coverage
// unverified, never absorbed silently -- the same rule TestH100_ATornTail...
// checks deterministically, here checked against whatever timing produces).
//
// The overall bound is generous on purpose: with dozens of real OS processes
// contending for CPU at once, wall-clock time is dominated by process-spawn
// scheduling, not by this package's own lock budget -- so it catches an
// actual deadlock or hang, not a benchmark of readLockBudget.
func TestH100_DigestUnderConcurrentWriters(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	const writers = 20
	var wg sync.WaitGroup
	failures := make(chan string, writers*2+8)

	runOnce := func(sub string, payload string) {
		cmd := exec.Command(rashomonBin, append([]string{sub}, e.installArgs()...)...)
		cmd.Stdin = strings.NewReader(payload)
		cmd.Env = e.environ()
		cmd.Dir = e.cwd
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		_ = cmd.Run()
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 {
			failures <- fmt.Sprintf("%s: exit %v stderr %q", sub, cmd.ProcessState, stderr.String())
		}
	}

	for i := 0; i < writers; i++ {
		id := fmt.Sprintf("toolu_c%03d", i)
		p := defaultPayload()
		p.ToolUseID = id
		payload := p.build(t)
		post := defaultPost()
		post.ToolUseID = id
		postPayload := post.build(t)

		wg.Add(1)
		go func() {
			defer wg.Done()
			runOnce("hook", payload)
			runOnce("post", postPayload)
		}()
	}

	// A FIXED number of digest reads, interleaved with the writers above,
	// rather than a tight loop for their whole duration -- the point is to
	// land some digest calls mid-write, not to maximise subprocess count.
	const readers = 8
	var digestWG sync.WaitGroup
	for i := 0; i < readers; i++ {
		digestWG.Add(1)
		go func() {
			defer digestWG.Done()
			res := e.digestRaw("--session", testSession, "--prompt", "prompt-1")
			if res.exitCode != 0 {
				failures <- fmt.Sprintf("digest: exit %d stderr %q", res.exitCode, res.stderr)
				return
			}
			var d digestOutput
			if err := json.Unmarshal([]byte(res.stdout), &d); err != nil {
				failures <- fmt.Sprintf("digest output is not JSON: %v\n%s", err, res.stdout)
				return
			}
			if d.SkippedRecords > 0 && !e.hasCoverageReason(d, "records_skipped") {
				failures <- fmt.Sprintf("skipped_records = %d but coverage carries no "+
					"records_skipped reason: %v -- a skip under real contention silently "+
					"absorbed instead of surfaced", d.SkippedRecords, d.Coverage.Reasons)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		digestWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent writers and digest reads did not finish within 30s -- a real hang " +
			"or deadlock, not a slow machine: everything here is bounded well under that")
	}
	close(failures)
	for f := range failures {
		t.Error(f)
	}
}
