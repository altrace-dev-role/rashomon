package spec

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
)

// Trust bindings (tech spec 2.2.10; PRD PF-SP2; card SPEC-TRUST1): the user's
// word, given at a terminal by `rashomon spec trust`, that one repository
// spec may reach the agent. A binding names the spec's project, its path and
// its digest, so an edit, being a new digest, matches no binding: Trusted's
// "any edit drops it". The file sits under the store root, never in the
// repository, so a repository cannot trust itself.
const (
	// TrustFile is the bindings file's name under the store root.
	TrustFile = "spec-trust.json"
	// MaxTrustBytes caps the file on read and on write, the project index's
	// cap (verify.MaxIndexBytes).
	MaxTrustBytes = 64 << 10
)

// Binding is one trusted repository spec. ProjectRoot is the project the spec
// sits in as baseline.Key names it, SpecPath the spec file discovery reads,
// SpecDigest the digest Parse gave the bytes the user was shown.
type Binding struct {
	ProjectRoot string `json:"project_root"`
	SpecPath    string `json:"spec_path"`
	SpecDigest  string `json:"spec_digest"`
	BoundAtMS   int64  `json:"bound_at_unix_ms"`
}

// Bindings is the file's one object: at most one binding per project and
// path, sorted by ProjectRoot then SpecPath.
type Bindings struct {
	Bindings []Binding `json:"bindings"`
}

// ReadBindings reads the bindings file at path through fsys. A missing file
// is no binding and no error. A file over MaxTrustBytes, or one that is not
// this shape (one JSON value, no field it does not name), is an error, which
// discovery reads as no binding: untrusted, the fail-closed direction.
func ReadBindings(fsys FS, path string) (*Bindings, error) {
	data, err := fsys.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Bindings{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTrustBytes {
		return nil, fmt.Errorf("spec: %s is over its %d-byte cap", path, MaxTrustBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var b Bindings
	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("spec: %s: %w", path, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("spec: %s: data after its one value", path)
	}
	return &b, nil
}

// Holds reports whether a binding names all three of the project, the path
// and the digest. A nil Bindings holds nothing.
func (b *Bindings) Holds(projectRoot, specPath, digest string) bool {
	return b != nil && slices.ContainsFunc(b.Bindings, func(x Binding) bool {
		return x.ProjectRoot == projectRoot && x.SpecPath == specPath && x.SpecDigest == digest
	})
}

// Bind adds x, replacing any binding of its project and path.
func (b *Bindings) Bind(x Binding) {
	b.Unbind(x.ProjectRoot, x.SpecPath)
	b.Bindings = append(b.Bindings, x)
	slices.SortFunc(b.Bindings, func(p, q Binding) int {
		return cmp.Or(cmp.Compare(p.ProjectRoot, q.ProjectRoot), cmp.Compare(p.SpecPath, q.SpecPath))
	})
}

// Unbind removes the binding of the project and path and reports whether
// there was one.
func (b *Bindings) Unbind(projectRoot, specPath string) bool {
	n := len(b.Bindings)
	b.Bindings = slices.DeleteFunc(b.Bindings, func(x Binding) bool {
		return x.ProjectRoot == projectRoot && x.SpecPath == specPath
	})
	return len(b.Bindings) < n
}

// EncodeBindings is b as the bindings file's bytes, which hook's
// WriteSpecBindings writes: this package writes no file (VER-37f). An
// encoding over MaxTrustBytes is refused: evicting a binding, as the project
// index evicts a family, would untrust another project's spec without a word.
func EncodeBindings(b *Bindings) ([]byte, error) {
	out := Bindings{Bindings: b.Bindings}
	if out.Bindings == nil {
		out.Bindings = []Binding{}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if data = append(data, '\n'); len(data) > MaxTrustBytes {
		return nil, fmt.Errorf("spec: the trust bindings would be over their %d-byte cap; untrust a spec first", MaxTrustBytes)
	}
	return data, nil
}
