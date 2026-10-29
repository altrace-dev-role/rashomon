package platform

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestPL8e_UnknownHeaderIsUnknown: the fixture's records under the next
// header version are a format this package does not know: Impact refuses it
// and Obligations is the one unknown row, never families read as v0.
func TestPL8e_UnknownHeaderIsUnknown(t *testing.T) {
	v1 := bytes.Replace(fixtureLog(t), []byte(LogHeader+"\n"), []byte("pgraph-log v1\n"), 1)
	root, lp := project(t, v1)
	if fams, err := Impact(context.Background(), root, lp, []string{"a/a.go"}); !errors.Is(err, ErrUnknownHeader) || fams != nil {
		t.Errorf("a v1 log: %+v, %v; want ErrUnknownHeader and no family", fams, err)
	}
	rows := Obligations(RunView{Root: root, WrittenPaths: []string{"a/a.go"}, LogPath: lp})
	if len(rows) != 1 || rows[0].State != verify.ObligationUnknown || rows[0].Kind != verify.PlatformImpact {
		t.Errorf("a v1 log's obligations: %+v, want one unknown platform_impact row", rows)
	}
}

// TestPL8f_OverCapIsUnknown: a log one byte over the cap Impact reads under is
// refused before it is read; at the cap it is read.
func TestPL8f_OverCapIsUnknown(t *testing.T) {
	log := fixtureLog(t)
	root, lp := project(t, log)
	n := int64(len(log))
	if fams, err := impactWithin(context.Background(), root, lp, []string{"a/a.go"}, n-1); !errors.Is(err, ErrOverCap) || fams != nil {
		t.Errorf("cap %d under a %d-byte log: %+v, %v; want ErrOverCap and no family", n-1, n, fams, err)
	}
	if fams, err := impactWithin(context.Background(), root, lp, []string{"a/a.go"}, n); err != nil || len(fams) != 1 {
		t.Errorf("cap %d at the log's size: %+v, %v; want a's family", n, fams, err)
	}
}

// TestPL8g_HashChangesWhenAPackageFactChanges: two logs that differ by one
// retraction of one of a's facts give a a new Hash and b the same one.
func TestPL8g_HashChangesWhenAPackageFactChanges(t *testing.T) {
	log1 := fixtureLog(t)
	log2 := append(append([]byte(nil), log1...),
		"-\tmethod:example.com/m/a.T.M\tcalls\tfunc:example.com/m/a.F\tPROVEN\t"+cfg+"\tc3\tgotypes@v2\n"...)
	root1, lp1 := project(t, log1)
	root2, lp2 := project(t, log2)
	f1 := impactOf(t, root1, lp1, "a/a.go", "b/b.go")
	f2 := impactOf(t, root2, lp2, "a/a.go", "b/b.go")
	if len(f1) != 2 || len(f2) != 2 {
		t.Fatalf("families %+v and %+v, want a and b in each", f1, f2)
	}
	if f1[0].Hash == f2[0].Hash {
		t.Errorf("a's Hash %s did not change when one of its facts was retracted", f1[0].Hash)
	}
	if f1[1].Hash != f2[1].Hash {
		t.Errorf("b's Hash moved from %s to %s; none of its facts changed", f1[1].Hash, f2[1].Hash)
	}
}

// TestPL8e2_MalformedLogIsRefused: a torn last line, a short record, an
// unknown op, predicate or tier, an empty field and a line over the record
// cap are each refused, never a live set built around them.
func TestPL8e2_MalformedLogIsRefused(t *testing.T) {
	good := "+\tpkg:p\tdeclares\tfunc:p.F\tPROVEN\t" + cfg + "\tc1\tx@v1"
	for name, body := range map[string]string{
		"torn":      good,
		"short":     "+\tpkg:p\tdeclares\tfunc:p.F\tPROVEN\n",
		"op":        strings.Replace(good, "+", "*", 1) + "\n",
		"predicate": strings.Replace(good, "declares", "contains", 1) + "\n",
		"tier":      strings.Replace(good, "PROVEN", "proven", 1) + "\n",
		"empty":     strings.Replace(good, "func:p.F", "", 1) + "\n",
		"long":      strings.Replace(good, "func:p.F", strings.Repeat("x", maxRecordBytes), 1) + "\n",
	} {
		if live, err := decode(context.Background(), strings.NewReader(LogHeader+"\n"+body), nil); err == nil {
			t.Errorf("%s: decoded to %v, want refused", name, live)
		}
	}
	if live, err := decode(context.Background(), strings.NewReader(LogHeader+"\n"+good+"\n"), nil); err != nil || len(live) != 1 {
		t.Errorf("one good record: %v, %v; want one live fact", live, err)
	}
}

// TestPL8c3_CancelledDecodeReturnsNothing: a decode whose context is done
// returns the context's error and no live set, not the facts read so far.
func TestPL8c3_CancelledDecodeReturnsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if live, err := decode(ctx, bytes.NewReader(synthLog(t, 2*checkEvery)), nil); !errors.Is(err, context.Canceled) || live != nil {
		t.Errorf("a cancelled decode of %d records: %d facts, %v; want context.Canceled and none", 2*checkEvery, len(live), err)
	}
}

// FuzzDecode: the log is an untrusted file. No input panics the decoder, and
// whatever it accepts is facts of non-empty fields within the field cap at a
// v0 tier.
func FuzzDecode(f *testing.F) {
	fix := fixtureLog(f)
	f.Add(fix)
	f.Add(bytes.Replace(fix, []byte(LogHeader), []byte("pgraph-log v1"), 1))
	f.Add(fix[:len(fix)-1])
	f.Add([]byte{})
	f.Add([]byte(LogHeader + "\n-\ta\tcalls\tb\tALGORITHMIC\tc\td\te\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		live, err := decode(context.Background(), bytes.NewReader(b), nil)
		if err != nil {
			return
		}
		for k := range live {
			for _, s := range []string{k.subject, k.predicate, k.object, k.tier, k.config} {
				if s == "" || len(s) > maxFieldBytes {
					t.Fatalf("accepted a field of %d bytes in %+v", len(s), k)
				}
			}
			if tierRank(k.tier) == 0 {
				t.Fatalf("accepted tier %q", k.tier)
			}
		}
	})
}
