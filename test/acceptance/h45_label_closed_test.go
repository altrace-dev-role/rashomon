package acceptance

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// TestH45_DeterministicAndClosed is H-45.
//
// Two installs and two runs, because the label must be comparable across
// machines: the digests in this store are keyed per install and deliberately
// are not, and a label that inherited that property would be useless in the
// report it is meant to feed. A label is a constant from a closed vocabulary,
// so it is the same everywhere or it is wrong.
//
// Break: fall back to a substring of the path.
func TestH45_DeterministicAndClosed(t *testing.T) {
	const path = "/home/u/.ssh/id_rsa"

	first := labelFromRun(t, path)
	second := labelFromRun(t, path)
	if first != second {
		t.Errorf("two installs labelled the same path %q and %q", first, second)
	}
	if first != "ssh-key" {
		t.Errorf("label = %q, want %q", first, "ssh-key")
	}
}

// TestH45_CaseVariant is the case-folding half: ~/.SSH/ID_RSA labels as
// ~/.ssh/id_rsa does. A case-sensitive table is a table that misses the file
// on a case-insensitive filesystem, which is the default on two of the three
// platforms this runs on.
func TestH45_CaseVariant(t *testing.T) {
	if got, want := labelFromRun(t, "~/.SSH/ID_RSA"), labelFromRun(t, "~/.ssh/id_rsa"); got != want {
		t.Errorf("case variant labelled %q, canonical spelling %q", got, want)
	}
}

// TestH45_NoneVersusUnknown holds apart the two answers that are easiest to
// collapse: a path that was read and matched nothing, and a path that could
// not be read at all.
func TestH45_NoneVersusUnknown(t *testing.T) {
	if got := labelFromRun(t, "/srv/app/main.go"); got != "none" {
		t.Errorf("an unmatched path labelled %q, want %q", got, "none")
	}
	if got := labelFromRun(t, ""); got != "unknown" {
		t.Errorf("an empty path labelled %q, want %q", got, "unknown")
	}
}

// TestH45_EveryLabelIsInTheVocabulary sweeps a spread of real paths through
// the whole recorder and asserts that whatever comes back is in shape.Labels().
// The vocabulary is the schema's enum, and a value outside it is a record that
// fails the published contract in the field.
func TestH45_EveryLabelIsInTheVocabulary(t *testing.T) {
	// Gated here rather than in each subtest: sixteen skips under a PASS
	// summary line is the shape a reader mistakes for coverage.
	if !labelSchemaLive(t) {
		t.Skip(labelSchemaSkip)
	}
	vocab := map[string]bool{}
	for _, l := range shape.Labels() {
		vocab[l] = true
	}
	for _, path := range []string{
		"/home/u/.ssh/id_ed25519", "/home/u/.ssh/authorized_keys",
		"/srv/.env", "/srv/.env.production", "/srv/.envrc",
		"/home/u/.kube/kubeconfig", "/infra/terraform.tfstate",
		"/etc/ssl/server.pem", "/etc/ssl/server.key", "/opt/app/app.jks",
		"/home/u/.netrc", "/home/u/.aws/credentials", "/k8s/secrets.yaml",
		"/srv/app/main.go", "README.md", "",
	} {
		t.Run(path, func(t *testing.T) {
			got := labelFromRun(t, path)
			if !vocab[got] {
				t.Errorf("label %q is not in shape.Labels()", got)
			}
		})
	}
}

// labelFromRun drives one Read call through a fresh install and returns the
// label its declaration carries, or the empty string when this build's schema
// does not carry the field yet.
func labelFromRun(t *testing.T, path string) string {
	t.Helper()
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolName = "Read"
	p.ToolInput = map[string]any{"file_path": path}
	e.mustHook(p.build(t))

	decls := e.declarations(testSession)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	// schema_version, not the presence of file_label. Gating on the field
	// would mean a regression that stopped emitting it -- an entry falling
	// out of pathFields, an early return in shape.Label -- silently switched
	// off the tests that exist to catch that.
	v, ok := decls[0].fields["schema_version"].(float64)
	if !ok {
		t.Fatalf("declaration carries no numeric schema_version: %v", decls[0].fields["schema_version"])
	}
	if v < 3 {
		t.Skip(labelSchemaSkip)
	}
	return decls[0].str("file_label")
}
