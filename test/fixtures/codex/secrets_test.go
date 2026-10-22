package codex

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// secretPatterns are what no fixture in this directory may hold: a home
// directory other than the placeholder /home/user, an email address, and the
// shapes of common credentials. The fixtures come from real sessions on a
// real machine, so this runs on every file, every time.
var secretPatterns = map[string]*regexp.Regexp{
	"a macOS home directory": regexp.MustCompile(`/Users/[A-Za-z0-9._-]+`),
	"a home directory":       regexp.MustCompile(`/home/([A-Za-z0-9._-]+)`),
	"an email address":       regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
	"an API key":             regexp.MustCompile(`\b(sk|pk|rk)-[A-Za-z0-9_-]{16,}`),
	"a bearer token":         regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/-]{16,}`),
	"an AWS access key":      regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	"a private key":          regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	"a GitHub token":         regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`),
}

// findSecrets returns what each pattern found in text, by pattern name.
func findSecrets(text string) map[string][]string {
	out := map[string][]string{}
	for name, re := range secretPatterns {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if name == "a home directory" && m[1] == "user" {
				continue
			}
			out[name] = append(out[name], m[0])
		}
	}
	return out
}

// TestScannerFindsWhatItLooksFor is the scan's positive twin: a scan that
// matched nothing would pass every fixture.
func TestScannerFindsWhatItLooksFor(t *testing.T) {
	planted := "/Users/someone/x /home/someone/y someone@example.com sk-ABCDEFGHIJKLMNOPqrstuv " +
		"Bearer abcdefghijklmnopqrstuvwx AKIAABCDEFGHIJKLMNOP -----BEGIN RSA PRIVATE KEY----- " +
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	found := findSecrets(planted)
	for name := range secretPatterns {
		if len(found[name]) == 0 {
			t.Errorf("the scan does not find %s in %q", name, planted)
		}
	}
	if got := findSecrets("/home/user/hook-test"); len(got) != 0 {
		t.Errorf("the placeholder home directory reads as a secret: %v", got)
	}
}

// TestFixturesHoldNoSecrets scans every file under this directory.
func TestFixturesHoldNoSecrets(t *testing.T) {
	n := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, "_test.go") {
			return err
		}
		n++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for name, hits := range findSecrets(string(body)) {
			t.Errorf("%s holds %s: %q", path, name, hits)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 30 {
		t.Fatalf("scanned %d files; the directory holds the 28 payloads, the phase 4 rollouts and the READMEs", n)
	}
}

// TestPhase4RolloutsKeepOnlyWhatTheReaderReads: in the phase 4 rollout lines,
// every item field other than the four the report reads is scrubbed, and the
// lines hold nothing but item_completed entries for tool items.
func TestPhase4RolloutsKeepOnlyWhatTheReaderReads(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("phase4", "rollout-*.jsonl"))
	if err != nil || len(paths) != 3 {
		t.Fatalf("found %d phase 4 rollouts (%v), want 3", len(paths), err)
	}
	read := map[string]bool{"type": true, "id": true, "exit_code": true, "status": true}
	tools := map[string]bool{"CommandExecution": true, "FileChange": true, "CollabAgentToolCall": true, "SubAgentActivity": true}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		for line := 1; sc.Scan(); line++ {
			var o struct {
				Type    string `json:"type"`
				Payload struct {
					Type string                     `json:"type"`
					Item map[string]json.RawMessage `json:"item"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
				t.Errorf("%s:%d: %v", path, line, err)
				continue
			}
			var typ string
			_ = json.Unmarshal(o.Payload.Item["type"], &typ)
			if o.Type != "event_msg" || o.Payload.Type != "item_completed" || !tools[typ] {
				t.Errorf("%s:%d: a %s/%s %q line, not a tool item", path, line, o.Type, o.Payload.Type, typ)
			}
			for k, v := range o.Payload.Item {
				switch s := string(v); {
				case read[k], s == `"[scrubbed]"`, s == "0", s == "null":
				default:
					t.Errorf("%s:%d: item.%s is %s, not scrubbed", path, line, k, s)
				}
			}
		}
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close() //nolint:errcheck // read-only
	}
}
