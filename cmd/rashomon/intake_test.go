package main

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// repeatReader yields prefix and then the byte c, n bytes in all; n < 0 is
// a stream that never ends, which fails with errEndless once it has given
// twice the cap, so a reader that does not stop fails the test rather than
// hanging it. read counts what was taken from it.
type repeatReader struct {
	prefix string
	c      byte
	n      int64
	read   int64
}

var errEndless = errors.New("an endless stream was read past twice the cap")

func (r *repeatReader) Read(p []byte) (int, error) {
	if r.n >= 0 && r.read >= r.n {
		return 0, io.EOF
	}
	if r.n < 0 && r.read > 2*hookio.MaxPayloadBytes {
		return 0, errEndless
	}
	k := len(p)
	if r.n >= 0 && int64(k) > r.n-r.read {
		k = int(r.n - r.read)
	}
	for i := range p[:k] {
		if off := r.read + int64(i); off < int64(len(r.prefix)) {
			p[i] = r.prefix[off]
		} else {
			p[i] = r.c
		}
	}
	r.read += int64(k)
	return k, nil
}

// TestRES7_FinalMessageReadStopsAtTheCap: readFinalMessage takes at most
// hookio.MaxPayloadBytes+1 bytes from stdin, whatever stdin holds, so a
// payload four times the cap costs no more than one byte over it, and a
// stream that never ends is refused rather than read forever. Break: count
// the bytes without limiting the read, and both read everything.
func TestRES7_FinalMessageReadStopsAtTheCap(t *testing.T) {
	const prefix = `{"session_id":"s","last_assistant_message":"`
	r := &repeatReader{prefix: prefix, c: 'a', n: 4 * hookio.MaxPayloadBytes}
	if _, ok := readFinalMessage(r, true, false); ok {
		t.Fatal("a payload four times the cap was read as a payload")
	}
	if r.read > hookio.MaxPayloadBytes+1 {
		t.Fatalf("read %d bytes of a %d-byte payload, want at most %d", r.read, r.n, hookio.MaxPayloadBytes+1)
	}
	endless := &repeatReader{prefix: prefix, c: 'a', n: -1}
	if _, ok := readFinalMessage(endless, true, false); ok || endless.read > hookio.MaxPayloadBytes+1 {
		t.Fatalf("an endless stream: ok %v after %d bytes, want refused after at most %d", ok, endless.read, hookio.MaxPayloadBytes+1)
	}
}

// TestRES7_FinalMessageRefusesATopLevelString: the Stop payload is one
// object (or null), as decoding it into a struct required; a top-level string
// is not, whatever its text says.
func TestRES7_FinalMessageRefusesATopLevelString(t *testing.T) {
	for _, in := range []string{`""`, `"{"`, `"{\"last_assistant_message\":\"x\"}"`, `"{\"x\":1}"`} {
		for _, withSession := range []bool{true, false} {
			if fm, ok := readFinalMessage(strings.NewReader(in), withSession, false); ok {
				t.Errorf("%s (session %v): read as a payload, %+v", in, withSession, fm)
			}
		}
	}
}

// TestRES7_FinalMessageSessionIDLastCopyWins: a repeated session_id is
// read from its last copy, as a struct decode reads it (a null copy changes
// nothing, as decoding null into a string did not), and an over-long one
// costs the payload its session, never its message: the account still
// reads, and recap falls back to the newest run, as for a payload that
// names no session.
func TestRES7_FinalMessageSessionIDLastCopyWins(t *testing.T) {
	long := strings.Repeat("s", 5000)
	for _, c := range []struct {
		in, session string
	}{
		{`{"session_id":"a","session_id":"b","last_assistant_message":"m"}`, "b"},
		{`{"session_id":"` + long + `","session_id":"short","last_assistant_message":"m"}`, "short"},
		{`{"session_id":"short","session_id":"` + long + `","last_assistant_message":"m"}`, ""},
		{`{"session_id":"` + long + `","last_assistant_message":"m"}`, ""},
		{`{"session_id":"a","session_id":null,"last_assistant_message":"m"}`, "a"},
	} {
		fm, ok := readFinalMessage(strings.NewReader(c.in), true, false)
		if !ok || fm.SessionID != c.session || fm.Message != "m" {
			t.Errorf("%.60s...: ok %v session %.20q message %q, want session %q and the message", c.in, ok, fm.SessionID, fm.Message, c.session)
		}
	}
}

// TestRES7_FinalMessageMatchesStructFieldNames: the Stop payload's keys are
// matched as the old struct decode matched them, in any case.
func TestRES7_FinalMessageMatchesStructFieldNames(t *testing.T) {
	in := `{"Last_Assistant_Message":"hi","SESSION_ID":"s"}`
	var want recapPayloadStruct
	if err := json.Unmarshal([]byte(in), &want); err != nil {
		t.Fatal(err)
	}
	fm, ok := readFinalMessage(strings.NewReader(in), true, false)
	if !ok || fm.SessionID != want.SessionID || fm.Message != want.LastAssistantMessage {
		t.Fatalf("ok %v %+v, want %+v", ok, fm, want)
	}
}

// recapPayloadStruct is the struct recap and digest --stdin decoded the Stop
// payload into before it was streamed: the oracle for the tests here.
type recapPayloadStruct struct {
	SessionID            string `json:"session_id"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

// FuzzRES7_FinalMessageMatchesTheStructDecode: readFinalMessage accepts
// exactly the payloads decoding into the old structs accepted, and reads the
// same session and message from them, keys matched in any case and a
// repeated key read from its last copy. digest --stdin's struct had no
// session_id, so there its type fails nothing. Break: match keys exactly,
// and a case-variant key is lost.
func FuzzRES7_FinalMessageMatchesTheStructDecode(f *testing.F) {
	for _, s := range []string{
		`{"session_id":"s","last_assistant_message":"done"}`,
		`{"Last_Assistant_Message":"hi","SESSION_ID":"s"}`,
		`{"session_id":"a","session_id":null,"last_assistant_message":"m"}`,
		`{"session_id":5,"last_assistant_message":"m"}`,
		`{"last_assistant_message":["x"]}`,
		`{"last_assistant_message":"x"} {}`,
		`"{\"x\":1}"`,
		`null`,
		``,
		`{"ſession_id":"long s","last_assistant_message":"\ud800"}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 { // under messageHeadBytes, so the head is the whole message
			return
		}
		var old recapPayloadStruct
		oldOK := len(raw) > 0 && json.Unmarshal(raw, &old) == nil
		if oldOK && len(old.SessionID) > sessionIDBytes {
			old.SessionID = ""
		}
		fm, ok := readFinalMessage(strings.NewReader(string(raw)), true, false)
		if ok != oldOK || (ok && (fm.SessionID != old.SessionID || fm.Message != old.LastAssistantMessage)) {
			t.Fatalf("recap on %q: ok %v %q %q, struct decode ok %v %q %q", raw, ok, fm.SessionID, fm.Message, oldOK, old.SessionID, old.LastAssistantMessage)
		}
		var digest struct {
			LastAssistantMessage string `json:"last_assistant_message"`
		}
		oldOK = len(raw) > 0 && json.Unmarshal(raw, &digest) == nil
		fm, ok = readFinalMessage(strings.NewReader(string(raw)), false, false)
		if ok != oldOK || (ok && fm.Message != digest.LastAssistantMessage) {
			t.Fatalf("digest on %q: ok %v %q, struct decode ok %v %q", raw, ok, fm.Message, oldOK, digest.LastAssistantMessage)
		}
	})
}
