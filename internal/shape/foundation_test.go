package shape

import "testing"

// TestPG1b_FourMoreRunnersAreTestGates: rake, composer, mix and bazel test
// runs are test gates, class test with a runner digest, so the evaluator can
// key a family on them. The shape probe of 2026-10-23 found all four missing.
func TestPG1b_FourMoreRunnersAreTestGates(t *testing.T) {
	for _, cmd := range []string{"rake test", "composer test", "mix test", "bazel test //..."} {
		sh := verbOf(t, cmd)
		if sh.VerbClass != VerbTest || sh.RunnerDigest == nil {
			t.Errorf("%q: class %s, runner digest %v; want test with a digest", cmd, sh.VerbClass, sh.RunnerDigest != nil)
		}
	}
}

// TestRC3b_InPlaceEditorsSetMayWrite: an editor that rewrites files in place
// sets may_write, so a gate that ran before it reads stale after it. A check
// or list run of the same tool does not write and keeps the bit clear. The
// probe of 2026-10-23 found every editor below reading false.
func TestRC3b_InPlaceEditorsSetMayWrite(t *testing.T) {
	for _, cmd := range []string{
		"sed -i 's/a/b/' x.go", "sed -i '' 's/a/b/' x.go", "sed -i.bak 's/a/b/' x.go",
		"perl -pi -e 's/a/b/' x.go", "perl -i -pe 's/a/b/' x.go",
		"gofmt -w .", "go fmt ./...", "cargo fmt", "black .", "ruff format .", "isort .", "rustfmt src/main.rs",
		"npx prettier --write .", "clang-format -i a.cc", "npx eslint --fix .",
	} {
		if sh := verbOf(t, cmd); !sh.MayWrite {
			t.Errorf("%q: may_write false, want true", cmd)
		}
	}
	for _, cmd := range []string{
		"sed 's/a/b/' x.go", "gofmt -l .", "cargo fmt --check", "black --check .", "ruff format --diff .",
		"npx prettier --check .", "sed -n '1,3p' x.go | grep -i foo",
	} {
		if sh := verbOf(t, cmd); sh.MayWrite {
			t.Errorf("%q: may_write true, want false (a check or a read does not write)", cmd)
		}
	}
}
