package shape

import (
	"slices"
	"testing"
)

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

// runnerKindLines carry a runner digest, each with the kind of the runner it
// digests. The last two have two runners of different kinds, so the kind is
// the digested runner's and not any runner's on the line.
var runnerKindLines = []struct{ cmd, kind string }{
	{"go test ./...", RunnerTest}, {"go test ./... && echo ok", RunnerTest}, {"make test 2>&1 | tail -40", RunnerTest},
	{"go build ./...", RunnerBuild}, {"make release; ls dist", RunnerBuild}, {"cargo build --release | tee log", RunnerBuild},
	{"bazel test //...", RunnerTest}, {"rake test", RunnerTest},
	{"go test ./... && go build ./...", RunnerBuild},      // both reach the line: the last is digested
	{"go build ./... | tail; go test ./...", RunnerBuild}, // the hidden runner is digested over the reaching one
}

// noRunnerDigestLines carry none: no runner, a runner a pushd displaced, two
// hidden runners, and a line whose masking is not known.
var noRunnerDigestLines = []string{
	"ls -la", "echo hi | grep h", "cd x && cat y",
	"pushd /a && go test ./...", "make release | tail; make test | tail", "make test | tail; exit ${PIPESTATUS[0]}",
}

func orNil(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// TestPG1c_RunnerKindFollowsRunnerDigest: the record names the list the
// digested runner came from (D9, B27). status_masked is none on a line read
// to its end and verb_class flips under sequencing, so `go test ./... && echo
// ok` is class package, masked none, digest set: without the kind nothing on
// the record says it ran a test runner.
func TestPG1c_RunnerKindFollowsRunnerDigest(t *testing.T) {
	for _, tc := range runnerKindLines {
		t.Run(tc.cmd, func(t *testing.T) {
			sh := verbOf(t, tc.cmd)
			if sh.RunnerDigest == nil || sh.RunnerKind == nil || *sh.RunnerKind != tc.kind {
				t.Fatalf("digest set %t kind %s, want kind %s", sh.RunnerDigest != nil, orNil(sh.RunnerKind), tc.kind)
			}
		})
	}
	for _, cmd := range noRunnerDigestLines {
		t.Run(cmd, func(t *testing.T) {
			sh := verbOf(t, cmd)
			if sh.RunnerDigest != nil || sh.RunnerKind != nil {
				t.Fatalf("no runner digest: digest set %t kind %s", sh.RunnerDigest != nil, orNil(sh.RunnerKind))
			}
		})
	}
}

// TestPG1c_RunnerKindIsSetExactlyWhenRunnerDigestIs: the two are one fact,
// so a reader that finds a digest finds its kind, and a kind never names a
// runner the record does not identify. Every line above, and a call that is
// not a shell.
func TestPG1c_RunnerKindIsSetExactlyWhenRunnerDigestIs(t *testing.T) {
	var shapes []Shape
	for _, tc := range runnerKindLines {
		shapes = append(shapes, verbOf(t, tc.cmd))
	}
	for _, cmd := range noRunnerDigestLines {
		shapes = append(shapes, verbOf(t, cmd))
	}
	shapes = append(shapes, Derive("Read", []byte(`{"file_path":"/x"}`), []byte("key")))
	for i, sh := range shapes {
		if (sh.RunnerKind == nil) != (sh.RunnerDigest == nil) {
			t.Errorf("shape %d: digest set %t, kind %s: one is set without the other", i, sh.RunnerDigest != nil, orNil(sh.RunnerKind))
		}
	}
}

// TestPG1c_RunnerKindsAreTheListsWords: the vocabulary is the two lists'
// names, in this order, for the schema enum test.
func TestPG1c_RunnerKindsAreTheListsWords(t *testing.T) {
	if got, want := RunnerKinds(), []string{"test", "build"}; !slices.Equal(got, want) {
		t.Fatalf("RunnerKinds() = %q, want %q", got, want)
	}
}
