package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

// H-112 -- a captured payload is scrubbed before it is committed.
//
// A capture from a real machine carries the machine. A session's init event
// lists every tool, skill, plugin, MCP server, slash command and agent the
// account has, and a skill or plugin name can name an employer or a client;
// paths carry the user's name and the session's id; a transcript can hold an
// address, or a token pasted into a prompt. A capture archive on the #84
// branch carried exactly this, and a secret scan of it was clean, because
// none of it is a credential. test/tools/scrub is what a contributor runs on
// a capture before committing it: every one of those shapes goes, a join
// survives (one id, in two places or two files, gets one placeholder), and
// every other byte stays, so the fixture still exercises what the capture
// did.
//
// Break: keep the inventory lists, and the account's skill and plugin names
// reach the fixture while every other shape is still removed.

// The planted values. Each is built here rather than written out, so this
// file holds no string a credential scanner would take for a live one.
var (
	h112Session    = "5f0c8a12-3b7e-4c1d-9a2f-6e8b4d0c7a31"
	h112Scratch    = "c4e9b7a0-1d2f-4e3a-8b5c-7f6a9d0e2b18"
	h112Event      = "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	h112Message    = "7e6d5c4b-3a29-4180-9f7e-6d5c4b3a2918"
	h112Token      = "ghp_" + strings.Repeat("Zq9", 12)
	h112Bearer     = "q8Zr4LmP0xT7vK2nW9yB3cHd" + "J5sF1gRe"
	h112Leaf       = strings.Repeat("0123456789abcdef", 4)
	h112Chain      = strings.Repeat("89abcdef01234567", 4)
	h112Stray      = strings.Repeat("fedcba9876543210", 4)
	h112Home       = "/srv/people/alice"
	h112Identities = []string{"alice", "acme", "liddell", "hunter2", "/tmp/", "/home/", `C:\\Users`, "/srv/people",
		h112Session, h112Scratch, h112Event, h112Message, h112Token, h112Bearer, h112Stray}
)

const (
	h112P1 = "00000000-0000-4000-8000-000000000001"
	h112P2 = "00000000-0000-4000-8000-000000000002"
)

