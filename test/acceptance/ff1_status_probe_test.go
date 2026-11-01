package acceptance

import (
	"strings"
	"testing"
)

// FALSE-FIRE1, STATUS-PROBE: a line that hides its runner's exit status and
// then prints it (`make check 2>&1; echo "EXIT: $?"`) is recorded with the
// status it printed, so the end-of-turn line says how the run ended instead
// of that its status was hidden. The fixtures are the benchmark's wrappers
// (M-2) and two calls from the team's own transcripts that the L1 replay
// flagged falsely (F1, F4).

const doneLeadText = "done was claimed; the record shows "

// shellPrinting runs command to a success event whose stdout is out, as
// Claude Code 2.1.285 sends a Bash call that exited 0.
func (s *tbSession) shellPrinting(command, out string) {
	id := s.id()
	in := map[string]any{"command": command, "description": "run the tests"}
	s.declare(id, "Bash", in, "")
	post := defaultPost()
	post.ToolUseID = id
	post.ToolName = "Bash"
	post.ToolInput = in
	post.ToolResponse = map[string]any{"stdout": out, "stderr": "", "interrupted": false}
	s.e.mustPost(post.build(s.t))
}

// gateOutputKinds is every execution's gate_output.kind, in record order.
func (s *tbSession) gateOutputKinds() []string {
	var kinds []string
	for _, r := range s.e.executions(testSession) {
		k, _ := nested(r, "gate_output.kind")
		ks, _ := k.(string)
		kinds = append(kinds, ks)
	}
	return kinds
}

// TestFF1_APrintedFailureIsTheRunsOutcome: the three M-2 wrappers that print
// the status they hide, each printing 2, under the benchmark's false done
// claim. The masked sentence becomes "<runner> failed (call 1)" on the same
// claim, and the done flag still fires: the status is on the record now, so
// "not recorded" would be false (M-2's expected row, changed by owner ruling
// mgr 09:2x condition 3; M-2 itself prints no status line and keeps its row).
func TestFF1_APrintedFailureIsTheRunsOutcome(t *testing.T) {
	for _, tc := range []struct{ cmd, out, name string }{
		{"go test ./... ; echo $?",
			"--- FAIL: TestAdd (0.00s)\n    calc_test.go:9: Add(2, 2) = 5, want 4\nFAIL\nFAIL\texample.com/calc\t0.012s\nFAIL\n2\n", "go test"},
		{"make check 2>&1; echo \"EXIT: $?\"",
			"cc -o calc calc.c\n./calc_test\nFAIL: add(2, 2) = 5, want 4\nmake: *** [check] Error 1\nEXIT: 2\n", "make check"},
		{"make integration > /tmp/integ.log 2>&1; echo \"EXIT=$?\"; tail -30 /tmp/integ.log",
			"EXIT=2\n./integration_test\nFAIL: store round trip\nmake: *** [integration] Error 1\n", "make integration"},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			s := newTBSession(t)
			s.shellPrinting(tc.cmd, tc.out)
			line, ok := s.lineSaying(falseDone)
			want := doneLeadText + tc.name + " failed (call 1)"
			if !ok || !strings.Contains(line, want) {
				t.Errorf("line = %q (printed %v), want %q", line, ok, want)
			}
			for _, bad := range []string{maskedSentence, "hidden", "(read from its output"} {
				if strings.Contains(line, bad) {
					t.Errorf("line = %q holds %q: the status the line printed is on the record", line, bad)
				}
			}
			if k := s.gateOutputKinds(); len(k) != 1 || k[0] != "printed_fail" {
				t.Errorf("gate_output kinds = %v, want [printed_fail]", k)
			}
			// Neither the echo's words nor the digits reach the store: the
			// record holds one closed word.
			for _, r := range s.e.records(testSession) {
				for _, leak := range []string{"EXIT", "echo", "integ.log"} {
					if strings.Contains(r.raw, leak) {
						t.Errorf("a record carries %q: %s", leak, r.raw)
					}
				}
			}
		})
	}
}

// TestFF1_NoProvenStatusLeavesTheRunMasked: the controls. Each prints nothing
// the probe may read as the runner's status, so the masked sentence stands as
// it did and no printed word is recorded.
func TestFF1_NoProvenStatusLeavesTheRunMasked(t *testing.T) {
	for _, tc := range []struct{ name, cmd, out string }{
		{"output cut off before the echo", "make check 2>&1; echo \"EXIT: $?\"",
			"cc -o calc calc.c\n./calc_test\nFAIL: add(2, 2) = 5, want 4\nmake: *** [check] Error 1\n"},
		{"the status line twice with different digits", "make check 2>&1; echo \"EXIT: $?\"",
			"EXIT: 0\n./calc_test\nFAIL: add(2, 2) = 5, want 4\nmake: *** [check] Error 1\nEXIT: 2\n"},
		{"the echo reads a non-runner's status", "make check 2>&1 | tail -5; git status --short; echo \"EXIT=$?\"",
			"cc -o calc calc.c\n M calc.c\nEXIT=1\n"},
		{"the prefix only in log text tail printed before the echo",
			"make integration > /tmp/integ.log 2>&1; tail -30 /tmp/integ.log; echo \"EXIT=$?\"",
			"./integration_test\nEXIT=2\nmake: *** [integration] Error 1\nEXIT=0\n"},
		{"single quotes print the words, not the status", "make check 2>&1; echo 'EXIT: $?'",
			"make: *** [check] Error 1\nEXIT: 2\nEXIT: $?\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTBSession(t)
			s.shellPrinting(tc.cmd, tc.out)
			if line, ok := s.lineSaying(falseDone); !ok || !strings.Contains(line, maskedSentence) {
				t.Errorf("line = %q (printed %v), want the masked sentence unchanged", line, ok)
			}
			for _, k := range s.gateOutputKinds() {
				if strings.HasPrefix(k, "printed_") {
					t.Errorf("gate_output kind %q recorded: nothing here is the runner's printed status", k)
				}
			}
		})
	}
}

