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
// declarations and all 64 terminals must land, with distinct ids and a
// contiguous seq -- the invariant the lock exists to protect, and the one that
// breaks when the lock is removed.
func TestH16_ConcurrentHandlersAllLand(t *testing.T) {
	for _, pad := range []int{0, 5000} {
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
			if len(decls) != n || len(terms) != n {
				t.Fatalf("got %d declarations and %d terminals, want %d each", len(decls), len(terms), n)
			}

			got := map[string]bool{}
			for _, d := range decls {
				got[d.str("tool_use_id")] = true
				if pad > 0 && len(d.str("transcript_path")) != pad+len("/tmp/.jsonl") {
					t.Errorf("padding did not reach the record: transcript_path is %d bytes", len(d.str("transcript_path")))
				}
			}
			for id := range want {
				if !got[id] {
					t.Errorf("declaration %s did not land", id)
				}
			}
			for id := range got {
				if !want[id] {
					t.Errorf("declaration %s was never sent", id)
				}
			}

			var seqs []int
			for _, r := range append(decls, terms...) {
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
