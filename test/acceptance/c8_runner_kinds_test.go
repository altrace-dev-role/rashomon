package acceptance

import (
	"encoding/json"
	"testing"
)

// C8: a lint, typecheck or scan run is a gate of its own kind, end to end.
// Real hooks through the built binary: the declaration and the execution both
// carry the runner's kind, the call's output is classified without being read
// (a check runner's output has no table of its own yet), and the report's
// autosteer block names the gate with its kind where, before the lists, a lint
// run was a command no runner list named and its kind was custom.
func TestC8_ALintRunIsAGateOfKindLint(t *testing.T) {
	for _, tc := range []struct{ cmd, kind, name string }{
		{"ruff check .", "lint", "ruff check"},
		{"mypy src", "typecheck", "mypy"},
		{"semgrep --config auto .", "scan", "semgrep"},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			s := newTBSession(t)
			s.shell(tc.cmd, true, "")

			decls := s.e.declarations(testSession)
			if got, _ := nested(decls[0], "shape.runner_kind"); got != tc.kind {
				t.Errorf("declaration runner_kind = %v, want %s", got, tc.kind)
			}
			execs := s.e.executions(testSession)
			if got := execs[0].fields["runner_kind"]; got != tc.kind {
				t.Errorf("execution runner_kind = %v, want %s", got, tc.kind)
			}
			if got := execs[0].fields["runner_name"]; got != tc.name {
				t.Errorf("execution runner_name = %v, want %q", got, tc.name)
			}
			if got, _ := nested(execs[0], "gate_output.kind"); got != "no_summary" {
				t.Errorf("gate_output.kind = %v, want no_summary: a check runner's output is not read", got)
			}

			if _, ok := s.lineSaying(tbHonest); !ok {
				t.Log("no Stop line printed; the checkpoint is still written")
			}
			js := s.e.run("", nil, "report", "--json", "--session", testSession).stdout
			var rep struct {
				Sessions []struct {
					Autosteer struct {
						Gates []struct{ Name, Kind, State string }
					}
				}
			}
			if err := json.Unmarshal([]byte(js), &rep); err != nil || len(rep.Sessions) != 1 {
				t.Fatalf("report --json: %v\n%s", err, js)
			}
			gates := rep.Sessions[0].Autosteer.Gates
			if len(gates) != 1 || gates[0].Kind != tc.kind {
				t.Errorf("autosteer gates = %+v, want one gate of kind %s", gates, tc.kind)
			}
		})
	}
}
