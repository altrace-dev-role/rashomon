package spec

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// FuzzVER37_ParseNeverPanicsAndKeepsNothing: Parse reads an untrusted file, so
// whatever the bytes are it must not panic, must answer within its bounds, and
// must not keep the input.
//
//   - bounded: an input over MaxBytes is refused as too large; every error is
//     an *Error with a closed reason, a pointer no longer than the depth cap
//     allows, and a message of the same bound; a File never carries more than
//     the caps allow;
//   - all or nothing: an error returns no File, and a File comes with a digest;
//   - no retention: the input is overwritten after the call, and the File reads
//     back as it did before, equal to a parse of an untouched copy;
//   - the digest is the document's: re-indenting a document that parsed gives
//     the same digest.
func FuzzVER37_ParseNeverPanicsAndKeepsNothing(f *testing.F) {
	for _, s := range []string{
		specExample, katDoc, top(""), ``, `{`, `[]`, "\x00", `{"rashomon_spec": 1.5}`,
		withRule(kv{"why", `"\ud800"`}), withRule(kv{"why", "\"\xff\""}), withRule(kv{"pattern", `"(?:a{2}){3}"`}),
		strings.Repeat("[", 200), strings.Repeat(`{"a":`, 200),
		withRule(kv{"pattern", q(pat2000)}), withGate(kv{"success_line", `["("]`}),
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		kept := append([]byte(nil), data...)
		file, digest, err := Parse(data)
		if len(kept) > MaxBytes {
			if e, ok := err.(*Error); !ok || e.Reason != ReasonTooLarge {
				t.Fatalf("%d bytes: %v", len(kept), err)
			}
		}
		if err != nil {
			e, ok := err.(*Error)
			if !ok || e.Reason == "" || file != nil {
				t.Fatalf("error %v (%T) with file %v", err, err, file)
			}
			if len(e.Pointer) > MaxDepth*(MaxIDBytes+1) || len(e.Error()) > 4*MaxDepth*(MaxIDBytes+1) {
				t.Fatalf("unbounded error: %d byte pointer", len(e.Pointer))
			}
			return
		}
		if file == nil || digest == "" {
			t.Fatalf("accepted with file %v and digest %q", file, digest)
		}
		if len(file.Gates)+len(file.CICommands) > MaxGates || len(file.Rules) > MaxRules || len(file.Facts) > MaxFacts {
			t.Fatalf("a File over its caps: %d gates, %d ci, %d rules, %d facts", len(file.Gates), len(file.CICommands), len(file.Rules), len(file.Facts))
		}
		// No retention: scribble over the caller's bytes, then compare.
		for i := range data {
			data[i] = 0xff
		}
		again, d2, err := Parse(kept)
		if err != nil || d2 != digest || !reflect.DeepEqual(file, again) {
			t.Fatalf("the File changed when the input did: %v", err)
		}
		// The digest is over the document, not its layout.
		var ind bytes.Buffer
		if json.Indent(&ind, kept, "", "\t") == nil {
			if _, d3, err := Parse(ind.Bytes()); err != nil || d3 != digest {
				t.Fatalf("re-indented: digest %q, err %v, want %q", d3, err, digest)
			}
		}
	})
}
