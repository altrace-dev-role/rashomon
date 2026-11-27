package lang

import "testing"

func TestMarkedOf(t *testing.T) {
	for _, c := range []struct {
		path string
		want Marked
	}{
		{"a/b_test.go", MarkedGo},
		{"A/B_TEST.GO", MarkedGo},
		{"tests/test_a.py", MarkedPython},
		{"tests/test_a.PY", MarkedPython},
		{"a_test.pyc", NotMarked},
		{"a.go.txt", NotMarked},
		{"go", NotMarked},
		{"", NotMarked},
	} {
		if got := MarkedOf([]byte(c.path)); got != c.want {
			t.Errorf("MarkedOf(%q) = %d, want %d", c.path, got, c.want)
		}
	}
}
