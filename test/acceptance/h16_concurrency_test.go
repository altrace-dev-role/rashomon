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
// every record padded past the 4 KiB a buffered writer would flush at. All 64
// terminals must land, every declaration must either land or be accounted for
// by its terminal's lock_timeout, and the ordered stream's seq must be
// contiguous and distinct -- the invariant the lock exists to protect, and the
// one that breaks when the lock is removed.
//
// A declaration may give up. The design resolves H-16's tension that way
// (docs/design-notes.md): the append lock is waited on for lockBudget, two
// seconds, against the five-second hook timeout, and on give-up the
// declaration is dropped and its terminal goes to spill.ndjson under the same
// tool_use_id, with reason lock_timeout (hook.Handler.Close). Sixty-four
// processes that fsync inside the lock fit that budget on an idle disk, and
// not on a runner whose disk stalls (store/lockbudget.go says as much), so
// demanding all 64 declarations made this test a measure of the runner's disk.
// What stays demanded is that nothing vanishes: a declaration that is not in
// the store has a terminal that says why.
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

			decls := e.declarations(testSession)
			terms := e.terminals(testSession)
			if len(terms) != n {
				t.Fatalf("got %d terminals, want %d: a handler's terminal must land in the stream or the spill file", len(terms), n)
			}
			// The ids whose declaration gave up on the lock: their terminal, in
			// the spill file, is the only record they have.
			gaveUp := map[string]bool{}
			closed := map[string]bool{}
			for _, r := range terms {
				closed[r.str("tool_use_id")] = true
				if r.str("reason") == "lock_timeout" {
					gaveUp[r.str("tool_use_id")] = true
				}
			}
			if len(decls) < n {
				t.Logf("%d of %d declarations gave up on the append lock (lock_timeout), which the design allows on a slow disk", n-len(decls), n)
			}

			got := map[string]bool{}
			for _, d := range decls {
				id := d.str("tool_use_id")
				if got[id] {
					t.Errorf("declaration %s landed twice", id)
				}
				got[id] = true
				if pad > 0 && len(d.str("transcript_path")) != pad+len("/tmp/.jsonl") {
					t.Errorf("padding did not reach the record: transcript_path is %d bytes", len(d.str("transcript_path")))
				}
			}
			for id := range want {
				if !closed[id] {
					t.Errorf("handler %s has no terminal", id)
				}
				if !got[id] && !gaveUp[id] {
					t.Errorf("declaration %s did not land, and its terminal does not say lock_timeout", id)
				}
			}
			for id := range got {
				if !want[id] {
					t.Errorf("declaration %s was never sent", id)
				}
			}

			// The ordered stream is what the lock orders: every record in it
			// carries a seq, and together they are 1..k. A spilled record is
			// written without the lock and has none, by design.
			var seqs []int
			for _, r := range e.read(testSession, "records.ndjson") {
				s, ok := r.fields["seq"].(float64)
				if !ok {
					t.Errorf("record without a numeric seq: %s", r.raw)
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
