package shape

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestRES9_BoundMatchesItsArithmetic pins maxTokens to the formula it is
// derived from and the formula to the measurement it is derived from:
// shapeBudget over the whole Derive's allocation per token on the 4 KB line,
// which is BenchmarkDerive/bash_long_4k's recorded B/op over that line's
// benchLongTokens, rounded up. A budgets.json row restated after a change
// that moves Derive's cost per token fails here until the bound is restated
// with it.
func TestRES9_BoundMatchesItsArithmetic(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "test", "perf", "budgets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var b struct {
		Benchmarks []struct {
			Name     string `json:"name"`
			Measured int64  `json:"measured_bytes_per_op"`
		} `json:"benchmarks"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	var perOp int64
	for _, r := range b.Benchmarks {
		if r.Name == "BenchmarkDerive/bash_long_4k" {
			perOp = r.Measured
		}
	}
	if perOp <= 0 {
		t.Fatal("budgets.json has no measured row for BenchmarkDerive/bash_long_4k")
	}
	perToken := (perOp + benchLongTokens - 1) / benchLongTokens
	if bytesPerToken != perToken {
		t.Errorf("bytesPerToken = %d, but the recorded Derive is %d B over %d tokens: %d per token", bytesPerToken, perOp, benchLongTokens, perToken)
	}
	if shapeBudget != 16<<20 {
		t.Errorf("shapeBudget = %d, want 16 MiB", shapeBudget)
	}
	if maxTokens != shapeBudget/bytesPerToken {
		t.Errorf("maxTokens = %d, want shapeBudget/bytesPerToken = %d", maxTokens, shapeBudget/bytesPerToken)
	}
}

// words is a line of n one-letter words after echo: n+1 tokens.
func words(n int) string {
	return "echo" + strings.Repeat(" a", n)
}

// boundBuildable stops a test that builds a line of maxTokens words when the
// bound is far past what it was derived as: a mutant that lifts it to 1 << 30
// must fail here, not build a 2 GiB line.
func boundBuildable(t *testing.T) {
	t.Helper()
	if maxTokens > 1<<20 {
		t.Fatalf("maxTokens is %d, past any line this test will build", maxTokens)
	}
}

// TestRES9_LexerStopsAtTheBound: each of the three readings of a line lexes
// exactly maxTokens tokens without complaint, and at one more stops with
// errTokenBound and the maxTokens tokens it completed, so a caller can still
// name a program that came before the bound.
func TestRES9_LexerStopsAtTheBound(t *testing.T) {
	boundBuildable(t)
	for _, tc := range []struct {
		name string
		lex  func(string) ([]token, error)
	}{
		{"tokenizeShape", tokenizeShape},
		{"tokenizeProgram", tokenizeProgram},
		{"tokenizeList", tokenizeList},
	} {
		toks, err := tc.lex(words(maxTokens - 1))
		if err != nil || len(toks) != maxTokens {
			t.Errorf("%s at the bound: %d tokens, %v; want %d and no error", tc.name, len(toks), err, maxTokens)
		}
		toks, err = tc.lex(words(maxTokens-1) + "   ")
		if err != nil || len(toks) != maxTokens {
			t.Errorf("%s at the bound with trailing blanks: %d tokens, %v; want %d and no error", tc.name, len(toks), err, maxTokens)
		}
		toks, err = tc.lex(words(maxTokens))
		if !errors.Is(err, errTokenBound) || len(toks) != maxTokens {
			t.Errorf("%s one past the bound: %d tokens, %v; want %d and errTokenBound", tc.name, len(toks), err, maxTokens)
		}
		toks, err = tc.lex(words(maxTokens-1) + ";")
		if !errors.Is(err, errTokenBound) || len(toks) != maxTokens {
			t.Errorf("%s one operator past the bound: %d tokens, %v; want %d and errTokenBound", tc.name, len(toks), err, maxTokens)
		}
	}
}

// TestRES9_TokenBoundCapsShapeMemory: 8 MiB of `echo a a a ...` is four
// million words, and lexed whole the shape of that line allocated 4.5 GB and
// drove one hook to about 1.07 GB of RSS. At the bound it allocates at most
// 32 MiB and says what it could not read: argc and the masked status are
// null and the line may write. So is the program: no separator comes before
// the bound, so the command is still open there, and `echo a a ... a () {
// ...; }` would define functions named echo and a in zsh rather than run
// anything (definesFunctions). The heredoc test below is the line whose
// program the bound keeps.
func TestRES9_TokenBoundCapsShapeMemory(t *testing.T) {
	cmd := words(4 << 20)
	if len(cmd) < 8<<20 {
		t.Fatalf("the line is %d bytes, want 8 MiB", len(cmd))
	}
	in := bashInput(t, cmd)
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	got := Derive("Bash", in, benchKey)
	runtime.ReadMemStats(&m1)
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 32<<20 {
		t.Errorf("Derive over 8 MiB of short words allocated %d MiB, want at most 32", alloc>>20)
	}
	if got.Program != nil || got.Argc != nil || got.StatusMasked != nil || !got.MayWrite {
		t.Errorf("program %v, argc %v, status_masked %v, may_write %v; want null, null, null, true past the bound",
			show(got.Program), show(got.Argc), show(got.StatusMasked), got.MayWrite)
	}
}

// heredocLine is `python3 - <<'E'`, a body of n words over lines of ten, and
// the delimiter line.
func heredocLine(n int) string {
	var b strings.Builder
	b.WriteString("python3 - <<'E'\n")
	for i := 0; i < n; i++ {
		b.WriteString("w" + strconv.Itoa(i%10))
		if i%10 == 9 {
			b.WriteByte('\n')
		} else {
			b.WriteByte(' ')
		}
	}
	b.WriteString("\nE")
	return b.String()
}

// TestRES9_HeredocKeepsItsProgram: a script handed to python3 as a
// here-document is read word by word by the program search, which does not
// skip bodies. An 18,005-token one is well under the bound and is read whole;
// one past the bound is not, and still names python3 -- the newline after its
// operator ends the command before the bound -- with argc null and the line
// marked as one that may write.
func TestRES9_HeredocKeepsItsProgram(t *testing.T) {
	boundBuildable(t)
	under := heredocLine(18_000)
	if toks, err := tokenizeShape(under); err != nil || len(toks) != 18_005 {
		t.Fatalf("the heredoc line lexes to %d tokens, %v; want 18,005", len(toks), err)
	}
	got := Derive("Bash", bashInput(t, under), benchKey)
	if got.Program == nil || *got.Program != "python3" || got.Argc == nil || *got.Argc != 18_005 {
		t.Errorf("18,005 tokens: program %v, argc %v; want python3 and 18005", show(got.Program), show(got.Argc))
	}
	got = Derive("Bash", bashInput(t, heredocLine(maxTokens)), benchKey)
	if got.Program == nil || *got.Program != "python3" || got.Argc != nil || !got.MayWrite {
		t.Errorf("past the bound: program %v, argc %v, may_write %v; want python3, null, true", show(got.Program), show(got.Argc), got.MayWrite)
	}
}
