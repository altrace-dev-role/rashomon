package shape

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The inputs are generated in code at fixed sizes: a case named 4 KB is 4,096
// bytes of command line and a case named 50k is 51,200 bytes of new_string,
// not a fixture file that drifted.

// benchKey stands in for a store's per-install HMAC key.
var benchKey = bytes.Repeat([]byte{0x5a}, 32)

const (
	// benchShortCommand is bash_short: a common line with one operator, so
	// the program search and the masked-status walk both have work to do.
	benchShortCommand = "git status --short && git diff --stat"

	// benchLongBytes is the size of bash_long_4k's command line.
	benchLongBytes = 4096

	// benchLongTokens is what tokenizeShape counts in benchLongCommand, and so
	// also its argc: pinned, so a lexer that stops early fails the benchmarks.
	// 1,068 was checked against an independent count of the same line by the
	// same splitting rules, not only read off this lexer.
	benchLongTokens = 1068

	// benchEditBytes is the size of edit_50k's new_string.
	benchEditBytes = 50 << 10
)

// benchLongCommand is the shape of the v1.1.0 baseline's 4 KB line -- a cd, a
// here-document writing a script, a pipeline running it -- padded by a comment
// line in the script to exactly benchLongBytes.
func benchLongCommand() string {
	const (
		head = "cd /Users/dev/src/project/web && cat > /tmp/analyze.py <<'PYEOF'\n" +
			"import json, sys\ndata = json.load(sys.stdin)\n"
		tail = "PYEOF\ncurl -sS -H 'Accept: application/vnd.github+json' " +
			"'https://api.github.com/repos/example/project/issues?state=all&per_page=100'" +
			" | python3 /tmp/analyze.py | sort -k3 -n | tail -20"
	)
	var b strings.Builder
	b.WriteString(head)
	for i := 1; ; i++ {
		n := strconv.Itoa(i)
		pair := "rows" + n + " = [r for r in data.get('items', []) if r.get('state') == 'open' and r.get('score', 0) > " + n + "]\n" +
			"print('bucket " + n + ":', len(rows" + n + "), sum(r.get('score', 0) for r in rows" + n + "))\n"
		if b.Len()+len(pair)+len("#\n")+len(tail) > benchLongBytes {
			break
		}
		b.WriteString(pair)
	}
	b.WriteString("#" + strings.Repeat("x", benchLongBytes-b.Len()-len("#\n")-len(tail)) + "\n")
	b.WriteString(tail)
	return b.String()
}

// benchFiller is n bytes of Go-looking text holding no byte JSON escapes, so
// it encodes to n bytes plus its quotes.
func benchFiller(n int) string {
	const line = "    for i := range batch { if err := handle(batch[i]); err != nil { return err } } "
	return strings.Repeat(line, n/len(line)+1)[:n]
}

// benchJSON encodes v as Claude Code writes a payload: JavaScript's
// JSON.stringify leaves & < > alone, where json.Marshal would escape them.
func benchJSON(tb testing.TB, v any) json.RawMessage {
	tb.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		tb.Fatal(err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// benchBashInput is a Bash tool_input running cmd.
func benchBashInput(tb testing.TB, cmd string) json.RawMessage {
	tb.Helper()
	return benchJSON(tb, struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}{cmd, "Run the command"})
}

// benchEditInput is edit_50k's tool_input: new_string is benchEditBytes.
func benchEditInput(tb testing.TB) json.RawMessage {
	tb.Helper()
	return benchJSON(tb, struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}{"/Users/dev/src/project/internal/batch/handle.go", "// handle processes one batch of records\n", benchFiller(benchEditBytes), false})
}

// deriveCase is one BenchmarkDerive case and what its shape must say.
// program is empty and argc is -1 for a tool that is not a shell.
type deriveCase struct {
	name    string
	tool    string
	program string
	verb    string
	argc    int
}

