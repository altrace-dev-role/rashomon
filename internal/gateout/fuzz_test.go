package gateout

import "testing"

func FuzzClassify(f *testing.F) {
	f.Add("test", "go", []byte("ok\tpkg\t0.01s"))
	f.Add("test", "make", []byte("Tests: 1 passed"))
	f.Add("build", "go", []byte("anything"))
	f.Fuzz(func(t *testing.T, kind, program string, input []byte) {
		got := Classify(kind, program, input)
		switch got.Kind {
		case "summary_pass", "summary_fail", "fail_line", "empty_run", "no_summary", "unattributable", "unread":
		default:
			t.Fatalf("unknown kind %q", got.Kind)
		}
		if got.Reason != nil && *got.Reason != "too_large" {
			t.Fatalf("unknown reason %q", *got.Reason)
		}
	})
}
