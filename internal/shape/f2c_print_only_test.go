package shape

import "testing"

// TestF2e_AStageThatOnlyPrintsDoesNotWrite (card F2c, (b')): call 9325 of
// eng-4's session 02bc13cc, `cat STATUS; echo ---; cat STATUS; date`, read two
// files and printed a separator and the date, and its record says it may
// write, since echo and date are outside the read class and mayWrite counts
// every later stage outside it. A stage whose program only prints (echo,
// date) writes no file of itself; what makes it write is a redirect, a
// substitution or a writer word, which stageMayWrite reads over the whole
// line still. Rows: 9325's shape, read class and may_write false; a read
// followed by a printing stage in each separator. Controls, each may_write:
// a printing stage that writes by a redirect, a pipe to a writer, a
// substitution or a pipe into a shell, alone or after a read, and a loop
// whose body writes. Residuals, as they read: a line that starts with echo
// is execute (its class is its first program's), and `date -s` sets the
// clock, not a file. Break: leave echo or date out of the stages that print,
// or read a printing stage as one that cannot write whatever its words.
func TestF2e_AStageThatOnlyPrintsDoesNotWrite(t *testing.T) {
	t.Run("call 9325", func(t *testing.T) {
		sh := verbOf(t, "cat /wt/rv-se4-gi1w/STATUS.txt; echo ---; cat /wt/rv-se4-gi1w5-red/STATUS.txt; date")
		if sh.VerbClass != VerbRead || sh.MayWrite {
			t.Fatalf("class %s, may_write %t; want read, false", sh.VerbClass, sh.MayWrite)
		}
	})
	for _, tc := range []struct {
		cmd   string
		write bool
	}{
		{"ls && echo done", false},
		{"cat a; date", false},
		{"cat a | head; echo", false},
		{"cat a && echo \"x > y\"", false},
		{"cat a\necho ---\ndate", false},
		{"cat a; date -s 1200", false},
		{"echo x > f", true},
		{"date >> log", true},
		{"printf x | tee f", true},
		{"for f in a b; do echo $f > $f.txt; done", true},
		{"cat a; echo x > f", true},
		{"cat a; date >> log", true},
		{"cat a; echo $(rm x)", true},
		{"cat a; echo x | sh", true},
		{"cat a; echo x | tee f", true},
	} {
		if sh := verbOf(t, tc.cmd); sh.MayWrite != tc.write {
			t.Errorf("%q: may_write %t, want %t", tc.cmd, sh.MayWrite, tc.write)
		}
	}
	if sh := verbOf(t, "echo hi; cat a"); sh.VerbClass != VerbExecute {
		t.Errorf("echo hi; cat a: class %s, want execute (a line that starts with echo still acts)", sh.VerbClass)
	}
}
