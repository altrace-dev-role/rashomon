package main

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// The two stdin readers recap and digest --stdin use, driven through the
// branches a healthy pipe never takes: a writer that never closes, a read
// error, a read that times out, readers that return a byte or half a buffer
// at a time, data delivered together with EOF, and a payload one byte over
// the cap. Each awkward-but-complete reader must give what a plain one gives;
// each failure must give no payload at all, never a partial one.

const test9Payload = `{"session_id":"s-test9","hook_event_name":"Stop","last_assistant_message":"All 12 tests pass."}`

// test9Readers are the readers that deliver the whole payload, awkwardly.
func test9Readers() map[string]func(io.Reader) io.Reader {
	return map[string]func(io.Reader) io.Reader{
		"plain":       func(r io.Reader) io.Reader { return r },
		"one byte":    iotest.OneByteReader,
		"half":        iotest.HalfReader,
		"data + EOF":  iotest.DataErrReader,
		"one byte+EO": func(r io.Reader) io.Reader { return iotest.DataErrReader(iotest.OneByteReader(r)) },
	}
}

var errTest9 = errors.New("test9: the read failed")

// test9Failures are the readers that never deliver a whole payload.
func test9Failures() map[string]io.Reader {
	return map[string]io.Reader{
		"an error at once":        iotest.ErrReader(errTest9),
		"a timeout midway":        iotest.TimeoutReader(strings.NewReader(test9Payload)),
		"an error after the head": io.MultiReader(strings.NewReader(test9Payload[:20]), iotest.ErrReader(errTest9)),
		"nothing":                 strings.NewReader(""),
	}
}

// sizedStop is a Stop payload exactly n bytes long.
func sizedStop(t *testing.T, n int) []byte {
	t.Helper()
	head := `{"session_id":"s-test9","last_assistant_message":"`
	tail := `"}`
	body := bytes.Repeat([]byte("x"), n-len(head)-len(tail))
	p := append(append([]byte(head), body...), tail...)
	if len(p) != n {
		t.Fatalf("built %d bytes, want %d", len(p), n)
	}
	return p
}

func TestRubricTEST9_RecapReaderGivesUpOnAWriterThatNeverCloses(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		// The reader goroutine is parked in Read on the open pipe. Seeing it
		// in the dump first proves the name matches a real frame, so the
		// absence check below cannot pass by looking for the wrong name.
		requireGoroutine(t, recapReaderFrame)
		// Closing the writer is what lets the abandoned goroutine's Read
		// return; it then sends into a buffered channel and exits.
		if err := pw.Close(); err != nil {
			t.Errorf("closing the pipe: %v", err)
		}
		waitForNoGoroutine(t, recapReaderFrame)
	})

	done := make(chan recapPayload, 1)
	start := time.Now()
	go func() { done <- readRecapPayload(pr, 50*time.Millisecond) }()
	select {
	case got := <-done:
		if got != (recapPayload{}) {
			t.Errorf("an open pipe with nothing on it gave %+v, want no payload", got)
		}
		if took := time.Since(start); took > time.Second {
			t.Errorf("a 50ms budget took %s", took)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readRecapPayload is still waiting on a pipe nobody writes to after 2s; its timeout did not fire")
	}
}

// recapReaderFrame is the frame of the goroutine readRecapPayload starts. In
// a test binary package main is named by its import path, not "main", so the
// frame reads ".../cmd/rashomon.readRecapPayload.func1".
const recapReaderFrame = "cmd/rashomon.readRecapPayload.func"

// requireGoroutine fails the test unless some goroutine's stack names fn.
func requireGoroutine(t *testing.T, fn string) {
	t.Helper()
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	if !bytes.Contains(buf[:n], []byte(fn)) {
		t.Errorf("no goroutine names %s, so its exit cannot be observed:\n%s", fn, buf[:n])
	}
}

// waitForNoGoroutine polls the goroutine dump until no stack names fn.
func waitForNoGoroutine(t *testing.T, fn string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if !bytes.Contains(buf[:n], []byte(fn)) {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("a goroutine in %s is still running 2s after its pipe closed:\n%s", fn, buf[:n])
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRubricTEST9_RecapReaderAwkwardAndFailingReaders(t *testing.T) {
	want := recapPayload{SessionID: "s-test9", LastAssistantMessage: "All 12 tests pass."}
	for name, wrap := range test9Readers() {
		if got := readRecapPayload(wrap(strings.NewReader(test9Payload)), time.Second); got != want {
			t.Errorf("%s reader: %+v, want %+v", name, got, want)
		}
	}
	for name, r := range test9Failures() {
		if got := readRecapPayload(r, time.Second); got != (recapPayload{}) {
			t.Errorf("%s: %+v, want no payload", name, got)
		}
	}
}

func TestRubricTEST9_DigestStdinAwkwardAndFailingReaders(t *testing.T) {
	for name, wrap := range test9Readers() {
		msg, ok := readStdinLastMessage(wrap(strings.NewReader(test9Payload)))
		if !ok || msg != "All 12 tests pass." {
			t.Errorf("%s reader: %q %v, want the message", name, msg, ok)
		}
	}
	for name, r := range test9Failures() {
		if msg, ok := readStdinLastMessage(r); ok || msg != "" {
			t.Errorf("%s: %q %v, want no message", name, msg, ok)
		}
	}
}

// TestRubricTEST9_TheCapIsMaxPayloadBytes: both readers take a payload of
// exactly hookio.MaxPayloadBytes and refuse one byte more, as the buffered
// hook entries do (TestRES4_CapAtNAndNPlusOne).
func TestRubricTEST9_TheCapIsMaxPayloadBytes(t *testing.T) {
	at := sizedStop(t, hookio.MaxPayloadBytes)
	over := sizedStop(t, hookio.MaxPayloadBytes+1)
	if got := readRecapPayload(bytes.NewReader(at), 5*time.Second); got.SessionID != "s-test9" {
		t.Errorf("recap refused a payload of exactly MaxPayloadBytes")
	}
	if got := readRecapPayload(bytes.NewReader(over), 5*time.Second); got != (recapPayload{}) {
		t.Errorf("recap took a payload one byte over MaxPayloadBytes (session %q)", got.SessionID)
	}
	if _, ok := readStdinLastMessage(bytes.NewReader(at)); !ok {
		t.Errorf("digest --stdin refused a payload of exactly MaxPayloadBytes")
	}
	if _, ok := readStdinLastMessage(bytes.NewReader(over)); ok {
		t.Errorf("digest --stdin took a payload one byte over MaxPayloadBytes")
	}
}
