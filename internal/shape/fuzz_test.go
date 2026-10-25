package shape

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/host"
)

// Fuzz targets over the hand-written shell lexer and the fields read from
// tool_input. Each input is handed to a parser only; none is ever run.
//
// In a hook, a recovered panic costs the call's declaration and a runtime
// fatal error, stack exhaustion among them, exits 2 and blocks the user's tool
// call. Beside "no panic", each target holds its function to the closed
// vocabulary or the shape its record field promises.

// shapeSeeds are the corpora the table tests already hold, the nesting bound
// passed by one in both its quoted and unquoted forms, and a few lines that
// name hosts and directories.
func shapeSeeds() []string {
	seeds := slices.Clone(leakCorpus)
	for _, c := range programClaims {
		seeds = append(seeds, c.in)
	}
	return append(seeds,
		"echo "+strings.Repeat("$(", maxNest+1)+"x"+strings.Repeat(")", maxNest+1),
		"echo "+strings.Repeat(`"$(`, maxNest+1)+"x"+strings.Repeat(`)"`, maxNest+1),
		"echo "+strings.Repeat("`", 2*(maxNest+1)),
		"cd /srv/app && cd sub && go test ./...",
		"cd ~/proj && make",
		"cd ~user/proj && make",
		"cd -x && ls",
		"cd a && pushd b && make test 2>&1 | tail -40",
		"curl -sS https://pypi.org/simple/ && git push git@github.com:o/r.git",
		"ssh -p 22 deploy@build.example.internal 'ls' ; scp f bastion:/tmp",
		"wget http://[::1]:8080/x https://Example.COM./y https://bücher.example/z",
		"",
		"\x00#hunter2",
		"a\\\nb && c",
		// Inputs the CI fuzz command found for FuzzHosts: a git@ text match
		// whose host carries a byte no hostname has, and a host with two
		// trailing dots, which host.Canonical once reduced to one. Beside
		// them, the command-argument form of a host with '!' in it.
		"&git@pusmaiECRET\x9b", " git@\xa0", " cuhttps://cret-token..", "ssh h.example.com!x",
	)
}

func FuzzDerive(f *testing.F) {
	for _, s := range shapeSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, cmd string) { checkDerive(t, cmd) })
}

func checkDerive(t *testing.T, cmd string) {
	t.Helper()
	raw := bashInput(t, cmd)
	s := Derive("Bash", raw, []byte("fuzz-key"))
	if !slices.Contains(VerbClasses(), s.VerbClass) {
		t.Fatalf("verb class %q for %q is outside VerbClasses()", s.VerbClass, cmd)
	}
	if s.Argc != nil && *s.Argc < 0 {
		t.Fatalf("argc %d for %q", *s.Argc, cmd)
	}
	if p := s.Program; p != nil {
		if *p == "" || strings.Contains(*p, "/") || strings.ContainsFunc(*p, isControl) {
			t.Fatalf("program %q for %q is not a command name", *p, cmd)
		}
	}
	if !isDigest(s.Digest) {
		t.Fatalf("digest %q for %q is not 64 hex characters", s.Digest, cmd)
	}
	if s.StatusMasked != nil && !slices.Contains(MaskedKinds(), *s.StatusMasked) {
		t.Fatalf("status_masked %q for %q is outside MaskedKinds()", *s.StatusMasked, cmd)
	}
	if s.RunnerDigest != nil && !isDigest(*s.RunnerDigest) {
		t.Fatalf("runner digest %q for %q is not 64 hex characters", *s.RunnerDigest, cmd)
	}
	if again := Derive("Bash", raw, []byte("fuzz-key")); !reflect.DeepEqual(s, again) {
		t.Fatalf("Derive(%q) is not deterministic: %+v then %+v", cmd, s, again)
	}
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

func isDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && strings.ToLower(s) == s
}

func FuzzHosts(f *testing.F) {
	for _, s := range shapeSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) { checkHosts(t, text) })
}

