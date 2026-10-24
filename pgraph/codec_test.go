package pgraph

import (
	"bytes"
	"strings"
	"testing"
)

func sampleLog(t testing.TB) *Log {
	t.Helper()
	var l Log
	p := mustProposed(t, "x")
	l.Reconcile(p, cfgA, "c1", []Triple{tri("a", Calls, "b"), tri("a", Calls, "c")})
	l.Reconcile(p, cfgA, "c2", []Triple{tri("a", Calls, "c"), tri("a", Embeds, "d")})
	return &l
}

func TestPL5_CodecRoundTripAndPrefix(t *testing.T) {
	l := sampleLog(t)
	b := l.Encode()
	l2, err := DecodeLog(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(l2.Encode(), b) || !bytes.Equal(l2.View(), l.View()) {
		t.Fatal("round trip changed bytes")
	}
	// Prefix property: appending never rewrites earlier bytes.
	n := len(b)
	l2.Reconcile(mustProposed(t, "x"), cfgA, "c3", nil)
	if !bytes.HasPrefix(l2.Encode(), b) || len(l2.Encode()) <= n {
		t.Fatal("append rewrote history")
	}
}

func TestPL5_DecodeRefuses(t *testing.T) {
	good := string(sampleLog(t).Encode())
	lines := strings.Split(strings.TrimSuffix(good, "\n"), "\n")
	rec := lines[1] // first assert record
	for name, in := range map[string]string{
		"no trailing newline": strings.TrimSuffix(good, "\n"),
		"bad header":          "pgraph-log v9\n" + rec + "\n",
		"unknown tier":        LogHeader + "\n" + strings.Replace(rec, "model-proposed", "PROVENX", 1) + "\n",
		"upgraded tier text":  LogHeader + "\n" + strings.Replace(rec, "model-proposed", "proven", 1) + "\n",
		"unknown op":          LogHeader + "\n" + "?" + rec[1:] + "\n",
		"unknown predicate":   LogHeader + "\n" + strings.Replace(rec, "calls", "causes", 1) + "\n",
		"short record":        LogHeader + "\n+\ta\tcalls\n",
		"double assert":       LogHeader + "\n" + rec + "\n" + rec + "\n",
		"retract of unknown":  LogHeader + "\n-" + rec[1:] + "\n",
		"nul byte":            LogHeader + "\n" + strings.Replace(rec, "a\t", "a\x00\t", 1) + "\n",
		"empty":               "",
	} {
		if _, err := DecodeLog([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPL5_DecodeCaps(t *testing.T) {
	big := LogHeader + "\n+\t" + strings.Repeat("x", MaxFieldBytes+1) + "\tcalls\tb\tmodel-proposed\t" + cfgA + "\tc\tx@1\n"
	if _, err := DecodeLog([]byte(big)); err == nil {
		t.Error("over-cap field accepted")
	}
	long := LogHeader + "\n" + strings.Repeat("y", MaxRecordBytes+1) + "\n"
	if _, err := DecodeLog([]byte(long)); err == nil {
		t.Error("over-cap record accepted")
	}
	if _, err := DecodeLog(make([]byte, MaxLogBytes+1)); err == nil {
		t.Error("over-cap log accepted")
	}
}

func FuzzDecodeLog(f *testing.F) {
	f.Add(sampleLog(f).Encode())
	f.Add([]byte(LogHeader + "\n"))
	f.Add([]byte(""))
	f.Add([]byte(LogHeader + "\n+\t\x00\tcalls\tb\tPROVEN\tc\tc\te@1\n"))
	f.Add([]byte(LogHeader + "\n+\tfunc:a.F\tmay-call\tmethod:a.T.M\tALGORITHMIC\t" + cfgA + "\tc\tcha@v0\n"))
	f.Add([]byte(LogHeader + "\n+\tfunc:a.F\tmay-call\tmethod:a.T.M\tPROVEN\t" + cfgA + "\tc\tgotypes@v1\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		l, err := DecodeLog(in)
		if err != nil {
			return
		}
		// Accepted input is canonical: it re-encodes to the same bytes, and
		// the log never grows beyond its input.
		if out := l.Encode(); !bytes.Equal(out, in) {
			t.Fatalf("not canonical:\n in=%q\nout=%q", in, out)
		}
		if len(l.View()) > len(in)+len(ViewHeader)+1 {
			t.Fatal("view larger than input")
		}
	})
}
