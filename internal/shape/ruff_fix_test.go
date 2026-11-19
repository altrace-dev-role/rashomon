package shape

import "testing"

// TestRH11_RuffFixFlagsAreWrites: ruff check rewrites the files it lints
// under --fix and under --fix-only, wherever the flag stands, so a stage
// carrying either may write and names its file operands as written paths,
// never ruff's subcommand: the first operand, check or format, names no file,
// and a later operand of either name is a path and is named, as is one after
// -- spelled like a flag. --diff beside either prints the fixes and writes
// none back (ruff 0.3.7's help: "Avoid writing any fixed files back ...
// Implies --fix-only"), so it reads, as ruff format --check does;
// --unsafe-fixes alone widens the fixes --fix would apply and applies none,
// and a plain ruff check only reports. A ruff config with fix = true writes
// on a plain ruff check, which the command words cannot show: a named limit
// with a debt row, not a row here. Break: drop --fix or --fix-only from
// ruff's words in inPlaceEditors, name the subcommand as a written path, or
// skip a later operand that spells check or format.
func TestRH11_RuffFixFlagsAreWrites(t *testing.T) {
	for _, r := range []struct {
		cmd   string
		write bool
	}{
		{"ruff check --fix .", true},
		{"ruff check --fix-only .", true},
		{"ruff check . --fix", true},
		{"ruff check .", false},
		{"ruff format --check .", false},
		{"ruff check --fix --diff .", false},
		{"ruff check --fix-only --diff .", false},
		{"ruff check --unsafe-fixes .", false},
	} {
		if sh := verbOf(t, r.cmd); sh.MayWrite != r.write {
			t.Errorf("%q: may_write %t, want %t", r.cmd, sh.MayWrite, r.write)
		}
	}
	checkWT(t, []wtRow{
		{"ruff check --fix a.py", []string{"a.py"}, true},
		{"ruff check --fix-only a.py b.py", []string{"a.py", "b.py"}, true},
		{"ruff format a.py", []string{"a.py"}, true},
		// A path that spells the subcommand, behind the first operand.
		{"ruff check --fix check", []string{"check"}, true},
		{"ruff format check", []string{"check"}, true},
		{"ruff check --fix format", []string{"format"}, true},
		{"ruff format format", []string{"format"}, true},
		{"ruff check --fix -- --fix", []string{"--fix"}, true},
	})
}