// benchDeriveInput is the tool_input of BenchmarkDerive's case name, and the
// keyed digest of the whole of it, which the shape must carry.
func benchDeriveInput(tb testing.TB, name string) (json.RawMessage, string) {
	tb.Helper()
	switch name {
	case "bash_short":
		return benchBashInput(tb, benchShortCommand), digest(benchKey, "Bash", benchShortCommand)
	case "bash_long_4k":
		long := benchLongCommand()
		if len(long) != benchLongBytes {
			tb.Fatalf("bash_long_4k command is %d bytes, want %d", len(long), benchLongBytes)
		}
		return benchBashInput(tb, long), digest(benchKey, "Bash", long)
	case "edit_50k":
		edit := benchEditInput(tb)
		return edit, canonicalDigest(benchKey, "Edit", edit)
	}
	tb.Fatalf("BenchmarkDerive has no case %q", name)
	return nil, ""
}

// checkDerived is the work-done check: the shape names the program and counts
// the line, and its digest is want, the keyed digest of the whole input.
func checkDerived(b *testing.B, c deriveCase, want string, got Shape) {
	b.Helper()
	if got.Digest != want {
		b.Fatalf("%s: digest %q, want the keyed digest of the whole input %q", c.name, got.Digest, want)
	}
	if got.VerbClass != c.verb {
		b.Fatalf("%s: verb class %q, want %q", c.name, got.VerbClass, c.verb)
	}
	if c.argc < 0 {
		if got.Program != nil || got.Argc != nil {
			b.Fatalf("%s: a tool that is not a shell got program %v, argc %v", c.name, show(got.Program), show(got.Argc))
		}
		return
	}
	if got.Program == nil || *got.Program != c.program {
		b.Fatalf("%s: program %v, want %q", c.name, show(got.Program), c.program)
	}
	if got.Argc == nil || *got.Argc != c.argc {
		b.Fatalf("%s: argc %v, want %d", c.name, show(got.Argc), c.argc)
	}
	if got.StatusMasked == nil || *got.StatusMasked != MaskedNone {
		b.Fatalf("%s: status_masked %v, want %q: the masked-status walk did not read the line to its end", c.name, show(got.StatusMasked), MaskedNone)
	}
}

// BenchmarkDerive is the shape of one call: the work the hook does on every
// PreToolUse and the post path on every PostToolUse that carries tool_input.
// The case names are literals, so the budget gate reads them from the source.
func BenchmarkDerive(b *testing.B) {
	for _, c := range []deriveCase{
		{name: "bash_short", tool: "Bash", program: "git", verb: VerbVCS, argc: 7},
		{name: "bash_long_4k", tool: "Bash", program: "cat", verb: VerbRead, argc: benchLongTokens},
		{name: "edit_50k", tool: "Edit", verb: VerbWrite, argc: -1},
	} {
		b.Run(c.name, func(b *testing.B) {
			input, want := benchDeriveInput(b, c.name)
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			var got Shape
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got = Derive(c.tool, input, benchKey)
			}
			b.StopTimer()
			checkDerived(b, c, want, got)
		})
	}
}

// BenchmarkLexTokens is the lexer on the 4 KB line, per token: B/token is
// what each token allocates, the figure the token bound's arithmetic (RES-9)
// divides into a memory budget. It is one reading of the line, the one argc
// counts; Derive reads it more than once, so the shape's own cost per token is
// BenchmarkDerive/bash_long_4k's B/op over the same 1,068 tokens. SetBytes is
// the line, so MB/s is the lexer's throughput.
func BenchmarkLexTokens(b *testing.B) {
	cmd := benchLongCommand()
	b.ReportAllocs()
	b.SetBytes(int64(len(cmd)))
	var (
		toks   []token
		err    error
		m0, m1 runtime.MemStats
	)
	runtime.ReadMemStats(&m0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toks, err = tokenizeShape(cmd)
	}
	b.StopTimer()
	runtime.ReadMemStats(&m1)
	if err != nil || len(toks) != benchLongTokens {
		b.Fatalf("lexed %d tokens (err %v), want %d: the lexer did not read the line to its end", len(toks), err, benchLongTokens)
	}
	b.ReportMetric(float64(len(toks)), "tokens/op")
	b.ReportMetric(float64(m1.TotalAlloc-m0.TotalAlloc)/float64(b.N)/float64(len(toks)), "B/token")
}

// show renders a nullable shape field for a failure message.
func show[T any](p *T) any {
	if p == nil {
		return "null"
	}
	return *p
}
