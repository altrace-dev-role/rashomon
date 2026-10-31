package shape

import (
	"slices"
	"strings"
	"testing"
)

// cmdRow is one line and the commands Commands should read from it: each
// command's words joined by "|", a word that expands written with a leading
// "$!", and a command fed by a here-document or here-string ending in "<<".
type cmdRow struct {
	line string
	want []string
	ok   bool
}

func checkCommands(t *testing.T, rows []cmdRow) {
	t.Helper()
	for _, r := range rows {
		cmds, ok := Commands(r.line)
		var got []string
		for _, c := range cmds {
			if len(c.Words) != len(c.Expands) {
				t.Errorf("%q: %d words and %d expansion marks", r.line, len(c.Words), len(c.Expands))
			}
			ws := make([]string, len(c.Words))
			for i, w := range c.Words {
				if c.Expands[i] {
					w = "$!" + w
				}
				ws[i] = w
			}
			s := strings.Join(ws, "|")
			if c.Heredoc {
				s += "<<"
			}
			got = append(got, s)
		}
		if ok != r.ok || !slices.Equal(got, r.want) {
			t.Errorf("Commands(%q) = %q, %t; want %q, %t", r.line, got, ok, r.want, r.ok)
		}
	}
}

// TestSR1s_CommandsAtCommandPosition: every simple command of a line, in line
// order, from its program on: stages split at ; && || | & and a newline;
// assignments before the program, redirections with their words and fd
// prefixes left out; the shell's reserved words before a program passed over,
// and a stage of reserved words alone gives no command; quotes removed, so a
// quoted separator stays inside its word.
func TestSR1s_CommandsAtCommandPosition(t *testing.T) {
	checkCommands(t, []cmdRow{
		{"git push --force origin main", []string{"git|push|--force|origin|main"}, true},
		{"cd repo && git push -f", []string{"cd|repo", "git|push|-f"}, true},
		{"FOO=1 git push --force > out.log 2>&1", []string{"git|push|--force"}, true},
		{"make test 2>&1 | tail -5; git status", []string{"make|test", "tail|-5", "git|status"}, true},
		{"git push --force &", []string{"git|push|--force"}, true},
		{"git fetch\ngit push --force", []string{"git|fetch", "git|push|--force"}, true},
		{`git commit -m "a; b && c"`, []string{"git|commit|-m|a; b && c"}, true},
		{"echo 'git push --force'", []string{"echo|git push --force"}, true},
		{"( git push --force )", []string{"git|push|--force"}, true},
		{"if git push --force; then echo ok; fi", []string{"git|push|--force", "echo|ok"}, true},
		{"! git push --force", []string{"git|push|--force"}, true},
		{"for b in a c; do git push --force origin $b; done", []string{"for|b|in|a|c", "git|push|--force|origin|$!$b"}, true},
		{"/usr/bin/git push --force", []string{"/usr/bin/git|push|--force"}, true},
		{"", nil, true},
	})
}

// TestSR1t_CommandsMarksWhatTheShellExpands: a word the shell expands is
// marked, never read as the word the shell will run: a parameter, a command
// substitution, a backtick, an unquoted glob. A quoted glob is literal. A
// here-document or here-string feeding a command is noted on it, and the
// body is not read as commands.
func TestSR1t_CommandsMarksWhatTheShellExpands(t *testing.T) {
	checkCommands(t, []cmdRow{
		{"git push $REMOTE --force", []string{"git|push|$!$REMOTE|--force"}, true},
		{`git push "$REMOTE" --force`, []string{"git|push|$!$REMOTE|--force"}, true},
		{"git push --force-with-lease=*", []string{"git|push|$!--force-with-lease=*"}, true},
		{"git push '--force*'", []string{"git|push|--force*"}, true},
		{"psql <<'SQL'\nTRUNCATE t;\nSQL", []string{"psql<<"}, true},
		{"bash <<<'git push --force'", []string{"bash<<"}, true},
		{"cat <<EOF\ngit push --force\nEOF\ngit status", []string{"cat<<", "git|status"}, true},
	})
}

// TestSR1u_CommandsRefusesWhatItCannotRead: ok is false, and nothing past the
// refusal is listed, where the line cannot be read as the shell reads it: a
// control byte, a command substitution or an expansion at command position,
// a function definition.
func TestSR1u_CommandsRefusesWhatItCannotRead(t *testing.T) {
	for _, line := range []string{
		"git push --force\x01",
		"$(echo git) push --force",
		"`echo git` push --force",
		"$GIT push --force",
		"f() { git push --force; }; f",
	} {
		if cmds, ok := Commands(line); ok {
			t.Errorf("Commands(%q) = %+v, true; want ok false", line, cmds)
		}
	}
}

// TestSR1v_CommandLineIsAShellToolsLineUnderTheCap: CommandLine is the line
// Derive reads, decoded, for a shell tool alone, and nothing for another
// tool's command field, a command that is no string, or a command whose
// encoded form is over the cap, which is refused before it is decoded.
func TestSR1v_CommandLineIsAShellToolsLineUnderTheCap(t *testing.T) {
	for _, c := range []struct {
		name, tool, input string
		max               int
		want              string
		ok                bool
	}{
		{"a Bash line", "Bash", `{"command": "git push --force origin main"}`, 64, "git push --force origin main", true},
		{"decoded", "Bash", `{"command": "echo \"a\tb\" é"}`, 64, "echo \"a\tb\" é", true},
		{"at the cap", "Bash", `{"command": "` + strings.Repeat("x", 16) + `"}`, 16, strings.Repeat("x", 16), true},
		{"over the cap", "Bash", `{"command": "` + strings.Repeat("x", 17) + `"}`, 16, "", false},
		{"escapes count as encoded", "Bash", `{"command": "` + strings.Repeat(`\"`, 9) + `"}`, 16, "", false},
		{"empty", "Bash", `{"command": ""}`, 16, "", true},
		{"null", "Bash", `{"command": null}`, 16, "", false},
		{"not a string", "Bash", `{"command": ["git", "push"]}`, 64, "", false},
		{"absent", "Bash", `{"description": "git push"}`, 64, "", false},
		{"a file tool's field", "Edit", `{"command": "git push", "file_path": "/p/a.go"}`, 64, "", false},
		{"an MCP tool's field", "mcp__srv__run", `{"command": "git push"}`, 64, "", false},
		{"no input", "Bash", ``, 64, "", false},
	} {
		got, ok := CommandLine(c.tool, []byte(c.input), c.max)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: CommandLine(%s, %s, %d) = %q, %v; want %q, %v", c.name, c.tool, c.input, c.max, got, ok, c.want, c.ok)
		}
	}
}