// checkHosts reads text both as a Bash command line and as a WebFetch url.
// Every wire and ssh host is already canonical (host.Canonical is a fixed
// point on it), every host a command ARGUMENT names is hostname-shaped and in
// the ssh list, and both lists are sorted and de-duplicated, as the record's
// determinism needs.
//
// Hostname-shaped is the argument reader's contract, not the whole list's.
// The ssh:// and git@ forms are matched in the text, like the wire schemes,
// and admit what host.Canonical admits, a byte >= 0x80 among it: the first
// version of this property held them to the argument reader's whitelist, and
// the CI fuzz command found `git@` before such a byte in under 13 s.
func checkHosts(t *testing.T, text string) {
	t.Helper()
	url, err := json.Marshal(map[string]string{"url": text})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		tool string
		raw  json.RawMessage
	}{{"Bash", bashInput(t, text)}, {"WebFetch", url}} {
		wire, ssh := Hosts(call.tool, call.raw)
		for _, h := range slices.Concat(wire, ssh) {
			if c, ok := host.Canonical(h); !ok || c != h {
				t.Fatalf("%s %q: host %q is not canonical (%q, %v)", call.tool, text, h, c, ok)
			}
		}
		if cmd, ok := commandField(call.raw); ok && call.tool == "Bash" {
			args, _ := sshCommandHosts(cmd)
			for _, h := range args {
				if !host.Shaped(h) || !slices.Contains(ssh, h) {
					t.Fatalf("%s %q: argument host %q is not hostname-shaped or not in %q", call.tool, text, h, ssh)
				}
			}
		}
		for _, list := range [][]string{wire, ssh} {
			for i := 1; i < len(list); i++ {
				if list[i-1] >= list[i] {
					t.Fatalf("%s %q: hosts %q are not sorted and unique", call.tool, text, list)
				}
			}
		}
		w2, s2 := Hosts(call.tool, call.raw)
		if !slices.Equal(wire, w2) || !slices.Equal(ssh, s2) {
			t.Fatalf("Hosts(%s, %q) is not deterministic", call.tool, text)
		}
	}
}

func FuzzLabel(f *testing.F) {
	for _, tool := range []string{"Read", "Edit", "Write", "NotebookEdit", "Bash", "WebFetch"} {
		for _, in := range []string{
			`{"file_path":"/home/u/.ssh/id_ed25519"}`, `{"file_path":"x_test.go"}`,
			`{"notebook_path":".env.local"}`, `{"file_path":""}`, `{"file_path":7}`, `"str"`, `{bad`,
			`{"file_path":"~/.aws/credentials","file_path":"a"}`, `{"file_path":"C:\\Users\\u\\cert.PEM"}`,
		} {
			f.Add(tool, []byte(in))
		}
	}
	f.Fuzz(func(t *testing.T, tool string, raw []byte) { checkLabel(t, tool, raw) })
}

// checkLabel: a tool that names no file gets "", any other gets a word from
// Labels(), whatever its input holds.
func checkLabel(t *testing.T, tool string, raw []byte) {
	t.Helper()
	got := Label(tool, raw)
	if _, names := pathFields[tool]; !names {
		if got != "" {
			t.Fatalf("Label(%q, %q) = %q for a tool that names no file", tool, raw, got)
		}
		return
	}
	if !slices.Contains(Labels(), got) {
		t.Fatalf("Label(%q, %q) = %q, outside Labels()", tool, raw, got)
	}
	if again := Label(tool, raw); again != got {
		t.Fatalf("Label(%q, %q) is not deterministic: %q then %q", tool, raw, got, again)
	}
}

func FuzzLeadingDirectory(f *testing.F) {
	for _, s := range shapeSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, cmd string) { checkLeadingDirectory(t, cmd) })
}

// checkLeadingDirectory: ok says whether there are directories, each is a word
// the shell takes as it stands or the home the hook expands (`~` alone or
// before a slash, never a named home or the directory stack's `~+` and `~-`),
// and the run is read one step past MaxLeadingCd and no further, the step that
// says the line is past it.
func checkLeadingDirectory(t *testing.T, cmd string) {
	t.Helper()
	dirs, ok := LeadingDirectory("Bash", bashInput(t, cmd))
	if ok != (len(dirs) > 0) {
		t.Fatalf("LeadingDirectory(%q) = %q, %v", cmd, dirs, ok)
	}
	for _, d := range dirs {
		if d == "" || strings.HasPrefix(d, "-") || strings.HasPrefix(d, "~") && d != "~" && !strings.HasPrefix(d, "~/") ||
			strings.ContainsAny(d, "$`*?[{\\'\"") || strings.ContainsFunc(d, isControl) {
			t.Fatalf("LeadingDirectory(%q) names %q, which the shell does not take as it stands and is not the home", cmd, d)
		}
	}
	if len(dirs) > MaxLeadingCd+1 {
		t.Fatalf("LeadingDirectory(%q) read %d steps, past MaxLeadingCd+1", cmd, len(dirs))
	}
}

// TestRubricTEST1_ShapeSeeds runs every fuzz property over the seeds as a
// plain test, which is what a mutant names and what runs without -fuzz.
func TestRubricTEST1_ShapeSeeds(t *testing.T) {
	for _, s := range shapeSeeds() {
		checkDerive(t, s)
		checkHosts(t, s)
		checkLeadingDirectory(t, s)
		raw, err := json.Marshal(map[string]string{"file_path": s})
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range []string{"Read", "NotebookEdit", "Bash"} {
			checkLabel(t, tool, raw)
		}
	}
}
