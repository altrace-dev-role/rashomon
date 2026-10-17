package perf

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Fuzz targets over the two readers of Go-toolchain and OS output the gates
// decide with (rubric fix list entry 23, errors-7). Neither reads bytes a
// model or a user wrote, but a reader that panics on a line it did not expect
// takes the gate down instead of failing it, and the first of these did.

const initTraceSeed = "init internal/bytealg @0.008 ms, 0.001 ms clock, 0 bytes, 0 allocs\n" +
	"init errors @0.11 ms, 0.003 ms clock, 160 bytes, 2 allocs\n" +
	"gc 1 @0.6 ms 0%: 0.010+0.2+0.004 ms clock, 0.10+0/0.3/0+0.04 ms cpu, 4->4->0 MB, 4 MB goal, 0 MB stacks, 0 MB globals, 8 P\n"

// TestRubricErrors7_ShortInitLineIsAnErrorNotAPanic: an init line of ten
// fields passed the length check and was then indexed at its eleventh, so a
// trace the runtime truncated mid-line crashed the gate rather than failing it.
func TestRubricErrors7_ShortInitLineIsAnErrorNotAPanic(t *testing.T) {
	for _, ln := range []string{
		"init a @0 ms, 0 ms clock, 0 bytes, 0\n",
		"init a @0 ms, 0 ms clock, 0 bytes,\n",
	} {
		if _, err := ParseInitTrace(ln); err == nil {
			t.Errorf("ParseInitTrace(%q) read a truncated init line as an init", ln)
		}
	}
}

// TestRubricErrors7_MalformedFootprintLineFailsClosed: /usr/bin/time -l writes
// one footprint line. When the first line shaped like it does not hold a
// number, the report is not trusted for a later one: the read fails, so the
// gate records no measurement rather than whichever figure came after.
func TestRubricErrors7_MalformedFootprintLineFailsClosed(t *testing.T) {
	for _, report := range []string{
		"oo2 peak memory\ffootprint\n             2622016  peak memory footprint\n",
		"99999999999999999999  peak memory footprint\n             2622016  peak memory footprint\n",
	} {
		if got, ok := ParseFootprint(report); ok {
			t.Errorf("ParseFootprint(%q) = %d, true; a malformed first footprint line fails the read", report, got)
		}
	}
}

// FuzzParseInitTrace: a trace that parses counted every init and gc line it
// holds, and a well-formed init line read back what it said.
func FuzzParseInitTrace(f *testing.F) {
	f.Add(initTraceSeed, "regexp/syntax", int64(61824), int64(53))
	f.Add("init a @0 ms, 0 ms clock, 0 bytes, 0\n", "x", int64(0), int64(0))
	f.Add("  init  \r\ngc \n", "p", int64(1)<<40, int64(7))
	f.Fuzz(func(t *testing.T, stderr, pkg string, b, n int64) {
		checkInitTrace(t, stderr)
		if f := strings.Fields(pkg); len(f) != 1 || f[0] != pkg || len(pkg) > 1024 {
			return // a package path is one field, as strings.Fields splits them
		}
		ln := fmt.Sprintf("init %s @0.5 ms, 0.1 ms clock, %d bytes, %d allocs\n", pkg, b, n)
		got, err := ParseInitTrace(ln)
		if err != nil || got.Packages != 1 || got.Bytes != b || got.Allocs != n {
			t.Fatalf("ParseInitTrace(%q) = %+v, %v", ln, got, err)
		}
	})
}

func checkInitTrace(t *testing.T, stderr string) {
	t.Helper()
	got, err := ParseInitTrace(stderr)
	if err != nil {
		return
	}
	var inits, gcs int
	for _, ln := range strings.Split(stderr, "\n") {
		ln = strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(ln, "gc "):
			gcs++
		case strings.HasPrefix(ln, "init "):
			inits++
		}
	}
	if got.Packages != inits || got.GCLines != gcs {
		t.Fatalf("ParseInitTrace(%q) = %+v; the trace holds %d init and %d gc lines", stderr, got, inits, gcs)
	}
}

// FuzzParseFootprint: a report without the footprint line is not a zero, the
// line appended to any such report reads back as written, and the first
// footprint line decides: a malformed one fails the read rather than being
// read past to a later one.
//
// Which line is the footprint line is the oracle's own reading, not the
// parser's: any line whose whitespace-separated fields are a number and then
// "peak memory footprint", whatever whitespace separates them. The first
// version skipped only a prefix holding the literal text, so a line split by
// \f or \r counted for the parser and not for the oracle, and the two seeds
// below are the inputs the CI command found in under 30 s.
func FuzzParseFootprint(f *testing.F) {
	f.Add("        0.01 real         0.00 user         0.00 sys\n             6029312  maximum resident set size\n", int64(2622016))
	f.Add("", int64(0))
	f.Add("peak memory footprint\n1 2 peak memory footprint", int64(-1))
	f.Add("k t\n1ory f1 tprint\noo2 peak memory\ffootprint", int64(-33))
	f.Add("k memo|y footprint\n\xff\xff2 peak\rmemory footprint", int64(141))
	f.Fuzz(func(t *testing.T, prefix string, v int64) {
		if len(prefix) > 60<<10 {
			return // a line bufio cannot hold
		}
		report := prefix + "\n" + fmt.Sprintf("%20d  %s\n", v, footprintLine)
		if num, found := firstFootprintField(prefix); found {
			want, err := strconv.ParseInt(num, 10, 64)
			for _, in := range []string{prefix, report} {
				if got, ok := ParseFootprint(in); ok != (err == nil) || ok && got != want {
					t.Fatalf("ParseFootprint(%q) = %d, %v; its first footprint line reads %q", in, got, ok, num)
				}
			}
			return
		}
		if got, ok := ParseFootprint(prefix); ok {
			t.Fatalf("ParseFootprint(%q) = %d without a footprint line", prefix, got)
		}
		if got, ok := ParseFootprint(report); !ok || got != v {
			t.Fatalf("ParseFootprint(%q) = %d, %v; want %d, true", report, got, ok, v)
		}
	})
}

// firstFootprintField returns the first field of the report's first line that
// is a footprint line: four whitespace-separated fields, the last three
// "peak memory footprint".
func firstFootprintField(report string) (string, bool) {
	for _, ln := range strings.Split(report, "\n") {
		if f := strings.Fields(ln); len(f) == 4 && f[1] == "peak" && f[2] == "memory" && f[3] == "footprint" {
			return f[0], true
		}
	}
	return "", false
}