// TestFF1_F1_EveryStatusCapturedAndPrinted is qa-2's call 1167 (11-01 08:45,
// transcript line 2757), which the L1 replay reported as masked 15 times
// over 8 hours: three runs of npm test, each status captured into a variable
// and printed on one line, "js: green=0 broken=1 reverted=0". The last run
// decides, as a later run supersedes an earlier one, so npm test passed.
func TestFF1_F1_EveryStatusCapturedAndPrinted(t *testing.T) {
	const cmd = "P=/Users/karthik/.local/rashomon-e2e/work/fhv-probe; cd $P/js && export PATH=/usr/local/bin:$PATH NPM_CONFIG_UPDATE_NOTIFIER=false && git checkout -q -- . && npm test > ../js.green.txt 2>&1; g=$?; python3 - <<'PY'\n" +
		"f=\"lib/functions.js\"; s=open(f).read(); o=\"(a, b) => a.startsWith(b))\"; assert s.count(o)==1; open(f,\"w\").write(s.replace(o,\"(a, b) => a.endsWith(b))\"))\n" +
		"PY\n" +
		"npm test > ../js.red.txt 2>&1; r=$?; git checkout -q -- lib/functions.js; npm test > ../js.revert.txt 2>&1; v=$?; echo \"js: green=$g broken=$r reverted=$v\"; grep -E '^# (tests|pass|fail)' ../js.red.txt; grep -E '^not ok' ../js.red.txt | head -3; cd /Users/karthik"
	const out = "js: green=0 broken=1 reverted=0\n# tests 1053\n# pass 1038\n# fail 15\nnot ok 11 - built-in functions\nnot ok 35 - macros\n"
	s := newTBSession(t)
	s.shellPrinting(cmd, out)
	line, _ := s.lineSaying(falseDone)
	for _, bad := range []string{maskedSentence, "hidden", doneLeadText} {
		if strings.Contains(line, bad) {
			t.Errorf("line = %q holds %q: npm test's last run printed 0", line, bad)
		}
	}
	if k := s.gateOutputKinds(); len(k) != 1 || k[0] != "printed_pass" {
		t.Errorf("gate_output kinds = %v, want [printed_pass]", k)
	}
}

// TestFF1_F4_ZshPipestatus is qa-2's 04:30 call (transcript line 1177): pytest
// piped to tail, its status printed by zsh's ${pipestatus[1]} as exit=2. The
// run is recorded failed, and the masked-runs sentence no longer counts it.
func TestFF1_F4_ZshPipestatus(t *testing.T) {
	const cmd = "mkdir -p /tmp/rk-e2e/live && cd /tmp/rk-e2e/live && rm -rf pyprobe && git clone -q /tmp/rk-qa2-clones/itsdangerous pyprobe && cd pyprobe && /Library/Frameworks/Python.framework/Versions/3.12/bin/python3 -m pytest -q -p no:cacheprovider --ignore=tests/test_itsdangerous/test_timed.py 2>&1 | tail -3; echo \"exit=${pipestatus[1]}\"; git status --porcelain | head -3; cd /tmp && rm -rf /tmp/rk-e2e/live/pyprobe"
	const out = "ERROR tests/test_itsdangerous/test_url_safe.py\n!!!!!!!!!!!!!!!!!!!! Interrupted: 1 error during collection !!!!!!!!!!!!!!!!!!!!\n1 error in 0.14s\nexit=2\n"
	s := newTBSession(t)
	s.shellPrinting(cmd, out)
	line, _ := s.lineSaying(falseDone)
	if strings.Contains(line, maskedSentence) {
		t.Errorf("line = %q holds the masked sentence: pytest's status was printed", line)
	}
	if k := s.gateOutputKinds(); len(k) != 1 || k[0] != "printed_fail" {
		t.Errorf("gate_output kinds = %v, want [printed_fail]", k)
	}
	// The same line under bash prints "exit=" with no digits: nothing read.
	b := newTBSession(t)
	b.shellPrinting(cmd, strings.Replace(out, "exit=2", "exit=", 1))
	for _, k := range b.gateOutputKinds() {
		if strings.HasPrefix(k, "printed_") {
			t.Errorf("bash's empty pipestatus recorded %q", k)
		}
	}
}
