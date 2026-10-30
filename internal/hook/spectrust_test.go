package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// TestST1d_DiscoveryReadsTheBinding: the steer step's discovery reads the
// trust bindings under the store root (card SPEC-TRUST1). A binding of the
// spec's project, path and digest makes the repository spec trusted; the
// same bytes at another path of the same project, a binding of another
// project, a binding of another digest (the file as it was before an edit)
// and a bindings file over its cap each leave it untrusted, as does no
// bindings file at all.
func TestST1d_DiscoveryReadsTheBinding(t *testing.T) {
	_, digest, err := spec.Parse([]byte(specOneGate))
	if err != nil {
		t.Fatal(err)
	}
	_, other, err := spec.Parse([]byte(specOtherGate))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		// bind is the binding written for a tree whose root spec is at path;
		// none is written when it is nil.
		bind func(s specTree, path string) *spec.Binding
		sub  bool // the cwd is base/sub, whose own spec has the same bytes
		over bool // the binding is padded past the file's cap
		want spec.Trust
	}{
		{"bound", func(s specTree, path string) *spec.Binding {
			return &spec.Binding{ProjectRoot: s.base, SpecPath: path, SpecDigest: digest, BoundAtMS: 1}
		}, false, false, spec.Trusted},
		{"no bindings file", nil, false, false, spec.Untrusted},
		{"the same bytes at another path", func(s specTree, path string) *spec.Binding {
			return &spec.Binding{ProjectRoot: s.base, SpecPath: path, SpecDigest: digest, BoundAtMS: 1}
		}, true, false, spec.Untrusted},
		{"another project", func(s specTree, path string) *spec.Binding {
			return &spec.Binding{ProjectRoot: filepath.Join(s.base, "other"), SpecPath: path, SpecDigest: digest, BoundAtMS: 1}
		}, false, false, spec.Untrusted},
		{"another digest", func(s specTree, path string) *spec.Binding {
			return &spec.Binding{ProjectRoot: s.base, SpecPath: path, SpecDigest: other, BoundAtMS: 1}
		}, false, false, spec.Untrusted},
		{"over the cap", func(s specTree, path string) *spec.Binding {
			return &spec.Binding{ProjectRoot: s.base, SpecPath: path, SpecDigest: digest, BoundAtMS: 1}
		}, false, true, spec.Untrusted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSpecTree(t)
			s.mkdir(".git")
			path := s.put(s.base, specOneGate)
			sub := s.mkdir("sub")
			s.put(sub, specOneGate)
			f := newST(t)
			f.cwd = s.base
			if tc.sub {
				f.cwd = sub
			}
			if tc.bind != nil {
				writeBindings(t, f.st.Root(), *tc.bind(s, path), tc.over)
			}
			_, view, ok := f.red("toolu_r").judge(stEnv(0))
			if !ok || view.Spec == nil {
				t.Fatalf("judge: ok %v, spec %+v; want a spec", ok, view.Spec)
			}
			if view.Spec.Trust != tc.want {
				t.Errorf("the view's spec %+v: trust %q, want %q", view.Spec, view.Spec.Trust, tc.want)
			}
		})
	}
}

// writeBindings writes b as the store root's bindings file: through
// WriteSpecBindings, or, over, after enough other projects' bindings that
// the file is past spec.MaxTrustBytes, written by hand since
// spec.EncodeBindings refuses that size, and signed under the store's trust
// key as the verb signs, so the cap alone leaves it unread (D173).
func writeBindings(t *testing.T, root string, b spec.Binding, over bool) {
	t.Helper()
	all := spec.Bindings{Bindings: []spec.Binding{b}}
	if !over {
		if err := WriteSpecBindings(root, &all); err != nil {
			t.Fatal(err)
		}
		return
	}
	for i := 0; i < 600; i++ {
		p := filepath.Join("/elsewhere", strconv.Itoa(i))
		all.Bindings = append(all.Bindings, spec.Binding{ProjectRoot: p,
			SpecPath: filepath.Join(p, ".claude", "rashomon", "spec.json"), SpecDigest: b.SpecDigest, BoundAtMS: 1})
	}
	raw, err := json.Marshal(all.Bindings)
	if err != nil {
		t.Fatal(err)
	}
	key, err := specTrustKey(root)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	data := []byte(`{"bindings":` + string(raw) + `,"mac":"` + hex.EncodeToString(mac.Sum(nil)) + `"}` + "\n")
	if len(data) <= spec.MaxTrustBytes {
		t.Fatalf("the padded file is %d bytes, not over the %d-byte cap", len(data), spec.MaxTrustBytes)
	}
	if err := os.WriteFile(filepath.Join(root, spec.TrustFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
