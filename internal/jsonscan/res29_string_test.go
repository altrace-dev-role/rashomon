package jsonscan

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRES29_StringDecodesOnce: DecodeString decodes a string span to what Head
// decodes it to -- escapes, surrogate pairs, an unpaired surrogate and
// invalid UTF-8 alike -- and does it in one allocation no larger than the
// encoded bytes. Head into a nil buffer and then string() made two copies of
// a command after growing the first from nothing: decoded by each of a
// hook's three readers of an 8 MiB command, that was most of the hook's RSS.
func TestRES29_StringDecodesOnce(t *testing.T) {
	for _, raw := range []string{
		`""`, `"plain"`, `"a\"b\\c\/d\n\t\r\b\f"`, `"é€"`, `"😀"`, `"\ud83d x"`,
		"\"\xff\xfe ok\"", `"caf` + "é" + `"`,
	} {
		buf := []byte(raw)
		s := ValueSpan(buf)
		want, _, err := Head(buf, s, 3*int(s.Len), nil)
		if err != nil {
			t.Fatalf("Head(%q): %v", raw, err)
		}
		got, err := DecodeString(buf, s)
		if err != nil || got != string(want) {
			t.Errorf("DecodeString(%q) = %q, %v; want %q", raw, got, err, want)
		}
		var std string
		if json.Unmarshal(buf, &std) == nil && got != std {
			t.Errorf("DecodeString(%q) = %q; encoding/json decodes %q", raw, got, std)
		}
	}
	buf := []byte(`"` + strings.Repeat(`a\n`, 1<<16) + `"`)
	s := ValueSpan(buf)
	var out string
	if n := testing.AllocsPerRun(10, func() { out, _ = DecodeString(buf, s) }); n != 1 {
		t.Errorf("DecodeString allocated %v times, want once", n)
	}
	if len(out) != 2<<16 {
		t.Fatalf("decoded %d bytes, want %d", len(out), 2<<16)
	}
	if _, err := DecodeString([]byte(`12`), ValueSpan([]byte(`12`))); err == nil {
		t.Error("DecodeString of a number: no error")
	}
}