// h112Capture is an init event, a PreToolUse payload, a nono-style chain
// record and a transcript line whose tool result holds a payload as text.
func h112Capture() string {
	r := strings.NewReplacer("U1", h112Session, "U2", h112Scratch, "U3", h112Event, "GHP", h112Token,
		"BEARER", h112Bearer, "LEAF", h112Leaf, "CHAIN", h112Chain, "STRAY", h112Stray)
	return r.Replace(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"U1","cwd":"/home/alice/src/app","tools":["Bash","Read","Edit","mcp__acme-crm__lookup"],"mcp_servers":[{"name":"acme-crm","status":"connected"}],"slash_commands":["acme-deploy","review"],"agents":["general-purpose","acme-auditor"],"skills":["acme-brand","pdf"],"plugins":[{"name":"acme-legal","path":"/home/alice/.claude/plugins/acme-legal"}],"apiKeySource":"none","uuid":"U3"}`,
		`{"session_id":"U1","transcript_path":"/home/alice/.claude/projects/-home-alice-src-app/U1.jsonl","cwd":"/home/alice/src/app","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git commit -m fix --author 'A <alice.liddell@acme-corp.io>' && curl -H 'Authorization: Bearer BEARER' https://api.example.com/v1","description":"commit and ping"},"env":{"GITHUB_TOKEN":"GHP","password":"hunter2"},"scratch":"/tmp/claude-1000/-home-alice-src-app/U2/scratchpad/notes.txt","build":"/tmp/go-build123/b001/app.test","notes":"/srv/people/alice/notes/todo.txt","win":"C:\\Users\\alice\\repo","wintemp":"C:\\Users\\alice\\AppData\\Local\\Temp\\x.txt","leaf_hash":"LEAF","note":"STRAY","label":"caf\u00e9 \/ ok","ts":1789851471548}`,
		`{"sequence":1,"prev_chain":"LEAF","chain_hash":"CHAIN"}`,
		`{"type":"user","sessionId":"U1","message":{"content":[{"type":"tool_result","content":"{\"skills\":[\"acme-brand\"],\"cwd\":\"/home/alice/src/app\"}"}]}}`,
	}, "\n") + "\n")
}

// scrub runs the scrubber with home as the machine's home directory.
func scrub(t *testing.T, dir, home string, args ...string) result {
	t.Helper()
	cmd := exec.Command(scrubBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running the scrubber: %v", err)
	}
	return result{exitCode: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestH112_AScrubbedCaptureCarriesNoAccountShape(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture.ndjson")
	if err := os.WriteFile(capture, []byte(h112Capture()), 0o600); err != nil {
		t.Fatal(err)
	}
	res := scrub(t, dir, h112Home, "capture.ndjson")
	if res.exitCode != 0 || res.stderr != "" {
		t.Fatalf("scrub: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	out := res.stdout

	for _, v := range h112Identities {
		if strings.Contains(out, v) {
			t.Errorf("the scrubbed capture still holds %q", v)
		}
	}
	if found := hygiene.Scan("capture.ndjson", []byte(out)); len(found) != 0 {
		t.Errorf("the scan H-113 runs finds what scrub left: %v", found)
	}

	// The inventory is its count, the init event otherwise intact.
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("scrub turned 4 lines into %d:\n%s", len(lines), out)
	}
	var initEvent map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &initEvent); err != nil {
		t.Fatalf("the init event no longer parses: %v\n%s", err, lines[0])
	}
	for name, n := range map[string]float64{"tools": 4, "mcp_servers": 1, "slash_commands": 2, "agents": 2, "skills": 2, "plugins": 1} {
		if initEvent[name] != n {
			t.Errorf("init %s = %v, want its count %v", name, initEvent[name], n)
		}
	}
	if initEvent["subtype"] != "init" || initEvent["apiKeySource"] != "none" || initEvent["uuid"] != h112P2 {
		t.Errorf("init event lost what was not a shape, or its uuid's placeholder: %v", initEvent)
	}

	// One session id, three places, one placeholder; member order and every
	// byte between the rewrites as captured.
	wantPrefix := `{"session_id":"` + h112P1 + `","transcript_path":"~/.claude/projects/-project/` + h112P1 +
		`.jsonl","cwd":"~/src/app","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git commit -m fix --author 'A <user-1@example.invalid>' && curl -H 'Authorization: ` +
		`[redacted]`
	if !strings.HasPrefix(lines[1], wantPrefix) {
		t.Errorf("payload line:\n got %s\nwant it to begin %s", lines[1], wantPrefix)
	}
	for _, kept := range []string{
		` https://api.example.com/v1","description":"commit and ping"}`,
		`"env":{"GITHUB_TOKEN":"[redacted]","password":"[redacted]"}`,
		`"scratch":"$SCRATCHPAD/notes.txt","build":"$TMPDIR/go-build123/b001/app.test","notes":"~/notes/todo.txt"`,
		`"win":"~\\repo","wintemp":"$TMPDIR\\x.txt"`,
		`"leaf_hash":"` + h112Leaf + `","note":"[redacted]","label":"caf\u00e9 \/ ok","ts":1789851471548}`,
	} {
		if !strings.Contains(lines[1], kept) {
			t.Errorf("payload line lacks %s:\n%s", kept, lines[1])
		}
	}
	if want := `{"sequence":1,"prev_chain":"` + h112Leaf + `","chain_hash":"` + h112Chain + `"}`; lines[2] != want {
		t.Errorf("chain record = %s, want the digests its file names kept: %s", lines[2], want)
	}
	if want := `{"type":"user","sessionId":"` + h112P1 + `","message":{"content":[{"type":"tool_result","content":"{\"skills\":1,\"cwd\":\"~/src/app\"}"}]}}`; lines[3] != want {
		t.Errorf("transcript line = %s\nwant the payload inside its text scrubbed too: %s", lines[3], want)
	}

	// Scrubbing what scrub wrote changes nothing.
	if err := os.WriteFile(capture, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	if again := scrub(t, dir, h112Home, "capture.ndjson"); again.exitCode != 0 || again.stdout != out {
		t.Errorf("a second scrub changed the output (exit %d):\n%s", again.exitCode, again.stdout)
	}
}

func TestH112_PlaceholdersHoldAcrossTheFilesOfOneRun(t *testing.T) {
	dir := t.TempDir()
	payload := "{\n  \"session_id\": \"" + h112Session + "\",\n  \"transcript_path\": \"/home/alice/.claude/projects/-home-alice-src-app/" +
		h112Session + ".jsonl\",\n  \"tool_name\": \"Read\"\n}\n"
	transcript := `{"sessionId":"` + h112Session + `","uuid":"` + h112Message + `","parentUuid":null}` + "\n"
	for name, body := range map[string]string{"payload.json": payload, "transcript.jsonl": transcript} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if res := scrub(t, dir, h112Home, "-w", "payload.json", "transcript.jsonl"); res.exitCode != 0 || res.stderr != "" {
		t.Fatalf("scrub -w: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	gotPayload, _ := os.ReadFile(filepath.Join(dir, "payload.json"))
	gotTranscript, _ := os.ReadFile(filepath.Join(dir, "transcript.jsonl"))
	wantPayload := "{\n  \"session_id\": \"" + h112P1 + "\",\n  \"transcript_path\": \"~/.claude/projects/-project/" +
		h112P1 + ".jsonl\",\n  \"tool_name\": \"Read\"\n}\n"
	if string(gotPayload) != wantPayload {
		t.Errorf("payload.json:\n%s\nwant\n%s", gotPayload, wantPayload)
	}
	if want := `{"sessionId":"` + h112P1 + `","uuid":"` + h112P2 + `","parentUuid":null}` + "\n"; string(gotTranscript) != want {
		t.Errorf("transcript.jsonl = %s, want the session's placeholder from payload.json and the next one for its own id: %s",
			gotTranscript, want)
	}
}

func TestH112_WhatScrubCannotRewriteIsNamedNotPrinted(t *testing.T) {
	dir := t.TempDir()
	body := "hook stdin was: {\"skills\":[\"acme-brand\"]} (cut)\n"
	if err := os.WriteFile(filepath.Join(dir, "capture.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	res := scrub(t, dir, h112Home, "capture.txt")
	if res.exitCode != 1 {
		t.Errorf("exit %d for an inventory scrub cannot rewrite, want 1 (the file still needs a person)", res.exitCode)
	}
	if !strings.Contains(res.stderr, "capture.txt:1:") || !strings.Contains(res.stderr, "inventory") {
		t.Errorf("stderr does not say where the inventory is: %q", res.stderr)
	}
	if strings.Contains(res.stderr, "acme") {
		t.Errorf("stderr prints what it found: %q", res.stderr)
	}
}
