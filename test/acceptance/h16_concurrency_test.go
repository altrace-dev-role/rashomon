package acceptance

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
)

// TestH16_ConcurrentHandlersAllLand runs 64 handlers at once, then again with
// every record padded past the 4 KiB a buffered writer would flush at. Every
// call must be recorded or accounted for: a declaration, or a spilled
// lock_timeout terminal naming it (the budget is wall clock, so a machine
// loaded enough to starve the lock holder drops declarations by design); every
// call has a terminal; and the ordered records carry distinct ids and a
// contiguous seq -- the invariant the lock exists to protect, and the one that
// breaks when the lock is removed.
//
// The padding is the transcript path, which REC-52 caps at 4096 bytes
// (store.MaxPathBytes): a path over the cap is withheld, and the record would
// then be short. 4000 bytes of it put the declaration past 4 KiB and leave
// the path under its cap.
func TestH16_ConcurrentHandlersAllLand(t *testing.T) {
	for _, pad := range []int{0, 4000} {
		t.Run(fmt.Sprintf("padding=%d", pad), func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)

			const n = 64
			want := map[string]bool{}
			var wg sync.WaitGroup
			failures := make(chan string, n)
			for i := 0; i < n; i++ {
				id := fmt.Sprintf("toolu_%02d", i)
				want[id] = true
				p := defaultPayload()
				p.ToolUseID = id
				if pad > 0 {
					p.TranscriptPath = "/tmp/" + strings.Repeat("p", pad) + ".jsonl"
				}
				payload := p.build(t)

				wg.Add(1)
				go func() {
					defer wg.Done()
					cmd := exec.Command(rashomonBin, "hook")
					cmd.Stdin = strings.NewReader(payload)
					cmd.Env = e.environ()
					cmd.Dir = e.cwd
					var stderr bytes.Buffer
					cmd.Stderr = &stderr
					_ = cmd.Run()
					if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 {
						failures <- fmt.Sprintf("%s: exit %v stderr %q", id, cmd.ProcessState, stderr.String())
					}
				}()
			}
			wg.Wait()
			close(failures)
			for f := range failures {
				t.Error(f)
			}

			// The invariant is accounting, not landing: under enough CPU
			// load a handler's declaration waits out the two-second lock
			// budget (wall clock, so a starved lock holder costs it), and
			// the design then drops the declaration and spills a terminal
			// naming the call, with lock_timeout coverage. That is an honest
			// loss, tested deterministically by TestRubricTEST4_. What may
			// never happen is a call with no trace.
			decls := e.declarations(testSession)
			terms := e.terminals(testSession)

			got := map[string]bool{}
			for _, d := range decls {
				got[d.str("tool_use_id")] = true
				if pad > 0 && len(d.str("transcript_path")) != pad+len("/tmp/.jsonl") {
					t.Errorf("padding did not reach the record: transcript_path is %d bytes", len(d.str("transcript_path")))
				}
				if pad > 0 && len(d.raw) <= 4096 {
					t.Errorf("a padded declaration is %d bytes, not past the 4 KiB a buffered writer flushes at", len(d.raw))
				}
			}
			for id := range got {
				if !want[id] {
					t.Errorf("declaration %s was never sent", id)
				}
			}
			termOf := map[string]record{}
			for _, tr := range terms {
				termOf[tr.str("tool_use_id")] = tr
			}
			lost := 0
			for id := range want {
				tr, ok := termOf[id]
				if !ok {
					t.Errorf("call %s has no terminal: no trace at all", id)
					continue
				}
				if got[id] {
					continue
				}
				lost++
				if tr.str("reason") != "lock_timeout" || tr.str("outcome") != "error" || tr.fields["seq"] != nil {
					t.Errorf("call %s has no declaration and its terminal does not account for it: %s", id, tr.raw)
				}
			}
			// A lost call must never read as covered: verified call
			// coverage records can be no more than the declarations that
			// landed (an absent coverage record reads as unverified, H-10).
			verified := 0
			for _, c := range e.coverage(testSession, "call") {
				if c.str("state") == "verified" {
					verified++
				}
			}
			if verified > len(decls) {
				t.Errorf("%d verified call coverage records for %d landed declarations (%d lost)", verified, len(decls), lost)
			}
			if lost > 0 {
				t.Logf("%d of %d declarations lost to the lock budget under load; each accounted for by a lock_timeout terminal", lost, n)
			}

			// The ordered stream's own invariant, which load cannot excuse:
			// distinct ids and a contiguous seq over the records that took
			// the lock. A spilled terminal has no seq by design.
			var seqs []int
			for _, r := range e.read(testSession, "records.ndjson") {
				if r.str("type") != "declaration" && r.str("type") != "terminal" {
					continue
				}
				s, ok := r.fields["seq"].(float64)
				if !ok {
					t.Errorf("ordered record without a numeric seq: %s", r.raw)
					continue
				}
				seqs = append(seqs, int(s))
			}
			sort.Ints(seqs)
			for i, s := range seqs {
				if s != i+1 {
					t.Fatalf("seq is not contiguous and distinct: at position %d found %d (sorted seqs: %v...)", i, s, seqs[:min(len(seqs), 10)])
				}
			}
		})
	}
}
