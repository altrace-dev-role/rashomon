package hookio

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// TestRES5_IntakeMatchesStructFieldNames: a payload key is found wherever
// decoding into a struct field of that name found it, in any case, the last
// match winning. Break: match keys exactly, and a wrong-case session_id
// leaves the call unattributed.
func TestRES5_IntakeMatchesStructFieldNames(t *testing.T) {
	in := `{"SESSION_ID":"s","Cwd":"/p","session_Id":"last","TOOL_INPUT":{"command":"ls"}}`
	var want struct {
		SessionID string          `json:"session_id"`
		CWD       string          `json:"cwd"`
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal([]byte(in), &want); err != nil {
		t.Fatal(err)
	}
	p, err := Intake(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	var session, cwd string
	if err := p.Strs(Into(SessionID, &session), Into(CWD, &cwd)); err != nil {
		t.Fatal(err)
	}
	if session != want.SessionID || cwd != want.CWD || string(p.Value(ToolInput)) != string(want.ToolInput) {
		t.Fatalf("got %q %q %s, want %q %q %s", session, cwd, p.Value(ToolInput), want.SessionID, want.CWD, want.ToolInput)
	}
}

// TestStrsFillsTheFieldPairedWithEachKey: each destination is named beside
// its key, so the order of the pairs is free.
func TestStrsFillsTheFieldPairedWithEachKey(t *testing.T) {
	p, err := Intake(strings.NewReader(`{"session_id":"s","cwd":"/p"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session, cwd string
	if err := p.Strs(Into(CWD, &cwd), Into(SessionID, &session)); err != nil || session != "s" || cwd != "/p" {
		t.Fatalf("session %q cwd %q: %v", session, cwd, err)
	}
}

// TestRES7_IntakeStreamRejectsANonObjectRoot: a streamed payload is one
// object or null, as decoding it into a struct required. The root's kind is
// checked on its first byte, so a top-level string whose text starts with a
// brace is refused, before fn sees anything. Break: check the first decoded
// byte instead, and "{" and "{\"x\":1}" as strings pass.
func TestRES7_IntakeStreamRejectsANonObjectRoot(t *testing.T) {
	paths := []jsonscan.Path{{Elems: []string{"x"}, Max: 8, Mode: jsonscan.CaptureHead}}
	for _, in := range []string{`""`, `"{"`, `"{\"x\":1}"`, ` "{}" `, `["{"]`, `[{"x":1}]`, `1`, `true`, `false`} {
		called := false
		err := IntakeStream(strings.NewReader(in), paths, func(int, []byte, bool) bool { called = true; return true })
		if !errors.Is(err, jsonscan.ErrNotObject) || called {
			t.Errorf("%s: err %v, fn called %v; want ErrNotObject before any call", in, err, called)
		}
	}
	for _, in := range []string{`{"x":1}`, " \n{}", `null`} {
		if err := IntakeStream(strings.NewReader(in), paths, func(int, []byte, bool) bool { return true }); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	if err := IntakeStream(strings.NewReader(`nul`), paths, func(int, []byte, bool) bool { return true }); err == nil {
		t.Error("a broken null passed")
	}
}

// TestRES7_IntakeStreamReportsAnEarlyStop: fn stopping the walk leaves the rest
// of the payload unread and unvalidated, and IntakeStream says so rather
// than returning nil for a payload it never finished.
func TestRES7_IntakeStreamReportsAnEarlyStop(t *testing.T) {
	paths := []jsonscan.Path{{Elems: []string{"x"}, Max: 8, Mode: jsonscan.CaptureHead}}
	err := IntakeStream(strings.NewReader(`{"x":1, this is not JSON`), paths, func(int, []byte, bool) bool { return false })
	if !errors.Is(err, ErrStopped) {
		t.Fatalf("err %v, want ErrStopped", err)
	}
}

// overCap is a payload one byte over the cap whose first bytes are head.
func overCap(head string) []byte {
	head += `"tool_name":"Write","tool_input":{"content":"`
	tail := `"}}`
	return []byte(head + strings.Repeat("x", MaxPayloadBytes+1-len(head)-len(tail)) + tail)
}

// TestRES4_ReadRefusesAsIntakeDoes: Read's refusal names the session and
// cwd in the payload's head as Intake's does, and Intake hands on a refusal
// its reader returns rather than re-deriving one from a head it never saw.
// The host guard reads with Read and replays the error to Intake. Break
// either, and every refusal behind the guard lands in the unattributed run.
func TestRES4_ReadRefusesAsIntakeDoes(t *testing.T) {
	_, err := Read(bytes.NewBuffer(overCap(`{"session_id":"s-big","cwd":"/p",`)))
	var tl *TooLargeError
	if !errors.As(err, &tl) {
		t.Fatalf("Read: %v (%T), want a *TooLargeError", err, err)
	}
	if tl.SessionID != "s-big" || tl.CWD != "/p" {
		t.Fatalf("Read: session %q cwd %q, want s-big /p", tl.SessionID, tl.CWD)
	}
	_, err = Intake(iotest.ErrReader(tl))
	var got *TooLargeError
	if !errors.As(err, &got) || got != tl {
		t.Fatalf("Intake over a replayed refusal: %v, want the refusal itself", err)
	}
}

// TestRES4_TooLargeNamesTheSessionInItsHead: a refused payload is not
// parsed, but its first 4 KiB, already read, still name the session and cwd
// its refusal is recorded under, as the supervisor scans that head (Technical
// spec 4.2.10). A value that does not end inside the head names nothing.
// Break: drop the head, and every refusal lands in the unattributed run.
func TestRES4_TooLargeNamesTheSessionInItsHead(t *testing.T) {
	for _, c := range []struct {
		name, head, session, cwd string
	}{
		{"named", `{"session_id":"s-big","cwd":"/p",`, "s-big", "/p"},
		{"case variant", `{"Session_Id":"s-case","CWD":"/q",`, "s-case", "/q"},
		{"last in the head wins", `{"session_id":"a","session_id":"b",`, "b", ""},
		{"wrong type", `{"session_id":7,"cwd":"/p",`, "", ""},
		{"not an object", `["session_id","s",`, "", ""},
		{"broken before its end", `{"session_id":"s-bad",,`, "", ""},
		{"past the head", `{"pad":"` + strings.Repeat("y", 5000) + `","session_id":"late",`, "", ""},
		{"cut by the head", `{"session_id":"` + strings.Repeat("z", 5000) + `",`, "", ""},
	} {
		_, err := Intake(bytes.NewReader(overCap(c.head)))
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("%s: %v, want ErrTooLarge", c.name, err)
		}
		var tl *TooLargeError
		if !errors.As(err, &tl) {
			t.Fatalf("%s: %T is not a *TooLargeError", c.name, err)
		}
		if tl.SessionID != c.session || tl.CWD != c.cwd {
			t.Errorf("%s: session %q cwd %q, want %q %q", c.name, tl.SessionID, tl.CWD, c.session, c.cwd)
		}
	}
}
