package main

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

const sample = `goos: darwin
BenchmarkDerive/bash_short-8         	      20	     12345 ns/op	    4096 B/op	      31 allocs/op
BenchmarkDerive/bash_short-8         	      20	     12000 ns/op	    4000 B/op	      30 allocs/op
BenchmarkRecordBytesPerCall-8        	      20	    900000 ns/op	 1850.00 MB/s	   16384 B/op	     120 allocs/op
BenchmarkLexTokens-8                 	      20	     37465 ns/op	 109.33 MB/s	       120.4 B/token	      1068 tokens/op	  128544 B/op	     750 allocs/op
BenchmarkRecordBytesPerCall-8   	      20	  25018085 ns/op	   0.08 MB/s	      2082 B/call	      1478 records-B/call	   16242 B/op	      84 allocs/op
PASS
`

func TestRES36b_ParseKeepsTheBestRun(t *testing.T) {
	got, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if r := got["BenchmarkDerive/bash_short"]; r.BytesPerOp != 4000 || r.AllocsPerOp != 30 {
		t.Errorf("best of two runs: %+v", r)
	}
	if r := got["BenchmarkRecordBytesPerCall"]; r.BytesPerOp != 16242 || r.AllocsPerOp != 84 {
		t.Errorf("best of a plain line and one with hyphenated custom units: %+v", r)
	}
	if r := got["BenchmarkLexTokens"]; r.BytesPerOp != 128544 || r.AllocsPerOp != 750 {
		t.Errorf("line with custom metrics between ns/op and B/op: %+v", r)
	}
	if _, err := Parse(strings.NewReader("BenchmarkNoAllocs-8   20   100 ns/op\n")); err == nil {
		t.Error("a benchmark without ReportAllocs parsed as gateable")
	}
}

func TestRES36c_CompareFailsClosed(t *testing.T) {
	results := map[string]Result{"BenchmarkA": {1000, 10}, "BenchmarkB": {2000, 20}}
	budgets := []budget.Benchmark{
		{Name: "BenchmarkA", BytesPerOpMax: 1000, AllocsPerOpMax: 10},
		{Name: "BenchmarkB", BytesPerOpMax: 1500, AllocsPerOpMax: 20},
		{Name: "BenchmarkC", BytesPerOpMax: 1, AllocsPerOpMax: 1},
		{Name: "BenchmarkA", BytesPerOpMax: 0, AllocsPerOpMax: 0},
	}
	problems := Compare(results, budgets, 0.10)
	want := []string{
		"BenchmarkB: 2000 B/op over budget 1500",
		"BenchmarkC: budgeted but did not run",
		"BenchmarkA: budget row has no figures",
	}
	if strings.Join(problems, "|") != strings.Join(want, "|") {
		t.Errorf("problems %q\nwant %q", problems, want)
	}
	if p := Compare(map[string]Result{"BenchmarkA": {1100, 11}}, budgets[:1], 0.10); len(p) != 0 {
		t.Errorf("ten percent over must pass: %q", p)
	}
	if p := Compare(map[string]Result{"BenchmarkA": {1101, 10}}, budgets[:1], 0.10); len(p) != 1 {
		t.Errorf("more than ten percent over must fail: %q", p)
	}
}
