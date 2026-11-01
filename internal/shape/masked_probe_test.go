package shape

import (
	"encoding/json"
	"testing"
)

// TestFF1_PrintedStatus pins what PrintedStatus reads and, row by row, each
// rule that keeps it from reading a status that is not the runner's: a
// mutant that drops one turns its row's want around.
func TestFF1_PrintedStatus(t *testing.T) {
	for _, tc := range []struct {
		name, cmd, out string
		code           int
		ok             bool
	}{
		{"$? in a quoted echo", `make check 2>&1; echo "EXIT: $?"`, "make: *** Error 1\nEXIT: 2\n", 2, true},
		{"a pass", `make check 2>&1; echo "EXIT: $?"`, "ok\nEXIT: 0\n", 0, true},
		{"$? alone on its line", `go test ./... ; echo $?`, "FAIL\texample.com/calc\t0.01s\nFAIL\n1\n", 1, true},
		{"the last line has no newline", `make check; echo "EXIT=$?"`, "EXIT=2", 2, true},
		{"the same status twice", `make check; echo "EXIT=$?"`, "EXIT=2\nEXIT=2\n", 2, true},
		{"bash PIPESTATUS of the runner's stage", `go test ./... 2>&1 | tail -3; echo "rc=${PIPESTATUS[0]}"`, "FAIL\nrc=1\n", 1, true},
		{"zsh pipestatus of the runner's stage", `go test ./... 2>&1 | tail -3; echo "rc=${pipestatus[1]}"`, "FAIL\nrc=1\n", 1, true},
		{"a variable set to $? right after", `make check; rc=$?; echo "done rc=$rc"`, "done rc=2\n", 2, true},
		{"after && the runner is last", `cd a && make check; echo "EXIT=$?"`, "EXIT=0\n", 0, true},
		{"the last of three runs decides", `make t > a; g=$?; make t > b; r=$?; make t > c; v=$?; echo "g=$g r=$r v=$v"`, "g=0 r=1 v=0\n", 0, true},

		{"no status line", `make check 2>&1; echo "EXIT: $?"`, "make: *** Error 1\n", 0, false},
		{"two different statuses", `make check 2>&1; echo "EXIT: $?"`, "EXIT: 0\nEXIT: 2\n", 0, false},
		{"the line is the template only in part", `make check; echo "EXIT=$?"`, "EXIT=2 from a sub-make\n", 0, false},
		{"the template inside a longer line", `make check; echo "EXIT=$?"`, "log: EXIT=2\n", 0, false},
		{"four digits", `make check; echo "EXIT=$?"`, "EXIT=1000\n", 0, false},
		{"leading zeros", `make check; echo "EXIT=$?"`, "EXIT=0002\n", 0, false},
		{"two hidden runners", `make check; echo "a=$?"; go vet ./...; echo "b=$?"`, "a=0\nb=1\n", 0, false},
		{"past 255", `make check; echo "EXIT=$?"`, "EXIT=256\n", 0, false},
		{"$? of tail, the last stage", `make check | tail -3; echo "EXIT=$?"`, "EXIT=0\n", 0, false},
		{"PIPESTATUS of another stage", `go test ./... 2>&1 | tail -3; echo "rc=${PIPESTATUS[1]}"`, "rc=0\n", 0, false},
		{"zsh index 0 is no stage", `go test ./... 2>&1 | tail -3; echo "rc=${pipestatus[0]}"`, "rc=0\n", 0, false},
		{"a command between run and echo", `make check; git status; echo "EXIT=$?"`, "EXIT=1\n", 0, false},
		{"single quotes print $? as it is", `make check; echo 'EXIT=$?'`, "EXIT=2\nEXIT=$?\n", 0, false},
		{"an escaped dollar prints as it is", `make check; echo "EXIT=\$?"`, "EXIT=2\nEXIT=$?\n", 0, false},
		{"an echo flag", `make check; echo -n "EXIT=$?"`, "EXIT=2", 0, false},
		{"a redirected echo", `make check; echo "EXIT=$?" > rc.txt`, "EXIT=2\n", 0, false},
		{"another expansion in the echo", `make check; echo "$HOME EXIT=$?"`, "/h EXIT=2\n", 0, false},
		{"a glob character in the echo", `make check; echo EXIT*=$?`, "EXIT*=2\n", 0, false},
		{"two statuses with nothing between", `make check; rc=$?; echo "x=$rc$?"`, "x=20\n", 0, false},
		{"the variable assigned twice", `make check; rc=$?; rc=0; echo "rc=$rc"`, "rc=2\n", 0, false},
		{"the variable set after a non-runner", `make check; git status; rc=$?; echo "rc=$rc"`, "rc=1\n", 0, false},
		{"the run after ||", `cd a || make check; echo "EXIT=$?"`, "EXIT=0\n", 0, false},
		{"a negated run", `! make check; echo "EXIT=$?"`, "EXIT=0\n", 0, false},
		{"a run in a subshell", `(make check); echo "EXIT=$?"`, "EXIT=2\n", 0, false},
		{"a run in the background", `make check & echo "EXIT=$?"`, "EXIT=0\n", 0, false},
		{"a hiding flag", `make -i check; echo "EXIT=$?"`, "EXIT=0\n", 0, false},
		{"the last run unprinted", `make t > a; g=$?; make t > b; echo "g=$g"`, "g=2\n", 0, false},
		{"a status the line returns", `make check && echo ok`, "ok\n", 0, false},
		{"no runner", `git status; echo "EXIT=$?"`, "EXIT=0\n", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, _ := json.Marshal(map[string]string{"command": tc.cmd})
			code, ok := PrintedStatus("Bash", in, []byte(tc.out))
			if code != tc.code || ok != tc.ok {
				t.Errorf("PrintedStatus(%q) on %q = %d, %v, want %d, %v", tc.cmd, tc.out, code, ok, tc.code, tc.ok)
			}
		})
	}
	// Not a shell tool: its "command" is not a line.
	if _, ok := PrintedStatus("mcp__x__run", json.RawMessage(`{"command":"make check; echo \"EXIT=$?\""}`), []byte("EXIT=2\n")); ok {
		t.Error("a non-shell tool's command field was read as a line")
	}
}
